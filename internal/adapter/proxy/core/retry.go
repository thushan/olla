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

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// RetryHandler manages connection failure recovery and endpoint failover
type RetryHandler struct {
	logger           logger.StyledLogger
	discoveryService ports.DiscoveryService
}

// NewRetryHandler creates a new retry handler
func NewRetryHandler(discoveryService ports.DiscoveryService, logger logger.StyledLogger) *RetryHandler {
	return &RetryHandler{
		discoveryService: discoveryService,
		logger:           logger,
	}
}

// ProxyFunc defines the signature for endpoint proxy implementations
type ProxyFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error

// responseStartedWriter wraps http.ResponseWriter and records whether any response
// bytes or status codes have been sent to the client. We use this to gate retry
// decisions: once the client has received data, retrying a non-idempotent request
// would send duplicate content or charge twice on metered APIs.
type responseStartedWriter struct {
	http.ResponseWriter
	started bool
}

func (rw *responseStartedWriter) WriteHeader(code int) {
	rw.started = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseStartedWriter) Write(b []byte) (int, error) {
	rw.started = true
	return rw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so http.NewResponseController can
// discover optional interfaces (Flush, Hijack, SetDeadline, etc.) via the chain.
// Without this, the wrapper hides the underlying flusher and SSE streams stall.
func (rw *responseStartedWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// isIdempotent reports whether the HTTP method is safe to retry after a partial
// response. GET, HEAD and OPTIONS are defined as idempotent by RFC 9110; POST,
// PATCH and DELETE are not. Retrying them risks double-billing or duplicate side
// effects if the upstream already processed the first attempt.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// ExecuteWithRetry attempts request delivery with automatic failover on connection errors.
// For non-idempotent methods (POST, PATCH, DELETE), retry is suppressed once the
// response has started. Resending to a different endpoint risks duplicate content
// reaching the client or double-charging a metered API.
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
	tracker := &responseStartedWriter{ResponseWriter: w}

	var lastErr error
	maxRetries := len(endpoints)
	attemptCount := 0

	for attemptCount < maxRetries && len(availableEndpoints) > 0 {
		if err := h.checkContextCancellation(ctx); err != nil {
			return err
		}

		h.resetRequestBodyForRetry(r, bodyBytes, attemptCount)

		endpoint, err := selector.Select(ctx, availableEndpoints)
		if err != nil {
			return fmt.Errorf("endpoint selection failed: %w", err)
		}

		attemptCount++
		lastErr = h.executeProxyAttempt(ctx, tracker, r, endpoint, selector, stats, proxyFunc)

		if lastErr == nil {
			return nil
		}

		if !IsConnectionError(lastErr) {
			// Non-connection error warrants immediate failure
			return lastErr
		}

		// For non-idempotent methods, once the response has started we cannot
		// safely retry. The client would receive a partial response from this
		// endpoint followed by a fresh one from the next, causing corruption or
		// double-billing. Return the error and let the caller decide.
		if tracker.started && !isIdempotent(r.Method) {
			h.logger.Debug("skipping retry: response already started for non-idempotent method",
				"method", r.Method,
				"endpoint", endpoint.Name)
			return lastErr
		}

		// Handle connection error and retry logic
		availableEndpoints = h.handleConnectionFailure(ctx, endpoint, lastErr, attemptCount, availableEndpoints, maxRetries)
	}

	return h.buildFinalError(availableEndpoints, maxRetries, lastErr)
}

// preserveRequestBody reads and preserves request body for potential retries
func (h *RetryHandler) preserveRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read request body for retry preservation", "error", err)
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

// handleConnectionFailure processes connection failures and manages endpoint removal
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

// buildFinalError constructs the appropriate error message for retry failure
func (h *RetryHandler) buildFinalError(availableEndpoints []*domain.Endpoint, maxRetries int, lastErr error) error {
	if len(availableEndpoints) == 0 {
		return fmt.Errorf("all endpoints failed with connection errors: %w", lastErr)
	}
	return fmt.Errorf("max attempts (%d) reached: %w", maxRetries, lastErr)
}

// IsConnectionError identifies transient network errors suitable for retry
func IsConnectionError(err error) bool {
	if err == nil {
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
