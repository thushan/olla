package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// ---- helpers ----------------------------------------------------------------

func newTestRetryHandler(t *testing.T) *RetryHandler {
	t.Helper()
	logCfg := &logger.Config{Level: "error"}
	log, _, _ := logger.New(logCfg)
	return NewRetryHandler(&testDiscoveryService{}, logger.NewPlainStyledLogger(log))
}

// roundRobinSelector cycles through endpoints in order.
type roundRobinSelector struct{ idx int }

func (s *roundRobinSelector) Select(_ context.Context, eps []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(eps) == 0 {
		return nil, errors.New("no endpoints")
	}
	ep := eps[s.idx%len(eps)]
	s.idx++
	return ep, nil
}
func (s *roundRobinSelector) Name() string                            { return "round-robin" }
func (s *roundRobinSelector) IncrementConnections(_ *domain.Endpoint) {}
func (s *roundRobinSelector) DecrementConnections(_ *domain.Endpoint) {}

// namedEndpoint creates a minimal endpoint with the given name.
func namedEndpoint(name string) *domain.Endpoint {
	return &domain.Endpoint{Name: name, CheckTimeout: 0}
}

// connectionResetError satisfies net.Error so IsConnectionError returns true.
type connectionResetError struct{}

func (e *connectionResetError) Error() string   { return "connection reset by peer" }
func (e *connectionResetError) Timeout() bool   { return false }
func (e *connectionResetError) Temporary() bool { return false }

// Ensure it implements net.Error.
var _ net.Error = (*connectionResetError)(nil)

// ---- tests ------------------------------------------------------------------

// TestIsIdempotent confirms the idempotency predicate matches RFC 9110.
func TestIsIdempotent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodPost, false},
		{http.MethodPatch, false},
		{http.MethodDelete, false},
		{http.MethodPut, false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isIdempotent(tt.method))
		})
	}
}

// TestResponseStartedWriter_TracksWrites verifies that the sentinel wrapper sets
// started=true on both WriteHeader and Write calls.
func TestResponseStartedWriter_TracksWrites(t *testing.T) {
	t.Parallel()

	t.Run("WriteHeader sets started", func(t *testing.T) {
		t.Parallel()
		rw := &responseStartedWriter{ResponseWriter: httptest.NewRecorder()}
		assert.False(t, rw.started)
		rw.WriteHeader(http.StatusOK)
		assert.True(t, rw.started)
	})

	t.Run("Write sets started", func(t *testing.T) {
		t.Parallel()
		rw := &responseStartedWriter{ResponseWriter: httptest.NewRecorder()}
		assert.False(t, rw.started)
		_, _ = rw.Write([]byte("hello"))
		assert.True(t, rw.started)
	})

	t.Run("neither called — not started", func(t *testing.T) {
		t.Parallel()
		rw := &responseStartedWriter{ResponseWriter: httptest.NewRecorder()}
		assert.False(t, rw.started)
	})
}

// TestRetry_POSTWithBytesWritten_NoRetry is the critical correctness test.
// An httptest backend writes 100 bytes then RSTs. ExecuteWithRetry must NOT
// retry to a second endpoint because the response has already started.
func TestRetry_POSTWithBytesWritten_NoRetry(t *testing.T) {
	t.Parallel()

	attemptsHit := 0

	// Backend: write 100 bytes then close the connection abruptly.
	// We hijack the connection to send a TCP RST after the body begins.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 100))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hijack and close to simulate a mid-stream RST.
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	h := newTestRetryHandler(t)
	ep1 := namedEndpoint("ep1")
	ep2 := namedEndpoint("ep2")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attemptsHit++
		// Simulate: write the headers + bytes, then return a connection error.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 100))
		return &connectionResetError{}
	}

	err := h.ExecuteWithRetry(context.Background(), w, req, []*domain.Endpoint{ep1, ep2},
		&roundRobinSelector{}, stats, proxyFunc)

	// Error expected — the stream failed.
	require.Error(t, err)
	// Critically: only ONE attempt. Retrying would double-bill the user.
	assert.Equal(t, 1, attemptsHit, "must not retry POST after response bytes flushed to client")
}

