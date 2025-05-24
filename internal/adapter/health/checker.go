package health

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

const (
	DefaultHealthCheckerWorkerCount = 10
	DefaultHealthCheckerQueueSize   = 100

	DefaultHealthCheckerInterval = 1 * time.Second
	DefaultHealthCheckerTimeout  = 5 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 5
	DefaultCircuitBreakerTimeout   = 30 * time.Second
)

var (
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)

// HTTPClient interface for better testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CircuitBreaker tracks failure rates and prevents cascading failures
type CircuitBreaker struct {
	mu               sync.RWMutex
	endpoints        map[string]*circuitState
	failureThreshold int
	timeout          time.Duration
}

type circuitState struct {
	failures    int64
	lastFailure time.Time
	isOpen      bool
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		endpoints:        make(map[string]*circuitState),
		failureThreshold: DefaultCircuitBreakerThreshold,
		timeout:          DefaultCircuitBreakerTimeout,
	}
}

func (cb *CircuitBreaker) IsOpen(endpointURL string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	state, exists := cb.endpoints[endpointURL]
	if !exists {
		return false
	}

	if state.isOpen && time.Since(state.lastFailure) > cb.timeout {
		state.isOpen = false
		state.failures = 0
	}

	return state.isOpen
}

func (cb *CircuitBreaker) RecordSuccess(endpointURL string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if state, exists := cb.endpoints[endpointURL]; exists {
		state.failures = 0
		state.isOpen = false
	}
}

func (cb *CircuitBreaker) RecordFailure(endpointURL string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state, exists := cb.endpoints[endpointURL]
	if !exists {
		state = &circuitState{}
		cb.endpoints[endpointURL] = state
	}

	atomic.AddInt64(&state.failures, 1)
	state.lastFailure = time.Now()

	if state.failures >= int64(cb.failureThreshold) {
		state.isOpen = true
	}
}

// HealthCheckMetrics tracks health check statistics
type HealthCheckMetrics struct {
	mu               sync.RWMutex
	totalChecks      int64
	successfulChecks int64
	failedChecks     int64
	totalLatency     time.Duration
	lastCheckTime    time.Time
}

func (m *HealthCheckMetrics) RecordCheck(latency time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalChecks++
	m.totalLatency += latency
	m.lastCheckTime = time.Now()

	if success {
		m.successfulChecks++
	} else {
		m.failedChecks++
	}
}

func (m *HealthCheckMetrics) GetStats() (total, successful, failed int64, avgLatency time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.totalChecks > 0 {
		avgLatency = m.totalLatency / time.Duration(m.totalChecks)
	}

	return m.totalChecks, m.successfulChecks, m.failedChecks, avgLatency
}

// scheduledCheck represents a health check scheduled for a specific time
type scheduledCheck struct {
	endpoint *domain.Endpoint
	nextTime time.Time
	index    int
}

// checkScheduler manages efficient scheduling of health checks using a heap
type checkScheduler struct {
	heap []*scheduledCheck
	mu   sync.Mutex
}

func (h *checkScheduler) Len() int { return len(h.heap) }

func (h *checkScheduler) Less(i, j int) bool {
	return h.heap[i].nextTime.Before(h.heap[j].nextTime)
}

func (h *checkScheduler) Swap(i, j int) {
	h.heap[i], h.heap[j] = h.heap[j], h.heap[i]
	h.heap[i].index = i
	h.heap[j].index = j
}

func (h *checkScheduler) Push(x interface{}) {
	n := len(h.heap)
	item := x.(*scheduledCheck)
	item.index = n
	h.heap = append(h.heap, item)
}

func (h *checkScheduler) Pop() interface{} {
	old := h.heap
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	h.heap = old[0 : n-1]
	return item
}

func (s *checkScheduler) schedule(endpoint *domain.Endpoint, when time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	check := &scheduledCheck{
		endpoint: endpoint,
		nextTime: when,
	}

	heap.Push(s, check)
}

func (s *checkScheduler) nextCheck() (*scheduledCheck, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.heap) == 0 {
		return nil, time.Hour
	}

	next := s.heap[0]
	waitTime := time.Until(next.nextTime)
	if waitTime <= 0 {
		return heap.Pop(s).(*scheduledCheck), 0
	}

	return nil, waitTime
}

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
	metrics        map[string]*HealthCheckMetrics
	scheduler      *checkScheduler
	stopCh         chan struct{}
	jobCh          chan healthCheckJob
	wg             sync.WaitGroup
	mu             sync.Mutex
	running        bool
	workerCount    int
	logger         *slog.Logger
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *slog.Logger) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		repository: repository,
		client: &http.Client{
			Timeout: DefaultHealthCheckerTimeout,
		},
		circuitBreaker: NewCircuitBreaker(),
		metrics:        make(map[string]*HealthCheckMetrics),
		scheduler:      &checkScheduler{heap: make([]*scheduledCheck, 0)},
		stopCh:         make(chan struct{}),
		jobCh:          make(chan healthCheckJob, DefaultHealthCheckerQueueSize),
		workerCount:    DefaultHealthCheckerWorkerCount,
		logger:         logger,
	}
}

