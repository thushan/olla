package health

import (
	"context"
	"errors"
	"fmt"
	"github.com/thushan/olla/internal/logger"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

/*
Health Check Status Promotion Logic:

Network Errors (connection refused, timeout, DNS failure):
- First failure: StatusOffline (immediate)
- Backoff: 2x, 4x, 8x up to MaxBackoffMultiplier
- Recovery: Reset to normal on any success

HTTP Errors (5xx responses):
- High latency (>SlowResponseThreshold): StatusBusy
- Normal latency: StatusUnhealthy
- Consecutive failures trigger backoff

HTTP Success (2xx):
- High latency (>SlowResponseThreshold): StatusBusy
- Normal latency: StatusHealthy
- Resets failure counters and backoff

Circuit Breaker:
- Opens after FailureThreshold consecutive failures
- Returns StatusOffline when open
- Auto-recovers after CircuitBreakerTimeout
*/

const (
	DefaultHealthCheckerWorkerCount = 10
	DefaultHealthCheckerQueueSize   = 100

	DefaultHealthCheckerTimeout = 5 * time.Second
	SlowResponseThreshold       = 10 * time.Second
	VerySlowResponseThreshold   = 30 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 3
	DefaultCircuitBreakerTimeout   = 30 * time.Second

	MaxBackoffMultiplier = 12
	BaseBackoffSeconds   = 2

	CleanupInterval = 5 * time.Minute
)

// healthCheckJob represents a health check task
type healthCheckJob struct {
	endpoint *domain.Endpoint
	ctx      context.Context
}

// HTTPHealthChecker implements domain.HealthChecker for HTTP health checks
type HTTPHealthChecker struct {
	repository     domain.EndpointRepository
	client         HTTPClient
	circuitBreaker *CircuitBreaker
	statusTracker  *StatusTransitionTracker
	cleanupTicker  *time.Ticker
	stopCh         chan struct{}
	jobCh          chan healthCheckJob
	wg             sync.WaitGroup
	mu             sync.Mutex
	running        bool
	workerCount    int
	logger         *logger.StyledLogger
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *logger.StyledLogger) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		repository: repository,
		client: &http.Client{
			Timeout: DefaultHealthCheckerTimeout,
		},
		circuitBreaker: NewCircuitBreaker(),
		statusTracker:  NewStatusTransitionTracker(),
		stopCh:         make(chan struct{}),
		jobCh:          make(chan healthCheckJob, DefaultHealthCheckerQueueSize),
		workerCount:    DefaultHealthCheckerWorkerCount,
		logger:         logger,
	}
}

func classifyError(err error) domain.HealthCheckErrorType {
	if errors.Is(err, ErrCircuitBreakerOpen) {
		return domain.ErrorTypeCircuitOpen
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return domain.ErrorTypeTimeout
		}
		return domain.ErrorTypeNetwork
	}

	return domain.ErrorTypeHTTPError
}

func determineStatus(statusCode int, latency time.Duration, err error, errorType domain.HealthCheckErrorType) domain.EndpointStatus {
	if err != nil {
		switch errorType {
		case domain.ErrorTypeNetwork, domain.ErrorTypeTimeout, domain.ErrorTypeCircuitOpen:
			return domain.StatusOffline
		default:
			return domain.StatusUnhealthy
		}
	}

	// HTTP response received
	if statusCode >= HealthyEndpointStatusRangeStart && statusCode < HealthyEndpointStatusRangeEnd {
		if latency > SlowResponseThreshold {
			return domain.StatusBusy
		}
		return domain.StatusHealthy
	}

	// HTTP error codes
	if latency > SlowResponseThreshold {
		return domain.StatusBusy
	}
	return domain.StatusUnhealthy
}

func calculateBackoff(endpoint *domain.Endpoint, success bool) (time.Duration, int) {
	if success {
		// Reset on success
		return endpoint.CheckInterval, 1
	}

	// Increase backoff multiplier
	// this is the simplest backoff strategy for now
	multiplier := endpoint.BackoffMultiplier * 2
	if multiplier > MaxBackoffMultiplier {
		multiplier = MaxBackoffMultiplier
	}

	backoffInterval := endpoint.CheckInterval * time.Duration(multiplier)
	return backoffInterval, multiplier
}

// Check performs a health check on the endpoint and returns its status
func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	start := time.Now()
	// Use pre-computed absolute URL instead of resolving each time
	healthCheckUrl := endpoint.HealthCheckURL.String()

	result := domain.HealthCheckResult{
		Status: domain.StatusUnknown,
	}

	if c.circuitBreaker.IsOpen(healthCheckUrl) {
		result.Status = domain.StatusOffline
		result.Error = ErrCircuitBreakerOpen
		result.ErrorType = domain.ErrorTypeCircuitOpen
		result.Latency = time.Since(start)
		return result, ErrCircuitBreakerOpen
	}

	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthCheckUrl, nil)
	if err != nil {
		result.Latency = time.Since(start)
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		c.circuitBreaker.RecordFailure(healthCheckUrl)
		return result, err
	}

	resp, err := c.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		c.circuitBreaker.RecordFailure(healthCheckUrl)
		return result, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	result.Status = determineStatus(resp.StatusCode, result.Latency, nil, domain.ErrorTypeNone)

	if result.Status == domain.StatusHealthy {
		c.circuitBreaker.RecordSuccess(healthCheckUrl)
	} else {
		c.circuitBreaker.RecordFailure(healthCheckUrl)
	}

	return result, nil
}