// TestRetry_POSTBeforeBytesWritten_DoesRetry confirms that a connection error
// before any bytes are written still triggers failover, even for POST.
func TestRetry_POSTBeforeBytesWritten_DoesRetry(t *testing.T) {
	t.Parallel()

	attemptsHit := 0

	h := newTestRetryHandler(t)
	ep1 := namedEndpoint("ep1")
	ep2 := namedEndpoint("ep2")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attemptsHit++
		if attemptsHit == 1 {
			// First attempt: connection error before writing anything.
			return &connectionResetError{}
		}
		// Second attempt: success.
		w.WriteHeader(http.StatusOK)
		return nil
	}

	err := h.ExecuteWithRetry(context.Background(), w, req, []*domain.Endpoint{ep1, ep2},
		&roundRobinSelector{}, stats, proxyFunc)

	require.NoError(t, err)
	assert.Equal(t, 2, attemptsHit, "POST should retry when no bytes have been written yet")
}

// TestRetry_GETMidStreamRST_DoesRetry confirms that GET is always retried on
// connection errors, even after bytes have been written, because GET is idempotent.
func TestRetry_GETMidStreamRST_DoesRetry(t *testing.T) {
	t.Parallel()

	attemptsHit := 0

	h := newTestRetryHandler(t)
	ep1 := namedEndpoint("ep1")
	ep2 := namedEndpoint("ep2")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attemptsHit++
		if attemptsHit == 1 {
			// First attempt: write bytes then RST — simulates mid-stream failure.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial"))
			return &connectionResetError{}
		}
		// Second attempt: success.
		w.WriteHeader(http.StatusOK)
		return nil
	}

	err := h.ExecuteWithRetry(context.Background(), w, req, []*domain.Endpoint{ep1, ep2},
		&roundRobinSelector{}, stats, proxyFunc)

	require.NoError(t, err)
	assert.Equal(t, 2, attemptsHit, "GET should always retry on connection errors")
}

// TestResponseStartedWriter_Unwrap verifies that Flush works through the wrapper via
// http.NewResponseController. Without Unwrap(), the controller cannot reach the
// underlying flusher and SSE streams stall silently.
func TestResponseStartedWriter_Unwrap(t *testing.T) {
	t.Parallel()

	inner := httptest.NewRecorder()
	rw := &responseStartedWriter{ResponseWriter: inner}

	rc := http.NewResponseController(rw)
	if err := rc.Flush(); err != nil {
		t.Errorf("Flush() via ResponseController on wrapped writer = %v, want nil", err)
	}
}

// TestRetry_HTTPTestBackend_PostRSTBeforeBody uses a real httptest backend that
// refuses the connection before sending a response. We verify that ExecuteWithRetry
// does attempt a second endpoint (no bytes written).
func TestRetry_HTTPTestBackend_PostRSTBeforeBody(t *testing.T) {
	t.Parallel()

	// Backend 1: immediately close — simulates a refused connection.
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv1.Close)

	secondHit := false

	// Backend 2: healthy.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv2.Close)

	h := newTestRetryHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	ep1 := namedEndpoint("ep1")
	ep2 := namedEndpoint("ep2")

	// proxyFunc uses srv2 on the second attempt to prove failover.
	attempt := 0
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attempt++
		if attempt == 1 {
			// Simulate connection error from srv1 before any bytes written.
			return fmt.Errorf("connection refused: %w", &connectionResetError{})
		}
		// Forward to srv2.
		resp, err := http.Get(srv2.URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close() //nolint:errcheck
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return nil
	}

	err := h.ExecuteWithRetry(context.Background(), w, req, []*domain.Endpoint{ep1, ep2},
		&roundRobinSelector{}, stats, proxyFunc)

	require.NoError(t, err)
	assert.True(t, secondHit, "second endpoint must be tried after first fails without writing bytes")
}
