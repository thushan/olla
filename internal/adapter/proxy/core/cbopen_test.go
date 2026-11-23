package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// These tests pin the v0.0.29 CB-open failover fix (spec
// v0.0.29-release-prep.md, FR-1..FR-5 / AC-1..AC-5). They exercise
// ExecuteWithRetry's CB-open branch directly, mirroring how
// olla/service_retry.go wraps the sentinel for a named endpoint.

// ---- helpers ----------------------------------------------------------------

// newCapturingRetryHandler wires a RetryHandler to a caller-held
// testDiscoveryService so a test can assert on whether any failure path
// reached UpdateEndpointStatus. That capture is the state-effect seam: if the
// CB-open path ever routed through markEndpointUnhealthy, the discovery
// service would record a demoted endpoint here.
func newCapturingRetryHandler(t *testing.T, ds *testDiscoveryService) *RetryHandler {
	t.Helper()
	logCfg := &logger.Config{Level: "error"}
	log, _, _ := logger.New(logCfg)
	return NewRetryHandler(ds, logger.NewPlainStyledLogger(log))
}

// cbOpenErr mirrors the wrapping shape service_retry.go produces, so the retry
// loop sees the real error chain (sentinel + endpoint name) it branches on.
func cbOpenErr(name string) error {
	return fmt.Errorf("%w: endpoint %s", ErrCircuitBreakerOpen, name)
}

// cbTimeoutNetError is a net.Error whose Timeout() reports true, used to prove
// IsConnectionError still classifies genuine net timeouts as connection errors
// after the CB-open branch was added. Distinct name avoids colliding with net
// error stubs other test files in this package may define.
type cbTimeoutNetError struct{}

func (e *cbTimeoutNetError) Error() string   { return "i/o timeout" }
func (e *cbTimeoutNetError) Timeout() bool   { return true }
func (e *cbTimeoutNetError) Temporary() bool { return false }

var _ net.Error = (*cbTimeoutNetError)(nil)

// ---- AC-1 (FR-1, FR-2): failover to a healthy peer, no health demotion -----

// TestExecuteWithRetry_CBOpen_FailoverWithoutDemotion proves a CB-open
// rejection on endpoint A fails over to endpoint B and the request is served
// by B, while A's persisted health is left untouched. The state-effect
// assertion (ds.updatedEndpoint stays nil) is the load-bearing check: it would
// fail the moment the CB-open branch regressed into markEndpointUnhealthy.
func TestExecuteWithRetry_CBOpen_FailoverWithoutDemotion(t *testing.T) {
	t.Parallel()

	ds := &testDiscoveryService{}
	h := newCapturingRetryHandler(t, ds)

	endpointA := namedEndpoint("endpoint-a")
	endpointB := namedEndpoint("endpoint-b")

	// attempts records dispatch order; the slice is only mutated by the single
	// goroutine ExecuteWithRetry drives, so no extra sync is needed.
	var attempts []string
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attempts = append(attempts, ep.Name)
		if ep.Name == "endpoint-a" {
			// CB-open fires before any network I/O: no bytes written, sentinel
			// returned, exactly as service_retry.go's IsOpen branch does.
			return cbOpenErr(ep.Name)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served-by-endpoint-b"))
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	err := h.ExecuteWithRetry(t.Context(), w, req,
		[]*domain.Endpoint{endpointA, endpointB},
		&roundRobinSelector{}, stats, proxyFunc)

	require.NoError(t, err, "CB-open on A must fail over to B and succeed")
	require.Len(t, attempts, 2, "both endpoints must be attempted in order")
	assert.Equal(t, []string{"endpoint-a", "endpoint-b"}, attempts)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "served-by-endpoint-b", w.Body.String(), "response must be served by B")

	// State-effect assertion: the CB-open path removes A from the candidate
	// list only. If it had demoted A, UpdateEndpointStatus would capture it.
	assert.Nil(t, ds.updatedEndpoint,
		"CB-open must not mutate persisted endpoint health")
}

// ---- AC-2 (FR-2, FR-4): total exhaustion yields a clean error, no demotion --

