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

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// RetryHandler provides connection failure retry logic for proxy implementations
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

// ProxyFunc is the function signature for proxying to a single endpoint
type ProxyFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error

// ExecuteWithRetry executes a proxy request with retry logic on connection failures
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
		return fmt.Errorf("no endpoints available")
	}

	// Create a copy of endpoints for retry logic
	availableEndpoints := make([]*domain.Endpoint, len(endpoints))
	copy(availableEndpoints, endpoints)

	var lastErr error
	maxRetries := len(endpoints) // Try each endpoint once
	retryCount := 0

	// Save the original request body for retry
	var bodyBytes []byte
	if r.Body != nil && r.Body != http.NoBody {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}

	for retryCount <= maxRetries && len(availableEndpoints) > 0 {
		// Restore request body for retry
		if bodyBytes != nil {
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		// Select endpoint
		endpoint, err := selector.Select(ctx, availableEndpoints)
		if err != nil {
			return fmt.Errorf("endpoint selection failed: %w", err)
		}

		// Try proxying to this endpoint
		err = proxyFunc(ctx, w, r, endpoint, stats)

		if err == nil {
			// Success!
			return nil
		}

		lastErr = err

		// Check if this is a connection error
		if IsConnectionError(err) {
			h.logger.Warn("Connection failed to endpoint, marking as unhealthy",
				"endpoint", endpoint.Name,
				"error", err,
				"retry", retryCount+1,
				"remaining_endpoints", len(availableEndpoints)-1)

			// Mark endpoint as unhealthy
			h.markEndpointUnhealthy(ctx, endpoint)

			// Remove this endpoint from available list
			newAvailable := make([]*domain.Endpoint, 0, len(availableEndpoints)-1)
			for _, ep := range availableEndpoints {
				if ep.Name != endpoint.Name {
					newAvailable = append(newAvailable, ep)
				}
			}
			availableEndpoints = newAvailable

			retryCount++

			// Continue to next endpoint
			if len(availableEndpoints) > 0 {
				h.logger.Info("Retrying request with different endpoint",
					"available_endpoints", len(availableEndpoints))
				continue
			}
		} else {
			// Non-connection error, don't retry
			return err
		}
	}

	if len(availableEndpoints) == 0 {
		return fmt.Errorf("all endpoints failed with connection errors: %w", lastErr)
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

// IsConnectionError determines if an error is a connection failure that warrants retry
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for specific syscall errors
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		switch syscallErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED:
			return true
		default:
			// Other syscall errors are not connection errors
		}
	}

	// Check for common connection error messages
	errStr := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"no route to host",
		"connection timed out",
		"i/o timeout",
		"dial tcp",
		"connectex:",
	}

	for _, pattern := range connectionErrors {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// markEndpointUnhealthy marks an endpoint as unhealthy
func (h *RetryHandler) markEndpointUnhealthy(ctx context.Context, endpoint *domain.Endpoint) {
	if endpoint == nil {
		return
	}

	// Create a copy to avoid modifying the original
	endpointCopy := *endpoint
	endpointCopy.Status = domain.StatusOffline
	endpointCopy.ConsecutiveFailures++
	endpointCopy.LastChecked = time.Now()

	// Calculate backoff for next check
	backoffSeconds := endpointCopy.ConsecutiveFailures * 2
	if backoffSeconds > 60 {
		backoffSeconds = 60
	}
	endpointCopy.NextCheckTime = time.Now().Add(time.Duration(backoffSeconds) * time.Second)

	h.logger.Warn("Marking endpoint as unhealthy due to connection failure",
		"endpoint", endpoint.Name,
		"consecutive_failures", endpointCopy.ConsecutiveFailures,
		"next_check", endpointCopy.NextCheckTime.Format(time.RFC3339))

	// Try to update endpoint status in repository through discovery service
	if discoveryService, ok := h.discoveryService.(ports.DiscoveryServiceWithEndpointUpdate); ok {
		if err := discoveryService.UpdateEndpointStatus(ctx, &endpointCopy); err != nil {
			h.logger.Debug("Failed to update endpoint status in repository", "error", err)
		}
	}
}
