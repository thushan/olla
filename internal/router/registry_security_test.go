package router

// Regression tests for Phase 3 security hardening item 3:
// Non-proxy routes must receive request-size validation when a sizeMiddlewareProvider
// is wired in, so that oversized bodies are rejected on status/health/stats endpoints.

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// capAdapters is a minimal security adapter that applies a 10-byte body cap on
// non-proxy routes (via CreateSizeMiddleware) and a pass-through chain on proxy
// routes (to keep the test focused on item 3 only).
type capAdapters struct{}

func (c *capAdapters) CreateChainMiddleware(security.RejectFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

func (c *capAdapters) CreateRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

func (c *capAdapters) CreateSizeMiddleware() func(http.Handler) http.Handler {
	const maxBytes = 10
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TestWireUpWithSecurityChain_NonProxyRouteGetsSizeValidation verifies that a
// non-proxy route rejects an oversized body when a sizeMiddlewareProvider is wired
// in. Without the fix, the route would receive no size check and return 200.
func TestWireUpWithSecurityChain_NonProxyRouteGetsSizeValidation(t *testing.T) {
	t.Parallel()

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	reg := NewRouteRegistry(&mockRouteLogger{})
	reg.RegisterWithMethod("/internal/status", handler, "status", http.MethodGet)

	mux := http.NewServeMux()
	reg.WireUpWithSecurityChain(mux, &capAdapters{})

	// Send a body larger than the 10-byte cap in capAdapters.
	body := strings.Repeat("x", 50)
	req := httptest.NewRequest(http.MethodGet, "/internal/status", strings.NewReader(body))
	req.ContentLength = int64(len(body))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d; size validation is not applied to non-proxy routes", rr.Code)
	}
	if reached {
		t.Error("handler was reached; request should have been rejected by size middleware")
	}
}

// TestWireUpWithSecurityChain_NonProxyRouteAllowsSmallBody verifies that a
// non-proxy route with a body under the size limit is still served normally.
func TestWireUpWithSecurityChain_NonProxyRouteAllowsSmallBody(t *testing.T) {
	t.Parallel()

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	reg := NewRouteRegistry(&mockRouteLogger{})
	reg.RegisterWithMethod("/internal/status", handler, "status", http.MethodGet)

	mux := http.NewServeMux()
	reg.WireUpWithSecurityChain(mux, &capAdapters{})

	// 5 bytes is within the 10-byte cap.
	req := httptest.NewRequest(http.MethodGet, "/internal/status", strings.NewReader("hello"))
	req.ContentLength = 5

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !reached {
		t.Error("handler was not reached; small request should pass size validation")
	}
}

// mockRouteLogger satisfies logger.StyledLogger for the RouteRegistry.
type mockRouteLogger struct{}

func (m *mockRouteLogger) Debug(msg string, args ...any)                                {}
func (m *mockRouteLogger) Info(msg string, args ...any)                                 {}
func (m *mockRouteLogger) Warn(msg string, args ...any)                                 {}
func (m *mockRouteLogger) Error(msg string, args ...any)                                {}
func (m *mockRouteLogger) ResetLine()                                                   {}
func (m *mockRouteLogger) InfoWithStatus(msg string, status string, args ...any)        {}
func (m *mockRouteLogger) InfoWithCount(msg string, count int, args ...any)             {}
func (m *mockRouteLogger) InfoWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockRouteLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {}
func (m *mockRouteLogger) InfoWithNumbers(msg string, numbers ...int64)                 {}
func (m *mockRouteLogger) WarnWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockRouteLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any)   {}
func (m *mockRouteLogger) InfoHealthy(msg string, endpoint string, args ...any)         {}
func (m *mockRouteLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
}
func (m *mockRouteLogger) GetUnderlying() *slog.Logger                                         { return slog.Default() }
func (m *mockRouteLogger) WithRequestID(requestID string) logger.StyledLogger                  { return m }
func (m *mockRouteLogger) InfoConfigChange(oldName, newName string)                            {}
func (m *mockRouteLogger) WithAttrs(attrs ...slog.Attr) logger.StyledLogger                    { return m }
func (m *mockRouteLogger) With(args ...any) logger.StyledLogger                                { return m }
func (m *mockRouteLogger) InfoWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockRouteLogger) WarnWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockRouteLogger) ErrorWithContext(msg string, endpoint string, ctx logger.LogContext) {}

// chainOnlyAdapters rejects everything on the proxy chain and is the only thing that can
// produce the 429, so the test can tell whether a route took the chain.
type chainOnlyAdapters struct{ capAdapters }

func (chainOnlyAdapters) CreateChainMiddleware(security.RejectFunc) func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		})
	}
}

// rejectAdapters adds the optional translator chain, which writes through the route's Reject.
type rejectAdapters struct{ chainOnlyAdapters }

func (rejectAdapters) CreateTranslatorChainMiddleware(reject security.RejectFunc, _ int64) func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reject(w, http.StatusTooManyRequests, "slow down")
		})
	}
}

// Translator routes are inference entry points: they take the proxy chain, not the
// observability-only wiring, and use the route's own rejection writer when the adapters
// offer one.
func TestWireUpWithSecurityChain_TranslatorRouteTakesProxyChain(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler reached through a rejecting chain")
	})
	statusHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	reject := func(w http.ResponseWriter, status int, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"custom":"` + message + `"}`))
	}

	tests := []struct {
		name     string
		adapters any
		wantBody string
	}{
		{"adapters without a reject chain use the plain proxy chain", &chainOnlyAdapters{}, ""},
		{"adapters with a reject chain write through the route's Reject", &rejectAdapters{}, `{"custom":"slow down"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := NewRouteRegistry(&mockRouteLogger{})
			reg.RegisterTranslatorRoute("/olla/x/v1/messages", handler, "x", http.MethodPost, reject, 0)
			reg.Register("/internal/status", statusHandler, "status")

			mux := http.NewServeMux()
			reg.WireUpWithSecurityChain(mux, tt.adapters)

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/olla/x/v1/messages", nil))
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("translator route skipped the proxy chain: got %d", rr.Code)
			}
			if rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}

			// Plain observability routes stay outside the rate limit.
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/internal/status", nil))
			if rr.Code == http.StatusTooManyRequests {
				t.Error("non-proxy route was rate limited")
			}
		})
	}
}

// limitRecordingAdapters captures the body limit the registry hands the translator chain.
type limitRecordingAdapters struct {
	chainOnlyAdapters
	got *int64
}

func (a limitRecordingAdapters) CreateTranslatorChainMiddleware(_ security.RejectFunc, bodyLimit int64) func(http.Handler) http.Handler {
	*a.got = bodyLimit
	return func(next http.Handler) http.Handler { return next }
}

// The translator's own cap travels with the route to the size middleware, so a chunked body
// is bounded by it at wiring time rather than by the server-wide max_body_size.
func TestWireUpWithSecurityChain_TranslatorRoutePassesBodyLimit(t *testing.T) {
	t.Parallel()

	var got int64
	reg := NewRouteRegistry(&mockRouteLogger{})
	reg.RegisterTranslatorRoute("/olla/x/v1/messages", func(http.ResponseWriter, *http.Request) {}, "x", http.MethodPost, nil, 12345)

	reg.WireUpWithSecurityChain(http.NewServeMux(), &limitRecordingAdapters{got: &got})
	if got != 12345 {
		t.Errorf("translator chain got body limit %d, want 12345", got)
	}
}