// StartChecking begins periodic health checks for all endpoints
func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Recreate channels for clean start
	c.stopCh = make(chan struct{})
	c.jobCh = make(chan healthCheckJob, DefaultHealthCheckerQueueSize)
	c.running = true

	// Start worker goroutines
	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	// Start the scheduler
	c.wg.Add(1)
	go c.schedulerLoop(ctx)

	c.cleanupTicker = time.NewTicker(CleanupInterval)
	c.wg.Add(1)
	go c.cleanupLoop()

	return nil
}

// StopChecking stops periodic health checks
func (c *HTTPHealthChecker) StopChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	close(c.stopCh)

	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}

	c.wg.Wait()
	c.running = false

	return nil
}
func (c *HTTPHealthChecker) cleanupLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case <-c.cleanupTicker.C:
			c.performCleanup()
		}
	}
}

// performCleanup checks for stale entries in circuit breaker and status tracker
func (c *HTTPHealthChecker) performCleanup() {
	endpoints, err := c.repository.GetAll(context.Background())
	if err != nil {
		return
	}

	// Only perform cleanup if we have endpoints to check against
	if len(endpoints) == 0 {
		return
	}

	// Build current endpoints set more efficiently
	currentEndpoints := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		currentEndpoints[endpoint.URL.String()] = struct{}{}
	}

	// Clean up circuit breaker stale entries
	circuitEndpoints := c.circuitBreaker.GetActiveEndpoints()
	for _, url := range circuitEndpoints {
		if _, exists := currentEndpoints[url]; !exists {
			c.circuitBreaker.CleanupEndpoint(url)
		}
	}

	// Clean up status tracker stale entries
	statusEndpoints := c.statusTracker.GetActiveEndpoints()
	for _, url := range statusEndpoints {
		if _, exists := currentEndpoints[url]; !exists {
			c.statusTracker.CleanupEndpoint(url)
		}
	}
}

// worker processes health check jobs from the job channel
func (c *HTTPHealthChecker) worker() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case job := <-c.jobCh:
			c.processHealthCheck(job)
		}
	}
}

// processHealthCheck performs the actual health check and updates status
func (c *HTTPHealthChecker) processHealthCheck(job healthCheckJob) {
	result, err := c.Check(job.ctx, job.endpoint)

	// Update endpoint state
	job.endpoint.Status = result.Status
	job.endpoint.LastChecked = time.Now()
	job.endpoint.LastLatency = result.Latency

	// Calculate next check time with backoff
	isSuccess := result.Status == domain.StatusHealthy
	nextInterval, newMultiplier := calculateBackoff(job.endpoint, isSuccess)

	if !isSuccess {
		job.endpoint.ConsecutiveFailures++
		job.endpoint.BackoffMultiplier = newMultiplier
	} else {
		job.endpoint.ConsecutiveFailures = 0
		job.endpoint.BackoffMultiplier = 1
	}

	job.endpoint.NextCheckTime = time.Now().Add(nextInterval)

	// Update repository
	if repoErr := c.repository.UpdateEndpoint(job.ctx, job.endpoint); repoErr != nil {
		c.logger.Error("Failed to update endpoint",
			"endpoint", job.endpoint.URL.String(),
			"error", repoErr)
	}

	// Smart logging - only log transitions and periodic summaries
	shouldLog, errorCount := c.statusTracker.ShouldLog(
		job.endpoint.URL.String(),
		result.Status,
		err != nil)

	if shouldLog {
		if errorCount > 0 {
			// Batch error summary
			c.logger.Warn("Endpoint health issues",
				"endpoint", job.endpoint.Name,
				"status", string(result.Status),
				"consecutive_failures", errorCount,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		} else {
			// Status transition
			c.logger.InfoHealthStatus("Endpoint status changed",
				job.endpoint.Name,
				result.Status,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		}
	}
}

// schedulerLoop manages health check timing with per-endpoint backoff
func (c *HTTPHealthChecker) schedulerLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case now := <-ticker.C:
			endpoints, err := c.repository.GetAll(ctx)
			if err != nil {
				continue
			}

			for _, endpoint := range endpoints {
				// Check if it's time for this endpoint
				if now.Before(endpoint.NextCheckTime) {
					continue
				}

				job := healthCheckJob{
					endpoint: endpoint,
					ctx:      ctx,
				}

				select {
				case c.jobCh <- job:
					// Job queued successfully
				default:
					// Queue full - extend check time slightly
					endpoint.NextCheckTime = now.Add(time.Second)
					c.repository.UpdateEndpoint(ctx, endpoint)
				}
			}
		}
	}
}

// SetWorkerCount allows configuring the number of worker goroutines
func (c *HTTPHealthChecker) SetWorkerCount(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		c.logger.Warn("Cannot change worker count while health checker is running")
		return
	}

	if count < 1 {
		count = 1
	}
	c.workerCount = count
}

// GetSchedulerStats returns statistics about the health check scheduler
func (c *HTTPHealthChecker) GetSchedulerStats() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return map[string]interface{}{
			"running": false,
		}
	}

	queueSize := len(c.jobCh)
	queueCap := cap(c.jobCh)

	return map[string]interface{}{
		"running":      c.running,
		"worker_count": c.workerCount,
		"queue_size":   queueSize,
		"queue_cap":    queueCap,
		"queue_usage":  float64(queueSize) / float64(queueCap),
	}
}

// ForceHealthCheck forces an immediate health check for all endpoints
func (c *HTTPHealthChecker) ForceHealthCheck(ctx context.Context) error {
	if !c.running {
		return fmt.Errorf("health checker is not running")
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	for _, endpoint := range endpoints {
		job := healthCheckJob{
			endpoint: endpoint,
			ctx:      ctx,
		}

		select {
		case c.jobCh <- job:
			// Queued successfully
		default:
			return fmt.Errorf("health check queue is full")
		}
	}

	return nil
}
