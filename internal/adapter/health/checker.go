package health

import (
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/thushan/olla/theme"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

const (
	DefaultHealthCheckerWorkerCount = 10  // Default number of worker goroutines for heath checks
	DefaultHealthCheckerQueueSize   = 100 // Default size of the job queue

	DefaultHealthCheckerInterval = 1 * time.Second // Default interval between health checks
	DefaultHealthCheckerTimeout  = 5 * time.Second // Default timeout for health checks

	HealthyEndpointStatusRangeStart = 200 // Start of the healthy status code range
	HealthyEndpointStatusRangeEnd   = 300 // End of the healthy status code range
)

// healthCheckJob represents a health check taks
type healthCheckJob struct {
	endpoint *domain.Endpoint
	ctx      context.Context
}

// HTTPHealthChecker implements domain.HealthChecker for HTTP health checks
type HTTPHealthChecker struct {
	repository  domain.EndpointRepository
	client      *http.Client
	stopCh      chan struct{}
	jobCh       chan healthCheckJob
	wg          sync.WaitGroup
	mu          sync.Mutex
	running     bool
	workerCount int
	logger      *slog.Logger
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(repository domain.EndpointRepository) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		repository: repository,
		client: &http.Client{
			Timeout: DefaultHealthCheckerTimeout,
		},
		stopCh:      make(chan struct{}),
		jobCh:       make(chan healthCheckJob, DefaultHealthCheckerQueueSize),
		workerCount: DefaultHealthCheckerWorkerCount,
	}
}

// Check performs a health check on the endpoint and returns its status
func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.EndpointStatus, error) {
	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, endpoint.HealthCheckURL.String(), nil)
	if err != nil {
		return domain.StatusUnhealthy, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return domain.StatusUnhealthy, fmt.Errorf("health check request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Check the response status code
	// bit tricky here, as we want to consider 2xx as healthy
	// TODO: add more sophisticated status code handling
	if resp.StatusCode >= HealthyEndpointStatusRangeStart && resp.StatusCode < HealthyEndpointStatusRangeEnd {
		return domain.StatusHealthy, nil
	}

	return domain.StatusUnhealthy, fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
}

// StartChecking begins periodic health checks for all endpoints
func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil // Already running, ignore this one
	}

	c.running = true

	// Start worker goroutines
	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	// Start the scheduler
	c.wg.Add(1)
	go c.scheduler(ctx)

	return nil
}

// StopChecking stops periodic health checks
func (c *HTTPHealthChecker) StopChecking(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil // Not running, so ignore here
	}

	close(c.stopCh)
	c.wg.Wait()

	// Reset channels for potential restart
	c.stopCh = make(chan struct{})
	c.jobCh = make(chan healthCheckJob, DefaultHealthCheckerQueueSize)
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
	if err != nil {
		// Log error but we can continue
		c.logger.Error(fmt.Sprintf("Health check failed for %s: %v", pterm.Style{theme.Default().Endpoint}.Sprintf(job.endpoint.URL.String())), err)
	}

	if err := c.repository.UpdateStatus(job.ctx, job.endpoint.URL, status); err != nil {
		c.logger.Error(fmt.Sprintf("Failed to update endpoint status for %s: %v", pterm.Style{theme.Default().Endpoint}.Sprintf(job.endpoint.URL.String())), err)
	}
}

// scheduler manages the timing of health checks and queues jobs
func (c *HTTPHealthChecker) scheduler(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(DefaultHealthCheckerInterval * time.Second)
	defer ticker.Stop()

	// Track next check time for each endpoint
	nextChecks := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case now := <-ticker.C:
			endpoints, err := c.repository.GetAll(ctx)
			if err != nil {
				// Log error but continue
				c.logger.Error("Failed to get endpoints: %v\n", err)
				continue
			}

			// Check each endpoint if it's time and queue the jab
			for _, endpoint := range endpoints {
				key := endpoint.URL.String()
				nextCheck, exists := nextChecks[key]

				// If it's not time to check this endpoint yet, skip it
				if exists && now.Before(nextCheck) {
					// Log that we're skipping this check
					// TODO: remove
					c.logger.Debug(fmt.Sprintf("Skipping health check for %s, next check at %s", pterm.Style{theme.Default().Endpoint}.Sprintf(endpoint.URL.String()), nextCheck))
					continue
				}

				// Schedule next check based on the endpoint's check interval
				nextChecks[key] = now.Add(endpoint.CheckInterval)

				job := healthCheckJob{
					endpoint: endpoint,
					ctx:      ctx,
				}

				select {
				case c.jobCh <- job:
					// Job queued successfully
					// TODO: remove
					c.logger.Debug(fmt.Sprintf("Health check queued for %s", pterm.Style{theme.Default().Endpoint}.Sprintf(endpoint.URL.String())))
				default:
					// Job channel is full, log and skip this check
					// probably not a good idea to log this every time with lots of endpoints
					c.logger.Warn(fmt.Sprintf("Health check queue full, skipping check for %s", pterm.Style{theme.Default().Endpoint}.Sprintf(endpoint.URL.String())))
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

	// Get current queue status
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

// ForceHealthCheck forces an immediate health check for all endpoints (useful for testing)
func (c *HTTPHealthChecker) ForceHealthCheck(ctx context.Context) error {
	if !c.running {
		return fmt.Errorf("health checker is not running")
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpants: %w", err)
	}

	for _, endpoint := range endpoints {
		job := healthCheckJob{
			endpoint: endpoint,
			ctx:      ctx,
		}

		select {
		case c.jobCh <- job:
			c.logger.Info(fmt.Sprintf("Forced health check queued for %s", pterm.Style{theme.Default().Endpoint}.Sprintf(endpoint.URL.String())))
		default:
			return fmt.Errorf("health check queue is full")
		}
	}

	return nil
}
