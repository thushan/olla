package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// countingHandler counts Handle calls, discards output, and respects a fixed min level.
// Used to assert whether log records are emitted without depending on stdout output.
type countingHandler struct {
	minLevel slog.Level
	count    int
}

func (h *countingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}
func (h *countingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.count++
	return nil
}
func (h *countingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(_ string) slog.Handler      { return h }

// setDefaultLogger replaces slog.Default for the duration of the test.
// NOT safe to call from t.Parallel() tests as slog.Default is process-global;
// these tests must run serially via subtests under a single parent.
func setDefaultLogger(t *testing.T, h slog.Handler) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
}

// TestEnhancedLogging_Gate exercises the level-gate logic in EnhancedLoggingMiddleware.
// All subtests share the same slog.Default mutation so they must run serially.
func TestEnhancedLogging_Gate(t *testing.T) {
	mockLogger := &mockStyledLogger{}
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// ProxyPath_InfoLevel: at info level, Debug records must not be emitted.
	// The responseWriter wrap and request-ID propagation must still occur.
	t.Run("ProxyPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		var gotRequestID string
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRequestID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		mw := EnhancedLoggingMiddleware(mockLogger)(inner)

		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		// The request-ID header must still be set - it is outside the log gate.
		if rr.Header().Get("X-Olla-Request-ID") == "" {
			t.Error("X-Olla-Request-ID header must be set even when debug logging is suppressed")
		}
		// The context request ID must still be propagated for downstream handlers.
		if gotRequestID == "" {
			t.Error("request ID must be in context even when debug logging is suppressed")
		}
		// "HTTP request started" and "HTTP request completed" are both Debug.
		// With minLevel=Info the gate must suppress both records.
		if ch.count != 0 {
			t.Errorf("expected 0 log records at info level for proxy path, got %d", ch.count)
		}
	})

	// ProxyPath_DebugLevel: at debug level, both records must be emitted.
	t.Run("ProxyPath_DebugLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelDebug}
		setDefaultLogger(t, ch)

		mw := EnhancedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		// Lower bound of 2: start + completion. Handler may emit more via slog.With.
		if ch.count < 2 {
			t.Errorf("expected at least 2 log records at debug level for proxy path, got %d", ch.count)
		}
	})

	// NonProxyPath_InfoLevel: ordinary (non-proxy, non-/internal/) requests log
	// at Info, which IS enabled at the default level - both "Request started"
	// and "Request completed" must appear.
	t.Run("NonProxyPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		mw := EnhancedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count < 2 {
			t.Errorf("expected at least 2 log records at info level for non-proxy path, got %d", ch.count)
		}
	})

	// InternalPollPath_InfoLevel: dashboard polls under /internal/ are quiet
	// traffic and log at Debug, so at the default Info level neither "Request
	// started" nor "Request completed" should appear - same treatment as the
	// proxy hot path, so an open dashboard tab doesn't flood the log.
	t.Run("InternalPollPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		mw := EnhancedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 0 {
			t.Errorf("expected 0 log records at info level for /internal/ poll path, got %d", ch.count)
		}
	})

	// InternalPollPath_DebugLevel: at debug level, /internal/ poll traffic is
	// still observable when an operator turns the verbosity up.
	t.Run("InternalPollPath_DebugLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelDebug}
		setDefaultLogger(t, ch)

		mw := EnhancedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count < 2 {
			t.Errorf("expected at least 2 log records at debug level for /internal/ poll path, got %d", ch.count)
		}
	})
}

// BenchmarkEnhancedLogging_ProxyPath_InfoLevel measures the hot-path overhead
// of EnhancedLoggingMiddleware at the default info level for proxy requests.
// At info level all Debug records are suppressed, so formatBytes, the []any field
// slices, and fmt.Sprintf must be skipped entirely.
//
// Run with:
//
//	go test -bench=BenchmarkEnhancedLogging_ProxyPath_InfoLevel -benchmem ./internal/app/middleware/
func BenchmarkEnhancedLogging_ProxyPath_InfoLevel(b *testing.B) {
	ch := &countingHandler{minLevel: slog.LevelInfo}
	prev := slog.Default()
	slog.SetDefault(slog.New(ch))
	b.Cleanup(func() { slog.SetDefault(prev) })

	mockLogger := &mockStyledLogger{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := EnhancedLoggingMiddleware(mockLogger)(inner)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}
}
