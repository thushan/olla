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
	scheduler       *HealthScheduler
	workerPool      *WorkerPool
	statsCollector  *StatsCollector
	circuitBreaker  *CircuitBreaker
	statusTracker   *StatusTransitionTracker
	repository    domain.EndpointRepository
	cleanupTicker *time.Ticker
	logger        *logger.StyledLogger
	mu      sync.Mutex
	running bool
}

// NewHTTPHealthChecker creates a health checker with the provided HTTP client, useful for testing or custom clients
func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *logger.StyledLogger, client HTTPClient) *HTTPHealthChecker {
	circuitBreaker := NewCircuitBreaker()
	statusTracker := NewStatusTransitionTracker()

	healthClient := NewHealthClient(client, circuitBreaker)

	workerPool := NewWorkerPool(
		DefaultHealthCheckerWorkerCount,
		BaseHealthCheckerQueueSize,
		healthClient,
		repository,
		statusTracker,
		logger,
	)

	scheduler := NewHealthScheduler(workerPool.GetJobChannel())

	statsCollector := NewStatsCollector(
		workerPool,
		scheduler,
		circuitBreaker,
		statusTracker,
		repository,
	)

	return &HTTPHealthChecker{
		healthClient:   healthClient,
		scheduler:      scheduler,
		workerPool:     workerPool,
		statsCollector: statsCollector,
		circuitBreaker: circuitBreaker,
		statusTracker:  statusTracker,
		repository:     repository,
		logger:         logger,
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

// Scale queue size based on endpoint count - extracted logic maintained
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

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints for queue sizing: %w", err)
	}

	queueSize := c.calculateQueueSize(len(endpoints))
	c.running = true
	c.statsCollector.SetRunning(true)

	c.logger.Info("Health checker starting",
		"workers", DefaultHealthCheckerWorkerCount,
		"queue_size", queueSize,
		"endpoints", len(endpoints))

	c.workerPool.Start(c.scheduler)

	c.scheduler.Start(ctx, c.repository)

	c.cleanupTicker = time.NewTicker(CleanupInterval)
	go c.cleanupLoop()

	return nil
}

func (c *HTTPHealthChecker) StopChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.scheduler.Stop()

	c.workerPool.Stop()

	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}

	c.running = false
	c.statsCollector.SetRunning(false)

	return nil
}

func (c *HTTPHealthChecker) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.performCleanup()
		}
	}
}

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

	// Note: In a full refactor, we'd recreate the worker pool with new count
	// For now, we maintain the existing behavior of just logging the warning
}

func (c *HTTPHealthChecker) GetSchedulerStats() map[string]interface{} {
	return c.statsCollector.GetSchedulerStats()
}

func (c *HTTPHealthChecker) ForceHealthCheck(ctx context.Context) error {
	if !c.running {
		return fmt.Errorf("health checker is not running")
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	jobCh := c.workerPool.GetJobChannel()

	for _, endpoint := range endpoints {
		job := healthCheckJob{
			endpoint: endpoint,
			ctx:      ctx,
		}

		select {
		case jobCh <- job:
			// Queued
		default:
			return fmt.Errorf("health check queue is full")
		}
	}

	return nil
}