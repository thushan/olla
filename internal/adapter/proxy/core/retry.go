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

	"github.com/thushan/olla/internal/adapter/health"
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

// ExecuteWithRetry attempts request delivery with automatic failover on connection errors
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

	// Work with a copy to avoid modifying the original slice
	availableEndpoints := make([]*domain.Endpoint, len(endpoints))
	copy(availableEndpoints, endpoints)

	var lastErr error
	maxRetries := len(endpoints)
	retryCount := 0

	// Preserve request body for potential retries
	var bodyBytes []byte
	if r.Body != nil && r.Body != http.NoBody {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}

	for retryCount <= maxRetries && len(availableEndpoints) > 0 {
		if bodyBytes != nil {
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		endpoint, err := selector.Select(ctx, availableEndpoints)
		if err != nil {
			return fmt.Errorf("endpoint selection failed: %w", err)
		}

		err = proxyFunc(ctx, w, r, endpoint, stats)

		if err == nil {
			return nil
		}

		lastErr = err

		if IsConnectionError(err) {
			h.logger.Warn("Connection failed to endpoint, marking as unhealthy",
				"endpoint", endpoint.Name,
				"error", err,
				"retry", retryCount+1,
				"remaining_endpoints", len(availableEndpoints)-1)

			h.markEndpointUnhealthy(ctx, endpoint)

			// Remove failed endpoint in-place to avoid allocation
			// Find and remove the failed endpoint by shifting elements
			for i := 0; i < len(availableEndpoints); i++ {
				if availableEndpoints[i].Name == endpoint.Name {
					// Remove element at index i by copying subsequent elements
					copy(availableEndpoints[i:], availableEndpoints[i+1:])
					availableEndpoints = availableEndpoints[:len(availableEndpoints)-1]
					break
				}
			}

			retryCount++

			if len(availableEndpoints) > 0 {
				h.logger.Info("Retrying request with different endpoint",
					"available_endpoints", len(availableEndpoints))
				continue
			}
		} else {
			// Non-connection error warrants immediate failure
			return err
		}
	}

	if len(availableEndpoints) == 0 {
		return fmt.Errorf("all endpoints failed with connection errors: %w", lastErr)
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
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

var connectionErrors = []string{
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
		if endpointCopy.BackoffMultiplier > health.MaxBackoffMultiplier {
			endpointCopy.BackoffMultiplier = health.MaxBackoffMultiplier
		}
	}

	if backoffInterval > health.MaxBackoffSeconds {
		backoffInterval = health.MaxBackoffSeconds
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
