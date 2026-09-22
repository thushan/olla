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
