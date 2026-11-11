package olla

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Issue #269: a request the client cancels before the first byte must not count
// against the backend's circuit breaker.
func TestClientCancelBeforeFirstByte_DoesNotTripBreaker(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fast") == "1" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	defer once.Do(func() { close(release) })

	u, err := url.Parse(slow.URL)
	require.NoError(t, err)
	ep := &domain.Endpoint{Name: "slow", URL: u, URLString: slow.URL, Status: domain.StatusHealthy}

	config := &Configuration{}
	config.StreamBufferSize = 8192
	svc, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	require.NoError(t, err)
	defer svc.Cleanup()

	do := func(ctx context.Context, path string) (int, error) {
		req := httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		stats := &ports.RequestStats{StartTime: time.Now()}
		err := svc.ProxyRequestToEndpoints(ctx, rec, req, []*domain.Endpoint{ep}, stats, createTestLogger())
		return rec.Code, err
	}

	const cancelled = 6 // breaker threshold is 5
	for i := 0; i < cancelled; i++ {
		// WithCancel, not WithTimeout: a client hanging up surfaces as
		// context.Canceled, which is what curl -m produces server-side.
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		_, _ = do(ctx, "/v1/chat/completions")
		cancel()
	}

	cb := svc.GetCircuitBreaker(ep.Name)
	require.False(t, cb.Tripped(), "breaker open after %d client cancels (failures=%d)", cancelled, cb.Failures())
	require.Zero(t, cb.Failures(), "client cancels were recorded as backend failures")

	code, err := do(context.Background(), "/v1/chat/completions?fast=1")
	require.NoError(t, err, "healthy request rejected after client cancels")
	require.Equal(t, http.StatusOK, code)
}

// A client cancel on the half-open probe must not re-open the breaker: the
// probe never learned anything about the backend.
func TestClientCancelOnHalfOpenProbe_DoesNotReopenBreaker(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()
	u, err := url.Parse(slow.URL)
	require.NoError(t, err)
	ep := &domain.Endpoint{Name: "slow", URL: u, URLString: slow.URL, Status: domain.StatusHealthy}

	config := &Configuration{}
	config.StreamBufferSize = 8192
	svc, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	require.NoError(t, err)
	defer svc.Cleanup()

	// Trip on genuine failures, then backdate so the next call is the half-open probe.
	cb := svc.GetCircuitBreaker(ep.Name)
	for i := 0; i < circuitBreakerThreshold; i++ {
		cb.RecordFailure(0)
	}
	require.True(t, cb.Tripped())
	cb.SetLastFailureNanos(time.Now().Add(-time.Minute).UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	_, _ = func() (int, error) {
		rec := httptest.NewRecorder()
		err := svc.ProxyRequestToEndpoints(ctx, rec, req, []*domain.Endpoint{ep}, &ports.RequestStats{StartTime: time.Now()}, createTestLogger())
		return rec.Code, err
	}()

	// Probe was cancelled: the failure count must be unchanged and the gate
	// must not be left re-opened by the client's own hang-up.
	require.Equal(t, int64(circuitBreakerThreshold), cb.Failures(), "client cancel counted as a probe failure")
}
