package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/core/domain"
)

// resetBackend reads the whole request, then (optionally after sending a partial
// body) closes the connection with a TCP RST, the way a backend that crashes
// mid-request does. hits counts requests that reached it.
func resetBackend(t *testing.T, partial string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		if partial != "" {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(partial))
			w.(http.Flusher).Flush()
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetLinger(0)
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func healthyBackend(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("from-b"))
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

// TestRetryPolicy_BothEngines runs the real engines, built through the factory
// the way the server builds them, against a backend that resets the connection
// and a healthy second backend. It proves the proxy.retry policy reaches both
// engines and that a started response is never replayed.
func TestRetryPolicy_BothEngines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		policy      proxyconfig.RetryPolicy
		partial     string // bytes the failing backend sends before resetting
		wantHitsB   int32
		wantBody    string
		wantErr     bool
		wantDemoted bool
	}{
		{name: "defaults replay a POST after a pre-response reset", method: http.MethodPost, wantHitsB: 1, wantBody: "from-b", wantDemoted: true},
		{name: "retry disabled is a single attempt", method: http.MethodPost, policy: proxyconfig.RetryPolicy{Disabled: true}, wantErr: true, wantDemoted: true},
		{name: "max_attempts 1 is a single attempt", method: http.MethodGet, policy: proxyconfig.RetryPolicy{MaxAttempts: 1}, wantErr: true, wantDemoted: true},
		{name: "non_idempotent off does not replay a POST", method: http.MethodPost, policy: proxyconfig.RetryPolicy{NoNonIdempotent: true}, wantErr: true, wantDemoted: true},
		{name: "non_idempotent off still fails a GET over", method: http.MethodGet, policy: proxyconfig.RetryPolicy{NoNonIdempotent: true}, wantHitsB: 1, wantBody: "from-b", wantDemoted: true},
		{name: "GET is not replayed once the response started", method: http.MethodGet, partial: "partial", wantErr: true, wantBody: "partial"},
		{name: "POST is not replayed once the response started", method: http.MethodPost, partial: "partial", wantErr: true, wantBody: "partial"},
	}

	for _, engine := range []string{DefaultProxySherpa, DefaultProxyOlla} {
		for _, tt := range tests {
			t.Run(engine+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				srvA, hitsA := resetBackend(t, tt.partial)
				srvB, hitsB := healthyBackend(t)
				epA := createTestEndpoint("a", srvA.URL, domain.StatusHealthy)
				epB := createTestEndpoint("b", srvB.URL, domain.StatusHealthy)

				discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{epA, epB}}
				collector := createTestStatsCollector()
				selector := newMockEndpointSelector(collector)
				factory := NewFactory(collector, nil, createTestLogger())
				svc, err := factory.Create(engine, discovery, selector, &Configuration{Retry: tt.policy})
				require.NoError(t, err)

				req, stats, rlog := createTestRequestWithBody(tt.method, "/v1/chat/completions", `{"model":"m"}`)
				w := httptest.NewRecorder()
				err = svc.ProxyRequestToEndpoints(req.Context(), w, req, discovery.endpoints, stats, rlog)

				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, int32(1), hitsA.Load())
				assert.Equal(t, tt.wantHitsB, hitsB.Load(), "second backend hits")
				if tt.wantBody != "" {
					assert.Equal(t, tt.wantBody, w.Body.String())
				}
				// A mid-response failure is not a clean connection failure, so the
				// endpoint is only demoted when nothing had reached the client.
				demoted := discovery.endpoints[0].Status == domain.StatusOffline
				assert.Equal(t, tt.wantDemoted, demoted, "endpoint a demoted")
				assert.False(t, strings.Contains(w.Body.String(), "partial-from-b"), "no spliced response")
			})
		}
	}
}
