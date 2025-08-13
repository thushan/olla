package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultConcurrentChecks    = 5
	DefaultHealthCheckInterval = 30 * time.Second
	LogThrottleInterval        = 2 * time.Minute
)

type HTTPHealthChecker struct {
	repository       domain.EndpointRepository
	logger           logger.StyledLogger
	recoveryCallback RecoveryCallback
	healthClient     *HealthClient
	ticker           *time.Ticker
	stopCh           chan struct{}
	isRunning        atomic.Bool
}

func NewHTTPHealthChecker(repository domain.EndpointRepository, logger logger.StyledLogger, client HTTPClient) *HTTPHealthChecker {
	circuitBreaker := NewCircuitBreaker()
	healthClient := NewHealthClient(client, circuitBreaker)

	return &HTTPHealthChecker{
		healthClient:     healthClient,
		repository:       repository,
		logger:           logger,
		stopCh:           make(chan struct{}),
		recoveryCallback: NoOpRecoveryCallback{},
	}
}

// SetRecoveryCallback sets the callback to be invoked when an endpoint recovers
func (c *HTTPHealthChecker) SetRecoveryCallback(callback RecoveryCallback) {
	if callback != nil {
		c.recoveryCallback = callback
	}
}

func NewHTTPHealthCheckerWithDefaults(repository domain.EndpointRepository, logger logger.StyledLogger) *HTTPHealthChecker {
	// We want to enable connection pooling and reuse with some sane defaults
	client := &http.Client{
		Timeout: DefaultHealthCheckerTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false,
		},
	}
	return NewHTTPHealthChecker(repository, logger, client)
}

func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	// Add timeout protection for individual checks
	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout*2) // Give extra time for retries
	defer cancel()

	return c.healthClient.Check(checkCtx, endpoint)
}

func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	if c.isRunning.Load() {
		return nil
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return domain.NewEndpointError("start_health_checking", "repository",
			fmt.Errorf("failed to get endpoints for health checking: %w", err))
	}

	c.logger.Info("Starting Health Checker Service", "check_interval", DefaultHealthCheckInterval, "endpoints", len(endpoints))

	c.isRunning.Store(true)

	c.ticker = time.NewTicker(DefaultHealthCheckInterval)
	go c.healthCheckLoop(ctx)

	return nil
}

func (c *HTTPHealthChecker) StopChecking(ctx context.Context) error {
	if !c.isRunning.Load() {
		return nil
	}

	if c.ticker != nil {
		c.ticker.Stop()
	}

	close(c.stopCh)
	c.isRunning.Store(false)

	return nil
}

func (c *HTTPHealthChecker) healthCheckLoop(ctx context.Context) {
	defer func() {
		if c.ticker != nil {
			c.ticker.Stop()
		}
		// Panic recovery for health check loop
		if r := recover(); r != nil {
			c.logger.Error("Health check loop panic recovered", "panic", r)
			// Could restart the loop here if needed
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("Health check loop stopping due to context cancellation")
			return
		case <-c.stopCh:
			c.logger.Debug("Health check loop stopping due to stop signal")
			return
		case <-c.ticker.C:
			// Use a separate context for health checks to avoid cancelling mid-check
			checkCtx, cancel := context.WithTimeout(context.Background(), DefaultHealthCheckInterval/2)
			c.performHealthChecks(checkCtx)
			cancel()
		}
	}
}

func (c *HTTPHealthChecker) performHealthChecks(ctx context.Context) {
	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		c.logger.Error("Failed to get endpoints for health checking",
			"error", domain.NewEndpointError("get_endpoints", "repository", err))
		return
	}

	now := time.Now()
	var wg sync.WaitGroup

	endpointsToCheck := make([]*domain.Endpoint, 0, len(endpoints))

	// Filter endpoints that are due for checking
	for _, endpoint := range endpoints {
		if now.Before(endpoint.NextCheckTime) {
			continue
		}
		endpointsToCheck = append(endpointsToCheck, endpoint)
	}

	if len(endpointsToCheck) == 0 {
		return
	}

	c.logger.Debug("Performing health checks", "endpoints_to_check", len(endpointsToCheck))

	// Limit concurrency to avoid overwhelming the health client
	semaphore := make(chan struct{}, DefaultConcurrentChecks)

	for _, endpoint := range endpointsToCheck {
		wg.Add(1)
		go func(ep *domain.Endpoint) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			c.checkEndpointSafely(ctx, ep)
		}(endpoint)
	}

	wg.Wait()
}