func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// Check performs a health check on the endpoint and returns its status
func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.EndpointStatus, error) {
	start := time.Now()
	endpointURL := endpoint.URL.String()

	if c.circuitBreaker.IsOpen(endpointURL) {
		c.recordMetrics(endpointURL, time.Since(start), false)
		return domain.StatusUnhealthy, ErrCircuitBreakerOpen
	}

	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, endpoint.HealthCheckURL.String(), nil)
	if err != nil {
		c.circuitBreaker.RecordFailure(endpointURL)
		c.recordMetrics(endpointURL, time.Since(start), false)
		return domain.StatusUnhealthy, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.circuitBreaker.RecordFailure(endpointURL)
		c.recordMetrics(endpointURL, time.Since(start), false)

		if isNetworkError(err) {
			return domain.StatusUnhealthy, fmt.Errorf("network error: %w", err)
		}
		return domain.StatusUnhealthy, fmt.Errorf("request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	success := resp.StatusCode >= HealthyEndpointStatusRangeStart && resp.StatusCode < HealthyEndpointStatusRangeEnd
	c.recordMetrics(endpointURL, time.Since(start), success)

	if success {
		c.circuitBreaker.RecordSuccess(endpointURL)
		return domain.StatusHealthy, nil
	}

	c.circuitBreaker.RecordFailure(endpointURL)
	return domain.StatusUnhealthy, fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
}

func (c *HTTPHealthChecker) recordMetrics(endpointURL string, latency time.Duration, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.metrics[endpointURL]; !exists {
		c.metrics[endpointURL] = &HealthCheckMetrics{}
	}

	c.metrics[endpointURL].RecordCheck(latency, success)
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
	c.wg.Wait()
	c.running = false

	return nil
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
	status, err := c.Check(job.ctx, job.endpoint)
	if err != nil && !errors.Is(err, ErrCircuitBreakerOpen) {
		c.logger.Error("Health check failed",
			"endpoint", job.endpoint.URL.String(),
			"error", err)
	}

	if err := c.repository.UpdateStatus(job.ctx, job.endpoint.URL, status); err != nil {
		c.logger.Error("Failed to update endpoint status",
			"endpoint", job.endpoint.URL.String(),
			"error", err)
	}
}

// schedulerLoop manages the timing of health checks using efficient heap-based scheduling
func (c *HTTPHealthChecker) schedulerLoop(ctx context.Context) {
	defer c.wg.Done()

	// Initial scheduling
	c.scheduleAllEndpoints(ctx)

	for {
		check, waitTime := c.scheduler.nextCheck()

		if check == nil {
			// No checks scheduled, wait and refresh
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(waitTime):
				c.scheduleAllEndpoints(ctx)
				continue
			}
		}

		// Wait until it's time for the next check
		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(waitTime):
			}
		}

		// Queue the health check job
		job := healthCheckJob{
			endpoint: check.endpoint,
			ctx:      ctx,
		}

		select {
		case c.jobCh <- job:
			// Schedule next check for this endpoint
			nextTime := time.Now().Add(check.endpoint.CheckInterval)
			c.scheduler.schedule(check.endpoint, nextTime)
		default:
			c.logger.Warn("Health check queue full, skipping check",
				"endpoint", check.endpoint.URL.String())
			// Reschedule for immediate retry
			c.scheduler.schedule(check.endpoint, time.Now().Add(time.Second))
		}
	}
}

func (c *HTTPHealthChecker) scheduleAllEndpoints(ctx context.Context) {
	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		c.logger.Error("Failed to get endpoints for scheduling", "error", err)
		return
	}

	now := time.Now()
	for _, endpoint := range endpoints {
		c.scheduler.schedule(endpoint, now)
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

// GetHealthCheckMetrics returns metrics for all endpoints
func (c *HTTPHealthChecker) GetHealthCheckMetrics() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]interface{})

	for endpoint, metrics := range c.metrics {
		total, successful, failed, avgLatency := metrics.GetStats()
		result[endpoint] = map[string]interface{}{
			"total_checks":      total,
			"successful_checks": successful,
			"failed_checks":     failed,
			"average_latency":   avgLatency.String(),
			"success_rate":      float64(successful) / float64(total) * 100,
		}
	}

	return result
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
			c.logger.Info("Forced health check queued",
				"endpoint", endpoint.URL.String())
		default:
			return fmt.Errorf("health check queue is full")
		}
	}

	return nil
}
