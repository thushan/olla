package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

type discoveryFuncClient struct {
	mockDiscoveryClient
	discover func(context.Context, *domain.Endpoint) ([]*domain.ModelInfo, error)
}

func (c *discoveryFuncClient) DiscoverModels(ctx context.Context, ep *domain.Endpoint) ([]*domain.ModelInfo, error) {
	return c.discover(ctx, ep)
}

func TestDiscoveryFailureDoesNotCancelOtherEndpoints(t *testing.T) {
	failed := createMockEndpoint("http://failed", "failed")
	healthy := createMockEndpoint("http://healthy", "healthy")
	failure := errors.New("backend failure")
	client := &discoveryFuncClient{discover: func(ctx context.Context, ep *domain.Endpoint) ([]*domain.ModelInfo, error) {
		if ep == failed {
			return nil, failure
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return []*domain.ModelInfo{{Name: "healthy-model"}}, nil
	}}
	registry := &mockModelRegistry{}
	service := NewModelDiscoveryService(client, &mockEndpointRepository{healthyEndpoints: []*domain.Endpoint{failed, healthy}}, registry, DiscoveryConfig{Timeout: time.Second, ConcurrentWorkers: 1}, createTestLogger())
	err := service.DiscoverAll(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("expected original failure, got %v", err)
	}
	if registry.registerCallCount != 1 || len(registry.registeredModels) != 1 || registry.registeredModels[0].Name != "healthy-model" {
		t.Fatalf("healthy endpoint was not registered: %+v", registry)
	}
	if service.getFailureCount(healthy.URLString) != 0 {
		t.Fatal("healthy endpoint penalised by sibling failure")
	}
}

func TestDiscoveryCancellationAndRequestTimeout(t *testing.T) {
	for _, parentCancelled := range []bool{true, false} {
		t.Run(fmt.Sprintf("parent_cancelled=%v", parentCancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			endpoint := createMockEndpoint("http://backend", "backend")
			client := &discoveryFuncClient{discover: func(ctx context.Context, _ *domain.Endpoint) ([]*domain.ModelInfo, error) {
				if parentCancelled {
					cancel()
				}
				<-ctx.Done()
				return nil, fmt.Errorf("request failed: %w", ctx.Err())
			}}
			service := NewModelDiscoveryService(client, &mockEndpointRepository{healthyEndpoints: []*domain.Endpoint{endpoint}}, &mockModelRegistry{}, DiscoveryConfig{Timeout: time.Millisecond, ConcurrentWorkers: 1}, createTestLogger())
			err := service.DiscoverAll(ctx)
			expected := context.DeadlineExceeded
			failures := 1
			if parentCancelled {
				expected = context.Canceled
				failures = 0
			}
			if !errors.Is(err, expected) {
				t.Fatalf("expected %v, got %v", expected, err)
			}
			if got := service.getFailureCount(endpoint.URLString); got != failures {
				t.Fatalf("failures=%d, want %d", got, failures)
			}
		})
	}
}

func TestDiscoveryCooldownRecovery(t *testing.T) {
	endpoint := createMockEndpoint("http://backend", "backend")
	calls := 0
	broken := true
	client := &discoveryFuncClient{discover: func(context.Context, *domain.Endpoint) ([]*domain.ModelInfo, error) {
		calls++
		if broken {
			return nil, &ParseError{Format: "json", Err: errors.New("invalid response")}
		}
		return []*domain.ModelInfo{{Name: "recovered-model"}}, nil
	}}
	registry := &mockModelRegistry{}
	service := NewModelDiscoveryService(client, &mockEndpointRepository{healthyEndpoints: []*domain.Endpoint{endpoint}}, registry, DiscoveryConfig{Timeout: time.Second, ConcurrentWorkers: 1}, createTestLogger())
	for i := 1; i <= MaxConsecutiveFailures; i++ {
		if err := service.DiscoverAll(context.Background()); err == nil {
			t.Fatal("expected failure")
		}
		if i < MaxConsecutiveFailures && service.GetMetrics().ErrorsByEndpoint["_disabled_endpoints"] != 0 {
			t.Fatal("ordinary failure reported as disabled")
		}
	}
	if got := service.GetMetrics().ErrorsByEndpoint["_disabled_endpoints"]; got != 1 {
		t.Fatalf("disabled=%d", got)
	}
	if err := service.DiscoverAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != MaxConsecutiveFailures {
		t.Fatal("endpoint retried during cooldown")
	}
	expireCooldown := func() {
		service.mu.Lock()
		service.retryAfter[endpoint.URLString] = time.Now().Add(-time.Second)
		service.mu.Unlock()
	}
	expireCooldown()
	if err := service.DiscoverAll(context.Background()); err == nil {
		t.Fatal("expected failed recovery probe")
	}
	if !service.isEndpointDisabled(endpoint.URLString) {
		t.Fatal("failed probe did not restart cooldown")
	}
	expireCooldown()
	broken = false
	if err := service.DiscoverAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != MaxConsecutiveFailures+2 || registry.registerCallCount != 1 || service.getFailureCount(endpoint.URLString) != 0 {
		t.Fatal("successful recovery did not register models and reset failures")
	}
	if got := service.GetMetrics().ErrorsByEndpoint["_disabled_endpoints"]; got != 0 {
		t.Fatalf("recovered endpoint reported disabled: %d", got)
	}
}

func TestDiscoveryCooldownDuration(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, 20 * time.Minute} {
		service := NewModelDiscoveryService(nil, nil, nil, DiscoveryConfig{Interval: interval}, createTestLogger())
		before := time.Now()
		service.disableEndpoint("backend")
		if service.retryAfter["backend"].Before(before.Add(max(DefaultDiscoveryCooldown, interval))) {
			t.Fatal("cooldown shorter than configured interval or minimum")
		}
	}
}

func TestHTTPDiscoveryParseErrorClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"models":`)) }))
	defer server.Close()
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())
	defer client.Close()
	_, err := client.DiscoverModels(context.Background(), createTestEndpoint(server.URL, domain.ProfileOllama))
	var parseErr *ParseError
	var discoveryErr *DiscoveryError
	if !errors.As(err, &parseErr) || !errors.As(err, &discoveryErr) || discoveryErr.StatusCode != http.StatusOK {
		t.Fatalf("missing typed HTTP200 parse error: %v", err)
	}
	if got := GetUserFriendlyMessage(err); got != "invalid response format" {
		t.Fatalf("misleading message: %s", got)
	}
	metrics := client.GetMetrics()
	if metrics.TotalDiscoveries != 1 || metrics.FailedRequests != 1 || metrics.SuccessfulRequests != 0 {
		t.Fatalf("incorrect failure metrics: %+v", metrics)
	}
}

func TestHTTPDiscoveryResponseSizeBoundary(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprintf("extra=%d", extra), func(t *testing.T) {
			body := `{"models":[{"name":"model"}]}`
			body += strings.Repeat(" ", MaxResponseSize-len(body)+extra)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())
			defer client.Close()
			_, err := client.DiscoverModels(context.Background(), createTestEndpoint(server.URL, domain.ProfileOllama))
			if extra == 0 && err != nil {
				t.Fatal(err)
			}
			if extra == 1 && (err == nil || !strings.Contains(err.Error(), "exceeds")) {
				t.Fatalf("oversize response not rejected explicitly: %v", err)
			}
		})
	}
}

func TestHTTPAutoDiscoveryContinuesAfterParseFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`invalid`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"recovered-model"}]}`))
	}))
	defer server.Close()
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())
	defer client.Close()
	models, err := client.DiscoverModels(context.Background(), createTestEndpoint(server.URL, domain.ProfileAuto))
	if err != nil || len(models) != 1 {
		t.Fatalf("auto detection did not recover: %v, %+v", err, models)
	}
	metrics := client.GetMetrics()
	if metrics.TotalDiscoveries != 1 || metrics.SuccessfulRequests != 1 || metrics.FailedRequests != 0 {
		t.Fatalf("profile attempts inflated metrics: %+v", metrics)
	}
}

