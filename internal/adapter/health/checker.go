package health

import (
	"container/heap"
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

const (
	DefaultHealthCheckerWorkerCount = 10
	BaseHealthCheckerQueueSize      = 100
	QueueScaleFactor                = 2 // Queue size = endpoints * factor

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

// Heap-based scheduler for efficient health check timing
type scheduledCheck struct {
	endpoint *domain.Endpoint
	dueTime  time.Time
	ctx      context.Context
}

type checkHeap []*scheduledCheck

func (h checkHeap) Len() int           { return len(h) }
func (h checkHeap) Less(i, j int) bool { return h[i].dueTime.Before(h[j].dueTime) }
func (h checkHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *checkHeap) Push(x interface{}) {
	*h = append(*h, x.(*scheduledCheck))
}

func (h *checkHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type healthCheckJob struct {
	endpoint *domain.Endpoint
	ctx      context.Context
}

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

	// Heap-based scheduler
	schedulerHeap *checkHeap
	heapMu        sync.Mutex
}

func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *logger.StyledLogger) *HTTPHealthChecker {
	heapInstance := &checkHeap{}
	heap.Init(heapInstance)

	return &HTTPHealthChecker{
		repository: repository,
		client: &http.Client{
			Timeout: DefaultHealthCheckerTimeout,
		},
		circuitBreaker: NewCircuitBreaker(),
		statusTracker:  NewStatusTransitionTracker(),
		stopCh:         make(chan struct{}),
		workerCount:    DefaultHealthCheckerWorkerCount,
		logger:         logger,
		schedulerHeap:  heapInstance,
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

// Status logic: offline for network errors, busy for slow responses, healthy otherwise
func determineStatus(statusCode int, latency time.Duration, err error, errorType domain.HealthCheckErrorType) domain.EndpointStatus {
	if err != nil {
		switch errorType {
		case domain.ErrorTypeNetwork, domain.ErrorTypeTimeout, domain.ErrorTypeCircuitOpen:
			return domain.StatusOffline
		default:
			return domain.StatusUnhealthy
		}
	}

	if statusCode >= HealthyEndpointStatusRangeStart && statusCode < HealthyEndpointStatusRangeEnd {
		if latency > SlowResponseThreshold {
			return domain.StatusBusy
		}
		return domain.StatusHealthy
	}

	if latency > SlowResponseThreshold {
		return domain.StatusBusy
	}
	return domain.StatusUnhealthy
}

func calculateBackoff(endpoint *domain.Endpoint, success bool) (time.Duration, int) {
	if success {
		return endpoint.CheckInterval, 1
	}

	// Double the backoff up to max
	multiplier := endpoint.BackoffMultiplier * 2
	if multiplier > MaxBackoffMultiplier {
		multiplier = MaxBackoffMultiplier
	}

	backoffInterval := endpoint.CheckInterval * time.Duration(multiplier)
	return backoffInterval, multiplier
}

func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	start := time.Now()
	// Use pre-computed URL string to avoid allocations
	healthCheckUrl := endpoint.GetHealthCheckURLString()

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

// Scale queue size based on endpoint count
func (c *HTTPHealthChecker) calculateQueueSize(endpointCount int) int {
	queueSize := endpointCount * QueueScaleFactor
	if queueSize < BaseHealthCheckerQueueSize {
		queueSize = BaseHealthCheckerQueueSize
	}
	return queueSize
}

func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Get endpoint count to scale queue size
	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints for queue sizing: %w", err)
	}

	queueSize := c.calculateQueueSize(len(endpoints))
	c.stopCh = make(chan struct{})
	c.jobCh = make(chan healthCheckJob, queueSize)
	c.running = true

	c.logger.Info("Health checker starting",
		"workers", c.workerCount,
		"queue_size", queueSize,
		"endpoints", len(endpoints))

	// Start workers
	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	// Start heap-based scheduler
	c.wg.Add(1)
	go c.heapSchedulerLoop(ctx)

	c.cleanupTicker = time.NewTicker(CleanupInterval)
	c.wg.Add(1)
	go c.cleanupLoop()

	return nil
}

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

// Clean up stale circuit breaker and status tracker entries
func (c *HTTPHealthChecker) performCleanup() {
	endpoints, err := c.repository.GetAll(context.Background())
	if err != nil {
		return
	}

	if len(endpoints) == 0 {
		return
	}

	currentEndpoints := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		currentEndpoints[endpoint.GetURLString()] = struct{}{}
	}

	// Clean circuit breaker
	circuitEndpoints := c.circuitBreaker.GetActiveEndpoints()
	for _, url := range circuitEndpoints {
		if _, exists := currentEndpoints[url]; !exists {
			c.circuitBreaker.CleanupEndpoint(url)
		}
	}

	// Clean status tracker
	statusEndpoints := c.statusTracker.GetActiveEndpoints()
	for _, url := range statusEndpoints {
		if _, exists := currentEndpoints[url]; !exists {
			c.statusTracker.CleanupEndpoint(url)
		}
	}
}

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

