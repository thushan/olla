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

// TestCombinedLogging_Gate exercises the level-gate logic in
// CombinedLoggingMiddleware, which now does both the console log (gated by
// isQuietPollRoute/isQuietPollOutcome) AND the access log (gated by
// accessLogLevel/isQuietAccessOutcome) in one pass - so ch.count here is the
// sum of both outputs, not just the console line's. Each subtest's comment
// breaks down console vs access counts explicitly since the two gates
// deliberately disagree on proxy traffic (console always quiets it, access
// never does).
// All subtests share the same slog.Default mutation so they must run serially.
func TestCombinedLogging_Gate(t *testing.T) {
	mockLogger := &mockStyledLogger{}
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// ProxyPath_InfoLevel: at info level, console Debug records must not be
	// emitted (0), but the access log is NOT quieted for proxy traffic - a
	// proxy success is the audit-worthy outcome, so it logs at Info (1).
	t.Run("ProxyPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		var gotRequestID string
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRequestID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		mw := CombinedLoggingMiddleware(mockLogger)(inner)

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
		// Console started/completed are both Debug (suppressed at Info); access
		// log is Info (not suppressed) - exactly 1 record total.
		if ch.count != 1 {
			t.Errorf("expected 1 log record (access log only) at info level for proxy path, got %d", ch.count)
		}
	})

	// ProxyPath_DebugLevel: at debug level, both console records fire (2) plus
	// the access log (always Info for proxy, which is >= Debug) (1) = 3.
	t.Run("ProxyPath_DebugLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelDebug}
		setDefaultLogger(t, ch)

		mw := CombinedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		// Lower bound of 3: console start + console completion + access log.
		if ch.count < 3 {
			t.Errorf("expected at least 3 log records at debug level for proxy path, got %d", ch.count)
		}
	})

	// NonProxyPath_InfoLevel: ordinary (non-proxy, non-/internal/) requests log
	// at Info on the console (started + completed = 2) and the access log is
	// also loud here (1) = 3.
	t.Run("NonProxyPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		mw := CombinedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count < 3 {
			t.Errorf("expected at least 3 log records at info level for non-proxy path, got %d", ch.count)
		}
	})

	// InternalPollPath_InfoLevel: dashboard polls under /internal/ are quiet
	// traffic on BOTH gates when they succeed - console (started+completed)
	// and the access log all demote to Debug, so at the default Info level
	// nothing should appear (0).
	t.Run("InternalPollPath_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		mw := CombinedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 0 {
			t.Errorf("expected 0 log records at info level for /internal/ poll path, got %d", ch.count)
		}
	})

	// InternalPollPath_DebugLevel: at debug level, /internal/ poll traffic is
	// still observable when an operator turns the verbosity up - console
	// (2) + access log (1) = 3.
	t.Run("InternalPollPath_DebugLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelDebug}
		setDefaultLogger(t, ch)

		mw := CombinedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count < 3 {
			t.Errorf("expected at least 3 log records at debug level for /internal/ poll path, got %d", ch.count)
		}
	})

	// InternalPath_404_InfoLevel: a 404 under /internal/ must never be
	// swallowed just because it lives under the quiet-poll prefix, on either
	// gate. Console: only the completed line fires (started is still
	// optimistically quiet, status unknown yet) = 1. Access log: also loud on
	// a 404 = 1. Total 2.
	t.Run("InternalPath_404_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		mw := CombinedLoggingMiddleware(mockLogger)(notFound)

		req := httptest.NewRequest(http.MethodGet, "/internal/does-not-exist", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 2 {
			t.Errorf("expected 2 log records (console completed + access log) for a 404 under /internal/, got %d", ch.count)
		}
	})

	// InternalPath_500_InfoLevel: a 500 under /internal/ must log at Info on
	// both gates, not be hidden as routine polling traffic. Console completed
	// (1) + access log (1) = 2.
	t.Run("InternalPath_500_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		serverError := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		mw := CombinedLoggingMiddleware(mockLogger)(serverError)

		req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 2 {
			t.Errorf("expected 2 log records for a 500 under /internal/, got %d", ch.count)
		}
	})

	// InternalPath_POST_InfoLevel: a POST under /internal/ is never routine
	// GET/HEAD polling on either gate, so it must log normally even when it
	// succeeds. Console started+completed (2) + access log (1) = 3.
	t.Run("InternalPath_POST_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		mw := CombinedLoggingMiddleware(mockLogger)(noop)

		req := httptest.NewRequest(http.MethodPost, "/internal/status", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 3 {
			t.Errorf("expected 3 log records (started + completed + access log) for a POST under /internal/, got %d", ch.count)
		}
	})

	// InternalPath_403_InfoLevel: the dashboard's access-control gate
	// returning 403 (e.g. being probed by a scanner) must be visible at Info
	// on both gates. Console completed (1) + access log (1) = 2.
	t.Run("InternalPath_403_InfoLevel", func(t *testing.T) {
		ch := &countingHandler{minLevel: slog.LevelInfo}
		setDefaultLogger(t, ch)

		forbidden := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		mw := CombinedLoggingMiddleware(mockLogger)(forbidden)

		req := httptest.NewRequest(http.MethodGet, "/internal/ui/", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if ch.count != 2 {
			t.Errorf("expected 2 log records for a 403 under /internal/, got %d", ch.count)
		}
	})
}

