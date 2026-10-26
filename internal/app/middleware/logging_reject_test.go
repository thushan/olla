package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/logger"
)

// recordHandler keeps every record so a test can count lines by message.
type recordHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.msgs = append(h.msgs, r.Message)
	h.mu.Unlock()
	return nil
}
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordHandler) count(msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, m := range h.msgs {
		if m == msg {
			n++
		}
	}
	return n
}

type warnCapture struct {
	*mockStyledLogger
	mu    sync.Mutex
	warns []string
}

func (w *warnCapture) Warn(msg string, _ ...any) {
	w.mu.Lock()
	w.warns = append(w.warns, msg)
	w.mu.Unlock()
}

func TestCombinedLogging_RejectionsAreSampledPerClient(t *testing.T) {
	t.Parallel()

	status := http.StatusForbidden
	h := &recordHandler{}
	log := &warnCapture{mockStyledLogger: &mockStyledLogger{underlying: slog.New(h)}}
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	mw := combinedLoggingMiddleware(log, clock, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))

	serve := func(path, remote string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = remote
		mw.ServeHTTP(httptest.NewRecorder(), req)
	}

	// a blocked dashboard client hammering one route
	for range 500 {
		serve("/internal/ui/", "203.0.113.7:5555")
	}
	assert.Equal(t, 1, h.count("Request completed"), "one console line per window")
	assert.Equal(t, 1, h.count("Access log"), "one access line per window")

	// another client is not hidden by the first one's flood
	serve("/internal/ui/", "198.51.100.9:1234")
	assert.Equal(t, 2, h.count("Access log"))

	// the window rolls: the tail is reported once and logging resumes
	now = now.Add(11 * time.Second)
	serve("/internal/ui/", "203.0.113.7:5555")
	assert.Equal(t, 3, h.count("Access log"))
	require.Equal(t, []string{"Rejected request, repeats suppressed"}, log.warns)
}

func TestCombinedLogging_OrdinaryTrafficIsNeverSampled(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusBadGateway, http.StatusBadRequest} {
		h := &recordHandler{}
		log := &warnCapture{mockStyledLogger: &mockStyledLogger{underlying: slog.New(h)}}
		mw := combinedLoggingMiddleware(log, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		for range 20 {
			req := httptest.NewRequest(http.MethodGet, "/version", nil)
			req.RemoteAddr = "203.0.113.7:5555"
			mw.ServeHTTP(httptest.NewRecorder(), req)
		}
		assert.Equal(t, 20, h.count("Access log"), "status %d must keep one access line per request", status)
		assert.Equal(t, 20, h.count("Request completed"), "status %d", status)
	}
}

// The tail of a flood that stops before shutdown is reported by the owner's set.
func TestNewCombinedLoggingMiddleware_FlushReportsPendingSummary(t *testing.T) {
	t.Parallel()

	h := &recordHandler{}
	log := &warnCapture{mockStyledLogger: &mockStyledLogger{underlying: slog.New(h)}}
	set := &logger.ThrottleSet{}
	mw := NewCombinedLoggingMiddleware(log, set)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/internal/ui/", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		mw.ServeHTTP(httptest.NewRecorder(), req)
	}
	require.Empty(t, log.warns, "nothing reported while the window is open")

	set.Flush()
	require.Equal(t, []string{"Rejected request, repeats suppressed"}, log.warns)
}

// Rotating X-Forwarded-For must not mint a fresh sampling key per request: this layer
// has no trusted-proxy configuration, so the TCP peer is the key.
func TestCombinedLogging_RejectionKeyIgnoresForwardedHeaders(t *testing.T) {
	t.Parallel()

	h := &recordHandler{}
	log := &warnCapture{mockStyledLogger: &mockStyledLogger{underlying: slog.New(h)}}
	mw := combinedLoggingMiddleware(log, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	for i := range 50 {
		req := httptest.NewRequest(http.MethodGet, "/internal/ui/", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		mw.ServeHTTP(httptest.NewRecorder(), req)
	}
	assert.Equal(t, 1, h.count("Access log"))
}

// A backend's own 401/403/429 is forwarded with X-Olla-Endpoint and is the only per-request
// record of what the backend said, so it must never be sampled.
func TestCombinedLogging_BackendRejectionStatusesAreNeverSampled(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		h := &recordHandler{}
		log := &warnCapture{mockStyledLogger: &mockStyledLogger{underlying: slog.New(h)}}
		mw := combinedLoggingMiddleware(log, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(constants.HeaderXOllaEndpoint, "backend-1")
			w.WriteHeader(status)
		}))
		for range 20 {
			req := httptest.NewRequest(http.MethodGet, "/version", nil)
			req.RemoteAddr = "203.0.113.7:5555"
			mw.ServeHTTP(httptest.NewRecorder(), req)
		}
		assert.Equal(t, 20, h.count("Access log"), "backend status %d must keep one access line per request", status)
		assert.Equal(t, 20, h.count("Request completed"), "backend status %d", status)
	}
}
