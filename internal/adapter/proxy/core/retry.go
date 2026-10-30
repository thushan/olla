package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// RetryHandler manages connection failure recovery and endpoint failover
type RetryHandler struct {
	logger           logger.StyledLogger
	discoveryService ports.DiscoveryService
	policy           proxyconfig.RetryPolicy
}

// RetryOption customises a RetryHandler at construction.
type RetryOption func(*RetryHandler)

// WithRetryPolicy sets the failover policy. Without it the handler fails over
// to every eligible endpoint once and replays any method after a connection failure.
func WithRetryPolicy(p proxyconfig.RetryPolicy) RetryOption {
	return func(h *RetryHandler) { h.policy = p }
}

// NewRetryHandler creates a new retry handler
func NewRetryHandler(discoveryService ports.DiscoveryService, logger logger.StyledLogger, opts ...RetryOption) *RetryHandler {
	h := &RetryHandler{
		discoveryService: discoveryService,
		logger:           logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// maxAttempts is the per-request attempt budget for the given candidate count.
func (h *RetryHandler) maxAttempts(candidates int) int {
	switch {
	case h.policy.Disabled:
		return 1
	case h.policy.MaxAttempts > 0:
		return min(candidates, h.policy.MaxAttempts)
	default:
		return candidates
	}
}

// ProxyFunc defines the signature for endpoint proxy implementations
type ProxyFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error

// ResponseStartedWriter wraps http.ResponseWriter and records whether any response
// bytes or status codes have been sent to the client. Retry logic uses this to gate
// retry decisions: once the client has received data, retrying a non-idempotent
// request would send duplicate content or charge twice on metered APIs. It is also
// the correct "has this response been committed" signal for callers upstream of
// retry (e.g. handler_proxy.go's handleProxyError) - Header().Set does not commit
// anything, only WriteHeader/Write do, so a Content-Type-presence check is not a
// reliable substitute.
//
// A flush is also a commit: streaming code can in principle reach a bare Flush()
// (via http.NewResponseController) without a prior Write on this wrapper - bytes
// already buffered by the underlying writer would then reach the client while
// started stays false. Flush/FlushError guard against that by marking started
// before delegating, so the signal holds regardless of call order.
type ResponseStartedWriter struct {
	http.ResponseWriter
	started bool
	status  int
}

// NewResponseStartedWriter wraps w so callers can observe whether a response has
// been committed via Started().
func NewResponseStartedWriter(w http.ResponseWriter) *ResponseStartedWriter {
	return &ResponseStartedWriter{ResponseWriter: w}
}

func (rw *ResponseStartedWriter) WriteHeader(code int) {
	rw.started = true
	// 1xx interim responses are not the final status the client sees.
	if rw.status == 0 && code >= http.StatusOK {
		rw.status = code
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseStartedWriter) Write(b []byte) (int, error) {
	rw.started = true
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Started reports whether a status code or body bytes have been committed to the
// client yet.
func (rw *ResponseStartedWriter) Started() bool {
	return rw.started
}

// Status is the final status code sent to the client: 0 until a response is committed,
// and 200 when body bytes were written without an explicit WriteHeader.
func (rw *ResponseStartedWriter) Status() int {
	return rw.status
}

// Unwrap exposes the underlying ResponseWriter so http.NewResponseController can
// discover optional interfaces (Flush, Hijack, SetDeadline, etc.) via the chain.
// Without this, the wrapper hides the underlying flusher and SSE streams stall.
func (rw *ResponseStartedWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// FlushError marks the response as started and flushes the underlying writer,
// mirroring http.ResponseController's FlushError semantics. It returns
// http.ErrNotSupported if nothing in the underlying chain supports flushing,
// rather than panicking or swallowing the failure.
//
// started is set before the flush is attempted: a flush pushes any buffered
// bytes to the client immediately, so the commit has already happened by the
// time this call is made, even if the underlying flush itself then fails.
func (rw *ResponseStartedWriter) FlushError() error {
	wasStarted, wasStatus := rw.started, rw.status
	rw.started = true
	// A bare flush commits an implicit 200, same as a body write.
	if rw.status == 0 {
		rw.status = http.StatusOK
	}

	rc := http.NewResponseController(rw.ResponseWriter)
	if err := rc.Flush(); err != nil {
		// ErrNotSupported means nothing in the chain could flush at all, so
		// nothing was committed - restore the prior state. Any other error
		// (dead connection, etc.) may have partially written bytes before
		// failing, so started stays true.
		if errors.Is(err, http.ErrNotSupported) {
			rw.started, rw.status = wasStarted, wasStatus
		}
		return err
	}
	return nil
}

// Flush satisfies the plain http.Flusher interface for callers that still
// type-assert for it directly. Like http.Flusher.Flush, it is best-effort and
// discards any error - use FlushError to observe failures.
func (rw *ResponseStartedWriter) Flush() {
	_ = rw.FlushError()
}

// isIdempotent reports whether the HTTP method is safe to replay on another endpoint
// after a connection failure. GET, HEAD and OPTIONS are defined as idempotent by
// RFC 9110; POST, PATCH and DELETE are not. Replaying them risks double-billing or
// duplicate side effects if the first backend already read and acted on the request.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// ExecuteWithRetry attempts request delivery with automatic failover on connection errors.
// Retry is always suppressed once any part of the response has reached the client,
// whatever the method: a second endpoint's reply would be appended to the first's
// partial output. The attempt budget and whether non-idempotent methods may be
// replayed after a connection failure come from the handler's RetryPolicy.
func (h *RetryHandler) ExecuteWithRetry(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	selector domain.EndpointSelector,
	stats *ports.RequestStats,
	proxyFunc ProxyFunc,
) error {
	if len(endpoints) == 0 {
		return errors.New("no endpoints available")
	}

	// Work with a copy to avoid modifying the original slice
	availableEndpoints := make([]*domain.Endpoint, len(endpoints))
	copy(availableEndpoints, endpoints)

	// Preserve request body for potential retries
	bodyBytes, err := h.preserveRequestBody(r)
	if err != nil {
		return err
	}

	// Wrap the writer so we can detect when bytes have been committed to the client.
	tracker := NewResponseStartedWriter(w)

	var lastErr error
	maxRetries := h.maxAttempts(len(endpoints))
	// attemptCount is the retry budget: only attempts that reached a backend count.
	// iteration counts every pass, so the body is re-armed after any earlier one.
	attemptCount, iteration := 0, 0
	// Tracked separately from lastErr because lastErr only ever reflects the
	// final attempt: if earlier endpoints failed with real connection errors
	// and the final one was CB-open, lastErr alone would mislabel the whole
	// exhaustion as "circuit breaker open" and hide the network failures.
	var cbOpenCount, connErrCount int

	for attemptCount < maxRetries && len(availableEndpoints) > 0 {
		if err := h.checkContextCancellation(ctx); err != nil {
			return err
		}

		h.resetRequestBodyForRetry(r, bodyBytes, iteration)
		iteration++

		endpoint, err := selector.Select(ctx, availableEndpoints)
		if err != nil {
			return fmt.Errorf("endpoint selection failed: %w", err)
		}

		attemptCount++
		lastErr = h.executeProxyAttempt(ctx, tracker, r, endpoint, selector, stats, proxyFunc)

		if lastErr == nil {
			return nil
		}

		// A CB-open rejection is failover-eligible, but unlike a connection
		// error it must NOT demote the endpoint's persisted health: the breaker
		// already reflects accumulated failure state, and the proxy engine
		// owns demotion for genuine connectivity failures. Remove the endpoint
		// from this request's candidate list only, then continue. Evaluated
		// before IsConnectionError so the sentinel never reaches the
		// connection-error/handleConnectionFailure path below.
		if errors.Is(lastErr, ErrCircuitBreakerOpen) {
			cbOpenCount++
			// The endpoint was never contacted, so the rejection does not spend
			// the attempt budget: with retry disabled or a small max_attempts a
			// healthy peer must still be reachable. The loop still terminates
			// because the endpoint leaves the candidate list below.
			attemptCount--
			// Once the response has started we cannot safely retry even on
			// CB-open: the same partial-response hazard applies. The guard runs
			// identically on every failover path.
			if tracker.started {
				h.logger.Debug("skipping retry: response already started",
					"method", r.Method,
					"endpoint", endpoint.Name)
				return lastErr
			}
			h.logger.Debug("circuit breaker open, failing over to next endpoint",
				"endpoint", endpoint.Name,
				"remaining_endpoints", len(availableEndpoints)-1)
			availableEndpoints = h.removeFailedEndpoint(availableEndpoints, endpoint)
			continue
		}

		if !IsConnectionError(lastErr) {
			// Non-connection error warrants immediate failure
			return lastErr
		}
		connErrCount++

		// Once the response has started we cannot safely retry, whatever the
		// method. The client would receive a partial response from this
		// endpoint followed by a fresh one from the next, causing corruption or
		// double-billing. Return the error and let the caller decide.
		if tracker.started {
			h.logger.Debug("skipping retry: response already started",
				"method", r.Method,
				"endpoint", endpoint.Name)
			return lastErr
		}

		// The connection genuinely failed, so the endpoint is still demoted, but
		// the request may already have been read and acted on by that backend.
		// With non_idempotent off, a POST/PATCH/DELETE is not replayed elsewhere.
		if h.policy.NoNonIdempotent && !isIdempotent(r.Method) {
			h.logger.Warn("Connection failed to endpoint, marking as unhealthy",
				"endpoint", endpoint.Name,
				"error", lastErr,
				"attempt", attemptCount,
				"retry", "skipped: non-idempotent method and proxy.retry.non_idempotent is off",
				"method", r.Method)
			h.markEndpointUnhealthy(ctx, endpoint)
			return lastErr
		}

		// Handle connection error and retry logic
		availableEndpoints = h.handleConnectionFailure(ctx, endpoint, lastErr, attemptCount, availableEndpoints, maxRetries)
	}

	return h.buildFinalError(availableEndpoints, maxRetries, lastErr, cbOpenCount, connErrCount)
}

// preserveRequestBody reads and preserves request body for potential retries
func (h *RetryHandler) preserveRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		// An oversized chunked body is a client rejection the handler answers with 413 and
		// samples; an ERROR here would write one line per request.
		var maxBytesErr *http.MaxBytesError
		if !errors.As(err, &maxBytesErr) {
			h.logger.Error("Failed to read request body for retry preservation", "error", err)
		}
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	if err := r.Body.Close(); err != nil {
		h.logger.Warn("Failed to close original request body", "error", err)
	}

	// Recreate the body for the first attempt
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

// checkContextCancellation verifies if the context has been cancelled
func (h *RetryHandler) checkContextCancellation(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("request cancelled: %w", ctx.Err())
	default:
		return nil
	}
}

// resetRequestBodyForRetry recreates request body for retry attempts
func (h *RetryHandler) resetRequestBodyForRetry(r *http.Request, bodyBytes []byte, attemptCount int) {
	if bodyBytes != nil && attemptCount > 0 {
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
}

// executeProxyAttempt executes a single proxy attempt with connection counting
func (h *RetryHandler) executeProxyAttempt(ctx context.Context, w http.ResponseWriter, r *http.Request,
	endpoint *domain.Endpoint, selector domain.EndpointSelector, stats *ports.RequestStats, proxyFunc ProxyFunc) error {

	selector.IncrementConnections(endpoint)
	defer selector.DecrementConnections(endpoint)

	return proxyFunc(ctx, w, r, endpoint, stats)
}

// handleConnectionFailure processes connection failures and manages endpoint removal.
//
// This is one of two layers of failure protection, and it fires first: a
// single connection-refused error demotes the endpoint to offline right here
// (see markEndpointUnhealthy), on the very first attempt, well before the
// proxy engine's own per-endpoint circuit breaker (olla/service.go's
// circuitBreaker, threshold 5) could accumulate enough failures to open.
// That's intentional layering, not redundancy: connection-refused means the
// backend process is down/unreachable, which request-path health demotion
// (backed by the health checker's periodic re-probing) is the right tool
// for - retrying it further or waiting for a 5-failure threshold would just
// burn latency on a backend that health checks will keep confirming is
// down. In practice this means the circuit breaker rarely gets to open on
// connection-refused; it mostly governs the failure classes that don't
// trip this 1-strike path - hangs and response-header timeouts, where the
// backend is reachable but not answering. See olla/service.go's
// circuitBreaker.IsOpen for the other side of this split.
func (h *RetryHandler) handleConnectionFailure(ctx context.Context, endpoint *domain.Endpoint,
	err error, attemptCount int, availableEndpoints []*domain.Endpoint, maxRetries int) []*domain.Endpoint {

	h.logger.Warn("Connection failed to endpoint, marking as unhealthy",
		"endpoint", endpoint.Name,
		"error", err,
		"attempt", attemptCount,
		"remaining_endpoints", len(availableEndpoints)-1)

	h.markEndpointUnhealthy(ctx, endpoint)

	// Remove failed endpoint from available list
	updatedEndpoints := h.removeFailedEndpoint(availableEndpoints, endpoint)

	if len(updatedEndpoints) > 0 && attemptCount < maxRetries {
		h.logger.Info("Retrying request with different endpoint",
			"available_endpoints", len(updatedEndpoints),
			"attempts_remaining", maxRetries-attemptCount)
	}

	return updatedEndpoints
}

// removeFailedEndpoint removes the failed endpoint from the available list
func (h *RetryHandler) removeFailedEndpoint(endpoints []*domain.Endpoint, failedEndpoint *domain.Endpoint) []*domain.Endpoint {
	for i := range endpoints {
		if endpoints[i].Name == failedEndpoint.Name {
			// Remove element at index i by copying subsequent elements
			copy(endpoints[i:], endpoints[i+1:])
			return endpoints[:len(endpoints)-1]
		}
	}
	return endpoints
}

// buildFinalError constructs the appropriate error message for retry failure.
// cbOpenCount and connErrCount are tallied across every attempt, not just the
// last one - lastErr alone can't distinguish "every endpoint was CB-open"
// from "some failed on the network and the last one happened to be CB-open",
// and conflating the two hides genuine connectivity failures from operators.
func (h *RetryHandler) buildFinalError(availableEndpoints []*domain.Endpoint, maxRetries int, lastErr error, cbOpenCount, connErrCount int) error {
	if len(availableEndpoints) == 0 {
		switch {
		case cbOpenCount > 0 && connErrCount > 0:
			return fmt.Errorf("all endpoints unavailable (%d connection failure(s), %d circuit breaker open): %w",
				connErrCount, cbOpenCount, lastErr)
		case cbOpenCount > 0:
			// CB-open exhaustion is reachable here now (FR-2 removes endpoints
			// without demoting them); don't mislabel it a connection failure, which
			// would imply a network outage that never happened.
			return fmt.Errorf("all endpoints unavailable (circuit breaker open): %w", lastErr)
		default:
			return fmt.Errorf("all endpoints failed with connection errors: %w", lastErr)
		}
	}
	return fmt.Errorf("max attempts (%d) reached: %w", maxRetries, lastErr)
}

// IsConnectionError identifies transient network errors suitable for retry.
// Client-side context cancellations and deadline exceeded errors are NOT
// connection errors — the backend is not at fault and must not be penalised
// or failed-over with an already-expired context.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Short-circuit before the net.Error check: context.DeadlineExceeded's
	// concrete type (*url.Error wrapping it) satisfies net.Error (Timeout()==true),
	// which would falsely mark the endpoint unhealthy for a client timeout.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		switch syscallErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED:
			return true
		default:
			// Non-connection syscall errors
		}
	}

	return hasConnectionError(err)
}

// connectionErrors is a last-resort string fallback for errors that have lost
// their type information (e.g. plain errors.New from middleware or test stubs).
// Well-wrapped OS errors are already caught by the net.Error / syscall.Errno
// branches above, so this list covers only cases those branches cannot reach.
var connectionErrors = []string{
	"connection refused",     // syscall.ECONNREFUSED on non-unwrappable paths
	"connection reset",       // syscall.ECONNRESET on non-unwrappable paths
	"no such host",           // *net.DNSError without type chain
	"network is unreachable", // syscall.ENETUNREACH without type chain
	"no route to host",       // syscall.EHOSTUNREACH without type chain
	"i/o timeout",            // plain-string timeout errors; net.Error.Timeout() covers wrapped ones
	"connectex:",             // Windows dial error prefix; appears without net.Error wrapping on some paths
}

func hasConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	for _, pattern := range connectionErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// markEndpointUnhealthy transitions endpoint to offline state with backoff calculation
func (h *RetryHandler) markEndpointUnhealthy(ctx context.Context, endpoint *domain.Endpoint) {
	if endpoint == nil {
		return
	}

	now := time.Now()

	// Work with copy to preserve original state
	endpointCopy := *endpoint
	endpointCopy.Status = domain.StatusOffline
	endpointCopy.ConsecutiveFailures++
	endpointCopy.LastChecked = now

	// Calculate proper exponential backoff multiplier
	// First failure: keep default interval from the endpoint but set multiplier to 2
	// Subsequent failures: apply exponential backoff
	var backoffInterval time.Duration

	if endpointCopy.BackoffMultiplier <= 1 {
		// First failure - use normal interval
		endpointCopy.BackoffMultiplier = 2
		backoffInterval = endpointCopy.CheckInterval
	} else {
		// Subsequent failures - apply current multiplier and calculate next
		backoffInterval = endpointCopy.CheckInterval * time.Duration(endpointCopy.BackoffMultiplier)

		// Calculate next multiplier for future failures
		endpointCopy.BackoffMultiplier *= 2
		if endpointCopy.BackoffMultiplier > constants.DefaultMaxBackoffMultiplier {
			endpointCopy.BackoffMultiplier = constants.DefaultMaxBackoffMultiplier
		}
	}

	if backoffInterval > constants.DefaultMaxBackoffSeconds {
		backoffInterval = constants.DefaultMaxBackoffSeconds
	}
	endpointCopy.NextCheckTime = now.Add(backoffInterval)

	h.logger.Warn("Marking endpoint as unhealthy due to connection failure",
		"endpoint", endpoint.Name,
		"consecutive_failures", endpointCopy.ConsecutiveFailures,
		"backoff_multiplier", endpointCopy.BackoffMultiplier,
		"next_check", endpointCopy.NextCheckTime.Format(time.RFC3339))

	// Persist status change via discovery service
	h.updateEndpointStatus(ctx, &endpointCopy)
}

// updateEndpointStatus persists endpoint state changes
func (h *RetryHandler) updateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) {
	if err := h.discoveryService.UpdateEndpointStatus(ctx, endpoint); err != nil {
		h.logger.Debug("Failed to update endpoint status in repository", "error", err)
	}
}