// TestCombinedLogging_PathMutatingHandlerStaysQuiet pins the fix for the
// quiet-gate fail-open: handler_proxy.go's dispatchToEndpoints strips the
// route prefix by mutating r.URL.Path in place (r.URL.Path = pr.targetPath)
// before forwarding upstream, so the post-request console line and the
// access log must not re-read r.URL.Path after next.ServeHTTP returns - they
// would see the backend's target path instead of the original route and lose
// the quiet-poll classification entirely. CombinedLoggingMiddleware now
// captures path once, up front, and reuses it throughout.
//
// The stand-in handler mutates r.URL.Path to "/v1/chat/completions" - a
// vLLM-style target path with neither an "/olla/" substring nor an "/api/"
// prefix, so a regression here cannot hide behind Ollama's coincidentally
// "/api/"-prefixed target paths.
func TestCombinedLogging_PathMutatingHandlerStaysQuiet(t *testing.T) {
	ch := &countingHandler{minLevel: slog.LevelInfo}
	setDefaultLogger(t, ch)

	mockLogger := &mockStyledLogger{}
	pathMutating := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/v1/chat/completions"
		w.WriteHeader(http.StatusOK)
	})
	mw := CombinedLoggingMiddleware(mockLogger)(pathMutating)

	req := httptest.NewRequest(http.MethodPost, "/olla/vllm/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Same expectation as ProxyPath_InfoLevel above: console stays suppressed
	// (0), only the access log fires (1) = 1 total. Before the fix, the
	// mutated path made isQuietPollOutcome/accessLogLevel see "/v1/chat/completions"
	// instead of the original "/olla/vllm/..." route, so the console's
	// completed line stopped being quiet - inflating the count.
	if ch.count != 1 {
		t.Errorf("expected 1 log record (access log only) when the inner handler mutates r.URL.Path, got %d", ch.count)
	}
}

// BenchmarkCombinedLogging_ProxyPath_InfoLevel measures the hot-path overhead
// of CombinedLoggingMiddleware at the default info level for proxy requests.
// At info level the console's Debug records are suppressed, so formatBytes,
// the []any field slices, and fmt.Sprintf for those must be skipped entirely;
// the access log line still fires (proxy traffic is not quieted there).
//
// Run with:
//
//	go test -bench=BenchmarkCombinedLogging_ProxyPath_InfoLevel -benchmem ./internal/app/middleware/
func BenchmarkCombinedLogging_ProxyPath_InfoLevel(b *testing.B) {
	ch := &countingHandler{minLevel: slog.LevelInfo}
	prev := slog.Default()
	slog.SetDefault(slog.New(ch))
	b.Cleanup(func() { slog.SetDefault(prev) })

	mockLogger := &mockStyledLogger{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := CombinedLoggingMiddleware(mockLogger)(inner)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}
}
