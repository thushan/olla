package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attrHandler keeps each record's message and attributes.
type attrHandler struct {
	mu   sync.Mutex
	recs map[string]map[string]any
}

func (h *attrHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *attrHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
	h.mu.Lock()
	if h.recs == nil {
		h.recs = map[string]map[string]any{}
	}
	h.recs[r.Message] = attrs
	h.mu.Unlock()
	return nil
}
func (h *attrHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *attrHandler) WithGroup(string) slog.Handler      { return h }

// get returns the attributes logged under msg; the console line is "Request
// completed" or its quiet variant depending on the route.
func (h *attrHandler) get(msg string) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if msg == "Request completed" && h.recs[msg] == nil {
		return h.recs["HTTP request completed"]
	}
	return h.recs[msg]
}

// A handler that aborts the connection (backend died mid-stream) must still leave
// its completion and access lines, flagged as aborted, and the abort panic must
// reach net/http untouched.
func TestCombinedLogging_AbortedRequestStillLogged(t *testing.T) {
	t.Parallel()
	h := &attrHandler{}
	log := &mockStyledLogger{underlying: slog.New(h)}
	mw := combinedLoggingMiddleware(log, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic(http.ErrAbortHandler)
	}))

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", nil))
	}()

	require.Equal(t, http.ErrAbortHandler, recovered, "the abort must not be swallowed")
	for _, msg := range []string{"Request completed", "Access log"} {
		attrs := h.get(msg)
		require.NotNil(t, attrs, msg)
		assert.Equal(t, true, attrs["aborted"], msg)
		assert.EqualValues(t, 7, attrs["response_bytes"], msg)
	}
}

func TestCombinedLogging_NormalRequestNotFlaggedAborted(t *testing.T) {
	t.Parallel()
	h := &attrHandler{}
	log := &mockStyledLogger{underlying: slog.New(h)}
	mw := combinedLoggingMiddleware(log, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", nil))

	for _, msg := range []string{"Request completed", "Access log"} {
		require.NotNil(t, h.get(msg), msg)
		assert.NotContains(t, h.get(msg), "aborted", msg)
	}
}