func TestExplicitDiscoveryResetsCooldownAfterRegistration(t *testing.T) {
	endpoint := createMockEndpoint("http://backend", "backend")
	registryErr := errors.New("registry unavailable")
	registry := &mockModelRegistry{registerError: registryErr}
	service := NewModelDiscoveryService(&mockDiscoveryClient{}, nil, registry, DiscoveryConfig{Timeout: time.Second}, createTestLogger())
	service.disableEndpoint(endpoint.URLString)
	if err := service.DiscoverEndpoint(context.Background(), endpoint); !errors.Is(err, registryErr) {
		t.Fatalf("expected registry error, got %v", err)
	}
	if !service.isEndpointDisabled(endpoint.URLString) {
		t.Fatal("failed registration cleared cooldown")
	}
	registry.registerError = nil
	if err := service.DiscoverEndpoint(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	if service.isEndpointDisabled(endpoint.URLString) || service.getFailureCount(endpoint.URLString) != 0 {
		t.Fatal("successful explicit discovery did not clear cooldown")
	}
}

func TestLlamaCppMetadataDriftRegistersModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"Qwen3.5-35B-A3B","object":"model","owned_by":"llamacpp","meta":{"vocab_type":true,"n_vocab":248320,"n_ctx_train":262144,"n_embd":2048,"n_params":35000000000,"size":21000000000,"ftype":"Q4_K_M"}}]}`))
	}))
	defer server.Close()
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())
	defer client.Close()
	endpoint := createTestEndpoint(server.URL, domain.ProfileLlamaCpp)
	registry := &mockModelRegistry{}
	service := NewModelDiscoveryService(client, nil, registry, DiscoveryConfig{Timeout: time.Second}, createTestLogger())
	if err := service.DiscoverEndpoint(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	if registry.registerCallCount != 1 || len(registry.registeredModels) != 1 || registry.registeredModels[0].Name != "Qwen3.5-35B-A3B" {
		t.Fatalf("model not registered: %+v", registry)
	}
	if service.getFailureCount(endpoint.URLString) != 0 || client.GetMetrics().SuccessfulRequests != 1 {
		t.Fatal("valid discovery recorded as failure")
	}
}

func TestConcurrentDiscoveryKeepsHealthyRequestRunning(t *testing.T) {
	failed := createMockEndpoint("http://failed", "failed")
	healthy := createMockEndpoint("http://healthy", "healthy")
	queued := createMockEndpoint("http://queued", "queued")
	healthyStarted := make(chan struct{})
	queuedStarted := make(chan struct{})
	client := &discoveryFuncClient{discover: func(ctx context.Context, ep *domain.Endpoint) ([]*domain.ModelInfo, error) {
		switch ep {
		case failed:
			<-healthyStarted
			return nil, errors.New("backend failed")
		case healthy:
			close(healthyStarted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-queuedStarted:
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		case queued:
			close(queuedStarted)
		}
		return []*domain.ModelInfo{{Name: ep.Name}}, nil
	}}
	registry := &mockModelRegistry{}
	service := NewModelDiscoveryService(client, &mockEndpointRepository{healthyEndpoints: []*domain.Endpoint{failed, healthy, queued}}, registry, DiscoveryConfig{Timeout: time.Second, ConcurrentWorkers: 2}, createTestLogger())
	if err := service.DiscoverAll(context.Background()); err == nil {
		t.Fatal("expected endpoint error")
	}
	if registry.registerCallCount != 2 || service.getFailureCount(healthy.URLString) != 0 {
		t.Fatalf("healthy concurrent requests cancelled: registered %d", registry.registerCallCount)
	}
}
