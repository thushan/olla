package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// benchNextHandler is the terminal handler both benchmarks wrap - representative
// of a typical status/health handler, not the proxy hot path itself.
func benchNextHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// BenchmarkLoggingMiddleware_ChainedOld reproduces the pre-fix composition -
// AccessLoggingMiddleware wrapping EnhancedLoggingMiddleware - which built two
// responseWriter wrappers and read time.Now() twice per request.
func BenchmarkLoggingMiddleware_ChainedOld(b *testing.B) {
	mockLogger := &mockStyledLogger{}
	handler := AccessLoggingMiddleware(mockLogger)(EnhancedLoggingMiddleware(mockLogger)(benchNextHandler()))
	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkLoggingMiddleware_Combined measures the fused replacement now wired
// into application.go: one responseWriter wrap, one timer, feeding both the
// console and access log outputs.
func BenchmarkLoggingMiddleware_Combined(b *testing.B) {
	mockLogger := &mockStyledLogger{}
	handler := CombinedLoggingMiddleware(mockLogger)(benchNextHandler())
	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