func (c *HTTPHealthChecker) checkEndpointSafely(ctx context.Context, endpoint *domain.Endpoint) {
	// Panic recovery for individual endpoint checks
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("Health check panic recovered for endpoint",
				"endpoint", endpoint.Name,
				"url", endpoint.GetURLString(),
				"panic", r)
		}
	}()

	c.checkEndpoint(ctx, endpoint)
}

func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint *domain.Endpoint) {
	now := time.Now()
	result, err := c.healthClient.Check(ctx, endpoint)

	oldStatus := endpoint.Status
	newStatus := result.Status
	statusChanged := oldStatus != newStatus

	endpointCopy := *endpoint
	endpointCopy.Status = newStatus
	endpointCopy.LastChecked = now
	endpointCopy.LastLatency = result.Latency

	isSuccess := result.Status == domain.StatusHealthy
	nextInterval, newMultiplier := calculateBackoff(&endpointCopy, isSuccess)

	if !isSuccess {
		endpointCopy.ConsecutiveFailures++
		endpointCopy.BackoffMultiplier = newMultiplier
	} else {
		endpointCopy.ConsecutiveFailures = 0
		endpointCopy.BackoffMultiplier = 1
	}

	endpointCopy.NextCheckTime = now.Add(nextInterval)

	// Check if endpoint still exists before updating
	if !c.repository.Exists(ctx, endpoint.URL) {
		c.logger.Debug("Endpoint removed from configuration, stopping health checks",
			"endpoint", endpoint.GetURLString())
		return
	}

	if repoErr := c.repository.UpdateEndpoint(ctx, &endpointCopy); repoErr != nil {
		enhancedErr := domain.NewEndpointError("update_endpoint", endpoint.GetURLString(), repoErr)
		c.logger.Error("Failed to update endpoint", "error", enhancedErr)
		return
	}

	// Trigger recovery callback if endpoint recovered (transitioned to healthy from unhealthy)
	if statusChanged && newStatus == domain.StatusHealthy && oldStatus != domain.StatusUnknown {
		c.logger.Info("Endpoint recovered, triggering model discovery refresh",
			"endpoint", endpoint.Name,
			"was", oldStatus.String())

		if c.recoveryCallback != nil {
			if callbackErr := c.recoveryCallback.OnEndpointRecovered(ctx, &endpointCopy); callbackErr != nil {
				c.logger.Warn("Failed to execute recovery callback",
					"endpoint", endpoint.Name,
					"error", callbackErr)
			}
		}
	}

	// Enhanced logging with better error context
	c.logHealthCheckResult(endpoint, oldStatus, newStatus, statusChanged, result, nextInterval, err)
}

func (c *HTTPHealthChecker) logHealthCheckResult(
	endpoint *domain.Endpoint,
	oldStatus, newStatus domain.EndpointStatus,
	statusChanged bool,
	result domain.HealthCheckResult,
	nextInterval time.Duration,
	checkErr error,
) {
	switch {
	case statusChanged:
		switch {
		case oldStatus == domain.StatusUnknown && newStatus == domain.StatusHealthy:
			c.logger.InfoHealthStatus("Endpoint initial status:",
				endpoint.Name,
				newStatus,
				"latency", result.Latency,
				"next_check_in", nextInterval)

		case oldStatus == domain.StatusUnknown:
			// Initial discovery of unhealthy endpoint
			detailedArgs := []interface{}{
				"endpoint_url", endpoint.GetURLString(),
				"status_code", result.StatusCode,
				"error_type", result.ErrorType,
			}
			if checkErr != nil {
				var healthCheckErr *domain.HealthCheckError
				if errors.As(checkErr, &healthCheckErr) {
					detailedArgs = append(detailedArgs, "check_error", healthCheckErr.Error())
				} else {
					detailedArgs = append(detailedArgs, "check_error", checkErr.Error())
				}
			}
			c.logger.WarnWithContext("Endpoint discovered offline:", endpoint.Name, logger.LogContext{
				UserArgs: []interface{}{
					"status", newStatus.String(),
					"latency", result.Latency,
					"next_check_in", nextInterval,
				},
				DetailedArgs: detailedArgs,
			})

		case newStatus == domain.StatusHealthy:
			c.logger.InfoHealthStatus("Endpoint recovered:",
				endpoint.Name,
				newStatus,
				"was", oldStatus.String(),
				"latency", result.Latency,
				"next_check_in", nextInterval)

		default:
			// Status changed to unhealthy
			detailedArgs := []interface{}{
				"endpoint_url", endpoint.GetURLString(),
				"status_code", result.StatusCode,
				"error_type", result.ErrorType,
			}
			if checkErr != nil {
				var healthCheckErr *domain.HealthCheckError
				if errors.As(checkErr, &healthCheckErr) {
					detailedArgs = append(detailedArgs, "check_error", healthCheckErr.Error())
				} else {
					detailedArgs = append(detailedArgs, "check_error", checkErr.Error())
				}
			}
			c.logger.WarnWithContext("Endpoint status changed:", endpoint.Name, logger.LogContext{
				UserArgs: []interface{}{
					"status", newStatus.String(),
					"was", oldStatus.String(),
					"consecutive_failures", endpoint.ConsecutiveFailures + 1,
					"latency", result.Latency,
					"next_check_in", nextInterval,
				},
				DetailedArgs: detailedArgs,
			})
		}

	case checkErr != nil && endpoint.ConsecutiveFailures > 0 && endpoint.ConsecutiveFailures%5 == 0:
		// Log ongoing issues every 5th consecutive failure instead of time-based throttling
		detailedArgs := []interface{}{
			"endpoint_url", endpoint.GetURLString(),
			"status_code", result.StatusCode,
			"error_type", result.ErrorType,
		}
		var healthCheckErr *domain.HealthCheckError
		if errors.As(checkErr, &healthCheckErr) {
			detailedArgs = append(detailedArgs, "check_error", healthCheckErr.Error())
		} else {
			detailedArgs = append(detailedArgs, "check_error", checkErr.Error())
		}

		c.logger.WarnWithContext("Endpoint still having issues:", endpoint.Name, logger.LogContext{
			UserArgs: []interface{}{
				"status", newStatus.String(),
				"consecutive_failures", endpoint.ConsecutiveFailures + 1,
				"next_check_in", nextInterval,
			},
			DetailedArgs: detailedArgs,
		})
	}
}

