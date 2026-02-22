package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/core/domain"
)

// countingEndpointSelector tracks the number of Increment and Decrement calls
// using atomic counters so the test is safe under concurrent execution.
type countingEndpointSelector struct {
	incrementCalls atomic.Int64
	decrementCalls atomic.Int64
	endpoint       *domain.Endpoint
}

func (c *countingEndpointSelector) Select(_ context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if c.endpoint != nil {
		return c.endpoint, nil
	}
	if len(endpoints) > 0 {
		return endpoints[0], nil
	}
	return nil, nil
}

func (c *countingEndpointSelector) Name() string { return "counting" }

func (c *countingEndpointSelector) IncrementConnections(_ *domain.Endpoint) {
	c.incrementCalls.Add(1)
}

func (c *countingEndpointSelector) DecrementConnections(_ *domain.Endpoint) {
	c.decrementCalls.Add(1)
}

// TestOllaProxy_ConnectionCountingNoDuplication verifies that a single successful proxy
// attempt results in exactly one IncrementConnections call and one DecrementConnections
// call. Before the fix, proxyToSingleEndpoint also incremented/decremented, producing
// counts of two each.
func TestOllaProxy_ConnectionCountingNoDuplication(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test-endpoint", upstream.URL, domain.StatusHealthy)

	selector := &countingEndpointSelector{endpoint: endpoint}

	config := &olla.Configuration{}
	config.ResponseTimeout = 5 * time.Second
	config.ReadTimeout = 2 * time.Second
	config.StreamBufferSize = 8192
	config.MaxIdleConns = 10
	config.IdleConnTimeout = 30 * time.Second
	config.MaxConnsPerHost = 5

	proxy, err := olla.NewService(
		&mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}},
		selector,
		config,
		createTestStatsCollector(),
		nil,
		createTestLogger(),
	)
	if err != nil {
		t.Fatalf("failed to create Olla proxy: %v", err)
	}

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model":"test"}`)
	w := httptest.NewRecorder()

	if err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog); err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	if got := selector.incrementCalls.Load(); got != 1 {
		t.Errorf("IncrementConnections called %d times; want exactly 1", got)
	}
	if got := selector.decrementCalls.Load(); got != 1 {
		t.Errorf("DecrementConnections called %d times; want exactly 1", got)
	}
}

// TestOllaProxy_ConnectionCountReturnsToZero verifies that after a completed request
// the net connection delta is zero — i.e. every increment is paired with a decrement.
func TestOllaProxy_ConnectionCountReturnsToZero(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test-endpoint", upstream.URL, domain.StatusHealthy)
	selector := &countingEndpointSelector{endpoint: endpoint}

	config := &olla.Configuration{}
	config.ResponseTimeout = 5 * time.Second
	config.ReadTimeout = 2 * time.Second
	config.StreamBufferSize = 8192
	config.MaxIdleConns = 10
	config.IdleConnTimeout = 30 * time.Second
	config.MaxConnsPerHost = 5

	proxy, err := olla.NewService(
		&mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}},
		selector,
		config,
		createTestStatsCollector(),
		nil,
		createTestLogger(),
	)
	if err != nil {
		t.Fatalf("failed to create Olla proxy: %v", err)
	}

	const requests = 5
	for i := 0; i < requests; i++ {
		req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model":"test"}`)
		w := httptest.NewRecorder()
		if err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog); err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
	}

	inc := selector.incrementCalls.Load()
	dec := selector.decrementCalls.Load()

	if inc != requests {
		t.Errorf("IncrementConnections called %d times; want %d", inc, requests)
	}
	if dec != requests {
		t.Errorf("DecrementConnections called %d times; want %d", dec, requests)
	}
	if net := inc - dec; net != 0 {
		t.Errorf("net connection delta is %d after all requests completed; want 0", net)
	}
}