func (c *HTTPHealthChecker) processHealthCheck(job healthCheckJob) {
	result, err := c.Check(job.ctx, job.endpoint)

	job.endpoint.Status = result.Status
	job.endpoint.LastChecked = time.Now()
	job.endpoint.LastLatency = result.Latency

	// Calculate backoff
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

	// Reschedule in heap
	c.heapMu.Lock()
	heap.Push(c.schedulerHeap, &scheduledCheck{
		endpoint: job.endpoint,
		dueTime:  job.endpoint.NextCheckTime,
		ctx:      job.ctx,
	})
	c.heapMu.Unlock()

	if repoErr := c.repository.UpdateEndpoint(job.ctx, job.endpoint); repoErr != nil {
		c.logger.Error("Failed to update endpoint",
			"endpoint", job.endpoint.GetURLString(),
			"error", repoErr)
	}

	// Only log status changes and periodic error summaries
	shouldLog, errorCount := c.statusTracker.ShouldLog(
		job.endpoint.GetURLString(),
		result.Status,
		err != nil)

	if shouldLog {
		if errorCount > 0 ||
			(result.Status == domain.StatusOffline ||
				result.Status == domain.StatusBusy ||
				result.Status == domain.StatusUnhealthy) {
			c.logger.WarnWithEndpoint("Endpoint health issues for", job.endpoint.Name,
				"status", result.Status.String(),
				"consecutive_failures", errorCount,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		} else {
			c.logger.InfoHealthStatus("Endpoint status changed for",
				job.endpoint.Name,
				result.Status,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		}
	}
}

// Heap-based scheduler - much more efficient than linear scanning
func (c *HTTPHealthChecker) heapSchedulerLoop(ctx context.Context) {
	defer c.wg.Done()

	// Initial population of heap
	endpoints, err := c.repository.GetAll(ctx)
	if err == nil {
		c.heapMu.Lock()
		for _, endpoint := range endpoints {
			heap.Push(c.schedulerHeap, &scheduledCheck{
				endpoint: endpoint,
				dueTime:  endpoint.NextCheckTime,
				ctx:      ctx,
			})
		}
		c.heapMu.Unlock()
	}

	ticker := time.NewTicker(100 * time.Millisecond) // Check more frequently for heap
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case now := <-ticker.C:
			c.heapMu.Lock()

			// Process all due checks
			for c.schedulerHeap.Len() > 0 {
				next := (*c.schedulerHeap)[0]
				if now.Before(next.dueTime) {
					break // Next check isn't due yet
				}

				check := heap.Pop(c.schedulerHeap).(*scheduledCheck)

				job := healthCheckJob{
					endpoint: check.endpoint,
					ctx:      check.ctx,
				}

				select {
				case c.jobCh <- job:
					// Queued
				default:
					// Queue full, reschedule in 1 second
					check.dueTime = now.Add(time.Second)
					heap.Push(c.schedulerHeap, check)
				}
			}

			c.heapMu.Unlock()
		}
	}
}

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

	c.heapMu.Lock()
	heapSize := c.schedulerHeap.Len()
	c.heapMu.Unlock()

	return map[string]interface{}{
		"running":       c.running,
		"worker_count":  c.workerCount,
		"queue_size":    queueSize,
		"queue_cap":     queueCap,
		"queue_usage":   float64(queueSize) / float64(queueCap),
		"scheduled_checks": heapSize,
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

	for _, endpoint := range endpoints {
		job := healthCheckJob{
			endpoint: endpoint,
			ctx:      ctx,
		}

		select {
		case c.jobCh <- job:
			// Queued
		default:
			return fmt.Errorf("health check queue is full")
		}
	}

	return nil
}