func (c *HTTPHealthChecker) GetSchedulerStats() map[string]interface{} {
	running := c.isRunning.Load()

	if !running {
		return map[string]interface{}{
			"isRunning": false,
		}
	}

	return map[string]interface{}{
		"isRunning":      running,
		"check_interval": DefaultHealthCheckInterval.String(),
	}
}

func (c *HTTPHealthChecker) RunHealthCheck(ctx context.Context, initial bool) error {
	if !c.isRunning.Load() {
		return domain.NewEndpointError("run_health_check", "health_checker",
			fmt.Errorf("health checker is not running"))
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return domain.NewEndpointError("run_health_check", "repository",
			fmt.Errorf("failed to get endpoints: %w", err))
	}

	if initial {
		c.logger.Info("Initialising health checks", "endpoints", len(endpoints))
	} else {
		c.logger.Info("Running full health checks", "endpoints", len(endpoints))
	}

	c.logEndpointsTable(endpoints)

	// Use controlled concurrency for manual health checks too
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Allow more concurrency for manual checks

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep *domain.Endpoint) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			c.checkEndpointSafely(ctx, ep)
		}(endpoint)
	}

	wg.Wait()
	return nil
}

func (c *HTTPHealthChecker) logEndpointsTable(endpoints []*domain.Endpoint) {
	if len(endpoints) == 0 {
		return
	}

	// Sort routes by registration priority
	type endpointEntry struct {
		url      string
		name     string
		kind     string
		health   string
		priority int
		interval int
		timeout  int
	}

	entries := make([]endpointEntry, 0, len(endpoints))
	for _, info := range endpoints {
		entries = append(entries, endpointEntry{
			url:      info.URL.String(),
			name:     info.Name,
			kind:     info.Type,
			health:   info.HealthCheckPathString,
			priority: info.Priority,
			interval: int(info.CheckInterval.Seconds()),
			timeout:  int(info.CheckTimeout.Seconds()),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	// Build table
	tableData := make([][]string, 0, len(entries)+1)
	tableData = append(tableData, []string{"PRI", "NAME", "TYPE", "URL", "HEALTH", "CHECK", "TIMEOUT"})

	for _, entry := range entries {
		tableData = append(tableData, []string{
			fmt.Sprintf("%d", entry.priority),
			entry.name,
			entry.kind,
			entry.url,
			entry.health,
			fmt.Sprintf("%ds", entry.interval),
			fmt.Sprintf("%ds", entry.timeout),
		})
	}

	c.logger.InfoWithCount("Registered endpoints", len(entries))
	// .WithHeaderStyle(c.logger.Theme.Accent)
	tableString, _ := pterm.DefaultTable.WithHasHeader().WithData(tableData).Srender()
	fmt.Print(tableString)
}