// TestExecuteWithRetry_AllCBOpen_CleanExhaustionError proves that when every
// candidate rejects with CB-open, the surfaced error wraps the sentinel, names
// breaker state (not a phantom connection failure), and demotes nothing.
func TestExecuteWithRetry_AllCBOpen_CleanExhaustionError(t *testing.T) {
	t.Parallel()

	ds := &testDiscoveryService{}
	h := newCapturingRetryHandler(t, ds)

	endpoints := []*domain.Endpoint{
		namedEndpoint("ep-1"),
		namedEndpoint("ep-2"),
		namedEndpoint("ep-3"),
	}

	var attempts int
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attempts++
		return cbOpenErr(ep.Name)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	err := h.ExecuteWithRetry(t.Context(), w, req, endpoints,
		&roundRobinSelector{}, stats, proxyFunc)

	require.Error(t, err, "total CB-open exhaustion must surface an error")
	assert.ErrorIs(t, err, ErrCircuitBreakerOpen,
		"exhaustion error must unwrap to the CB-open sentinel")

	msg := err.Error()
	assert.Contains(t, msg, "circuit breaker open",
		"message should name breaker state as the cause")
	assert.NotContains(t, msg, "connection errors",
		"must not mislabel breaker exhaustion as a connection failure")

	assert.Equal(t, len(endpoints), attempts, "every candidate must have been tried exactly once")
	assert.Nil(t, ds.updatedEndpoint,
		"no endpoint may be health-demoted on CB-open exhaustion")
}

// ---- AC-3 (existing guard, regression): POST + started holds on CB-open ----

// TestExecuteWithRetry_CBOpen_POSTResponseStarted_NoRetry proves the
// non-idempotent "response already started" guard holds on the CB-open path:
// once bytes have reached the client on a POST, no further endpoint is tried
// even though CB-open is otherwise failover-eligible, and the partial response
// stands. This mirrors TestRetry_POSTWithBytesWritten_NoRetry but exercises the
// CB-open branch instead of the connection-error branch.
func TestExecuteWithRetry_CBOpen_POSTResponseStarted_NoRetry(t *testing.T) {
	t.Parallel()

	ds := &testDiscoveryService{}
	h := newCapturingRetryHandler(t, ds)

	endpointA := namedEndpoint("endpoint-a")
	endpointB := namedEndpoint("endpoint-b")

	var attempts int
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attempts++
		if ep.Name == "endpoint-a" {
			// Commit bytes to the client, then surface a CB-open error. In
			// production the breaker check precedes I/O so this exact ordering
			// can't arise from service_retry.go; we force it here to prove the
			// response-started guard is evaluated independently of which
			// failover branch produced the error.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial-from-a"))
			return cbOpenErr(ep.Name)
		}
		// B would succeed; reaching it would mean the guard failed.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served-by-endpoint-b"))
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stats := &ports.RequestStats{}

	err := h.ExecuteWithRetry(t.Context(), w, req,
		[]*domain.Endpoint{endpointA, endpointB},
		&roundRobinSelector{}, stats, proxyFunc)

	require.Error(t, err, "guard returns the error rather than retrying")
	assert.ErrorIs(t, err, ErrCircuitBreakerOpen)
	assert.Equal(t, 1, attempts,
		"must not attempt B once a POST response has started")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "partial-from-a", w.Body.String(),
		"the partial response committed on attempt 1 must stand")
	assert.Nil(t, ds.updatedEndpoint, "guard path must not demote health")
}

// ---- AC-4 (FR-3, regression guard): IsConnectionError guardrail ------------

// TestIsConnectionError_CircuitBreakerGuardrail pins the design guardrail that
// keeps CB-open handling off the connection-error path. The sentinel, bare or
// wrapped, must never satisfy IsConnectionError: if it did, the error would
// flow into handleConnectionFailure and 1-strike-demote persisted health,
// double-counting breaker state as a connectivity failure. Existing true and
// false cases are re-asserted so a future change to the type matching or string
// fallback list can't sneak CB-open through this gate silently.
func TestIsConnectionError_CircuitBreakerGuardrail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		// CB-open sentinel under every wrapping shape seen in production.
		{"bare ErrCircuitBreakerOpen", ErrCircuitBreakerOpen, false},
		{"wrapped ErrCircuitBreakerOpen (olla shape)", cbOpenErr("endpoint-x"), false},
		{"doubly wrapped ErrCircuitBreakerOpen", fmt.Errorf("dispatch: %w", cbOpenErr("endpoint-x")), false},

		// Genuine connection errors must remain true.
		{"net.Error timeout", &cbTimeoutNetError{}, true},
		{"net.Error connection reset", &connectionResetError{}, true},
		{"connection refused string fallback", errors.New("dial tcp: connection refused"), true},
		{"syscall.ECONNREFUSED", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), true},

		// Client-side context errors must remain false (endpoint is innocent).
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"context.Canceled", context.Canceled, false},

		// Unrelated error baseline.
		{"generic error", errors.New("internal server error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsConnectionError(tt.err)
			assert.Equal(t, tt.want, got, "IsConnectionError(%v)", tt.err)
		})
	}
}
