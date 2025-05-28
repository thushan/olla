package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type HTTPHealthChecker struct {
	healthClient    *HealthClient
	circuitBreaker  *CircuitBreaker
	statusTracker   *StatusTransitionTracker
	repository      domain.EndpointRepository
	ticker          *time.Ticker
	stopCh          chan struct{}
	logger          *logger.StyledLogger
	mu              sync.Mutex
	running         bool
}

// NewHTTPHealthChecker creates a health checker with the provided HTTP client
func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *logger.StyledLogger, client HTTPClient) *HTTPHealthChecker {
	circuitBreaker := NewCircuitBreaker()
	statusTracker := NewStatusTransitionTracker()
	healthClient := NewHealthClient(client, circuitBreaker)

	return &HTTPHealthChecker{
		healthClient:   healthClient,
		circuitBreaker: circuitBreaker,
		statusTracker:  statusTracker,
		repository:     repository,
		logger:         logger,
		stopCh:         make(chan struct{}),
	}
}

// NewHTTPHealthCheckerWithDefaults creates a health checker with default HTTP client
func NewHTTPHealthCheckerWithDefaults(repository domain.EndpointRepository, logger *logger.StyledLogger) *HTTPHealthChecker {
	client := &http.Client{
		Timeout: DefaultHealthCheckerTimeout,
	}
	return NewHTTPHealthChecker(repository, logger, client)
}

// Check delegates to the health client - maintains existing public API
func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	return c.healthClient.Check(ctx, endpoint)
}

func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints for health checking: %w", err)
	}

	c.running = true

	c.logger.Info("Health checker starting",
		"check_interval", DefaultHealthCheckInterval,
		"endpoints", len(endpoints))

	// Start simple ticker-based health checking
	c.ticker = time.NewTicker(DefaultHealthCheckInterval)
	go c.healthCheckLoop(ctx)

	return nil
}

func (c *HTTPHealthChecker) StopChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	if c.ticker != nil {
		c.ticker.Stop()
	}

	close(c.stopCh)
	c.running = false

	return nil
}

func (c *HTTPHealthChecker) healthCheckLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-c.ticker.C:
			c.performHealthChecks(ctx)
		}
	}
}

func (c *HTTPHealthChecker) performHealthChecks(ctx context.Context) {
	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		c.logger.Error("Failed to get endpoints for health checking", "error", err)
		return
	}

	now := time.Now()

	for _, endpoint := range endpoints {
		// Check if this endpoint is due for a health check
		if now.Before(endpoint.NextCheckTime) {
			continue
		}

		// Perform health check
		c.checkEndpoint(ctx, endpoint)
	}

	// Cleanup old circuit breaker and status tracker entries
	c.performCleanup(endpoints)
}

func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint *domain.Endpoint) {
	result, err := c.healthClient.Check(ctx, endpoint)

	endpoint.Status = result.Status
	endpoint.LastChecked = time.Now()
	endpoint.LastLatency = result.Latency

	isSuccess := result.Status == domain.StatusHealthy
	nextInterval, newMultiplier := calculateBackoff(endpoint, isSuccess)

	if !isSuccess {
		endpoint.ConsecutiveFailures++
		endpoint.BackoffMultiplier = newMultiplier
	} else {
		endpoint.ConsecutiveFailures = 0
		endpoint.BackoffMultiplier = 1
	}

	endpoint.NextCheckTime = time.Now().Add(nextInterval)

	// Check if endpoint still exists before updating
	if !c.repository.Exists(ctx, endpoint.URL) {
		c.logger.Debug("Endpoint removed from configuration, stopping health checks",
			"endpoint", endpoint.GetURLString())
		return
	}

	if repoErr := c.repository.UpdateEndpoint(ctx, endpoint); repoErr != nil {
		c.logger.Error("Failed to update endpoint",
			"endpoint", endpoint.GetURLString(),
			"error", repoErr)
		return
	}

	shouldLog, errorCount := c.statusTracker.ShouldLog(
		endpoint.GetURLString(),
		result.Status,
		err != nil)

	if shouldLog {
		if errorCount > 0 ||
			(result.Status == domain.StatusOffline ||
				result.Status == domain.StatusBusy ||
				result.Status == domain.StatusUnhealthy) {
			c.logger.WarnWithEndpoint("Endpoint health issues for", endpoint.Name,
				"status", result.Status.String(),
				"consecutive_failures", errorCount,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		} else {
			c.logger.InfoHealthStatus("Endpoint status changed for",
				endpoint.Name,
				result.Status,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		}
	}
}

func (c *HTTPHealthChecker) performCleanup(currentEndpoints []*domain.Endpoint) {
	if len(currentEndpoints) == 0 {
		return
	}

	currentEndpointURLs := make(map[string]struct{}, len(currentEndpoints))
	for _, endpoint := range currentEndpoints {
		currentEndpointURLs[endpoint.GetURLString()] = struct{}{}
	}

	// Clean circuit breaker
	circuitEndpoints := c.circuitBreaker.GetActiveEndpoints()
	for _, url := range circuitEndpoints {
		if _, exists := currentEndpointURLs[url]; !exists {
			c.circuitBreaker.CleanupEndpoint(url)
		}
	}

	// Clean status tracker
	statusEndpoints := c.statusTracker.GetActiveEndpoints()
	for _, url := range statusEndpoints {
		if _, exists := currentEndpointURLs[url]; !exists {
			c.statusTracker.CleanupEndpoint(url)
		}
	}
}

func (c *HTTPHealthChecker) GetSchedulerStats() map[string]interface{} {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()

	if !running {
		return map[string]interface{}{
			"running": false,
		}
	}

	return map[string]interface{}{
		"running":          running,
		"check_interval":   DefaultHealthCheckInterval.String(),
		"circuit_breaker": c.getCircuitBreakerStats(),
		"status_tracker":  c.getStatusTrackerStats(),
	}
}

func (c *HTTPHealthChecker) getCircuitBreakerStats() map[string]interface{} {
	activeEndpoints := c.circuitBreaker.GetActiveEndpoints()

	openCircuits := 0
	for _, endpoint := range activeEndpoints {
		if c.circuitBreaker.IsOpen(endpoint) {
			openCircuits++
		}
	}

	return map[string]interface{}{
		"total_endpoints": len(activeEndpoints),
		"open_circuits":   openCircuits,
	}
}

func (c *HTTPHealthChecker) getStatusTrackerStats() map[string]interface{} {
	activeEndpoints := c.statusTracker.GetActiveEndpoints()

	return map[string]interface{}{
		"tracked_endpoints": len(activeEndpoints),
	}
}

func (c *HTTPHealthChecker) ForceHealthCheck(ctx context.Context) error {
	if !c.running {
		return fmt.Errorf("health checker is not running")
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	c.logger.Info("Forcing health check", "endpoints", len(endpoints))

	// Force check all endpoints immediately
	for _, endpoint := range endpoints {
		c.checkEndpoint(ctx, endpoint)
	}

	return nil
}

const (
	DefaultHealthCheckInterval = 30 * time.Second
)