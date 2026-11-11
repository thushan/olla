package olla

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func newCancelTestService(t *testing.T, mutate func(*Configuration)) *Service {
	t.Helper()
	config := &Configuration{}
	config.StreamBufferSize = 8192
	if mutate != nil {
		mutate(config)
	}
	svc, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	require.NoError(t, err)
	t.Cleanup(svc.Cleanup)
	return svc
}

func endpointFor(t *testing.T, rawURL string) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{Name: "ep", URL: u, URLString: rawURL, Status: domain.StatusHealthy}
}

func proxyOnce(ctx context.Context, svc *Service, ep *domain.Endpoint) error {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	return svc.ProxyRequestToEndpoints(ctx, rec, req, []*domain.Endpoint{ep}, &ports.RequestStats{StartTime: time.Now()}, createTestLogger())
}

func hangingBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Genuine transport failures must keep opening the breaker, otherwise the
// cancel fix has simply switched failure accounting off.
func TestBackendRoundTripFailures_StillOpenBreaker(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	ep := endpointFor(t, dead.URL)
	dead.Close() // connection refused from here on

	svc := newCancelTestService(t, nil)
	for range circuitBreakerThreshold {
		require.Error(t, proxyOnce(context.Background(), svc, ep))
	}

	cb := svc.GetCircuitBreaker(ep.Name)
	require.True(t, cb.Tripped(), "genuine round-trip failures no longer open the breaker (failures=%d)", cb.Failures())
	require.Positive(t, svc.GetProxyStats().FailedRequests)
}

// A response-header timeout is the backend being slow, with the client still
// waiting, so it must count even though it surfaces as a timeout error.
func TestResponseHeaderTimeout_StillCountsAgainstBreaker(t *testing.T) {
	srv := hangingBackend(t)
	ep := endpointFor(t, srv.URL)

	svc := newCancelTestService(t, func(c *Configuration) {
		c.ResponseHeaderTimeout = 50 * time.Millisecond
	})
	for range circuitBreakerThreshold {
		require.Error(t, proxyOnce(context.Background(), svc, ep))
	}

	cb := svc.GetCircuitBreaker(ep.Name)
	require.True(t, cb.Tripped(), "response-header timeouts did not open the breaker (failures=%d)", cb.Failures())
}

// Client cancels are not request failures in either the proxy counters or the
// error events, so dashboards do not show a healthy backend as erroring.
func TestClientCancel_DoesNotCountAsFailedRequest(t *testing.T) {
	srv := hangingBackend(t)
	ep := endpointFor(t, srv.URL)
	svc := newCancelTestService(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_ = proxyOnce(ctx, svc, ep)
	cancel()

	require.Zero(t, svc.GetProxyStats().FailedRequests, "client cancel counted as a failed request")
}

// The cancelled probe must free the half-open slot: the very next caller is
// admitted as the new probe rather than waiting out the staleness window.
func TestClientCancelOnHalfOpenProbe_ReleasesProbeSlot(t *testing.T) {
	srv := hangingBackend(t)
	ep := endpointFor(t, srv.URL)
	svc := newCancelTestService(t, nil)

	cb := svc.GetCircuitBreaker(ep.Name)
	for range circuitBreakerThreshold {
		cb.RecordFailure(0)
	}
	cb.SetLastFailureNanos(time.Now().Add(-time.Minute).UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_ = proxyOnce(ctx, svc, ep)
	cancel()

	require.Zero(t, cb.LastAttemptNanos(), "cancelled probe left the half-open slot held")
	open, attempt := cb.IsOpen(HalfOpenStaleness)
	require.False(t, open, "next probe rejected after a cancelled probe")
	require.NotZero(t, attempt)
}
