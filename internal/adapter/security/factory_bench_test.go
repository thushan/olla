package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkCreateChainMiddleware_ServeHTTP measures the hot path: the chain is
// built once (as it is in production, at route-wiring time) and ServeHTTP is
// then invoked per request. Before the fix, CreateChainMiddleware's returned
// handler rebuilt the rate-limit and size-validation middleware on every
// single call to ServeHTTP; this benchmark is what catches that regression.
func BenchmarkCreateChainMiddleware_ServeHTTP(b *testing.B) {
	cfg := createTestConfig()
	cfg.Server.RateLimits.GlobalRequestsPerMinute = 1_000_000
	cfg.Server.RateLimits.PerIPRequestsPerMinute = 1_000_000
	cfg.Server.RateLimits.BurstSize = 1_000_000
	_, adapters := createNewTestSecurityServicesWithConfig(cfg)
	defer adapters.Stop()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := adapters.CreateChainMiddleware()(next)

	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/test", nil)
	req.RemoteAddr = "10.0.0.1:54321"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
