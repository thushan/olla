package health

import (
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultHealthCheckInterval = 30 * time.Second
	LogThrottleInterval        = 2 * time.Minute
)

type HTTPHealthChecker struct {
	healthClient     *HealthClient
	repository       domain.EndpointRepository
	ticker           *time.Ticker
	stopCh           chan struct{}
	logger           *logger.StyledLogger
	isRunning        atomic.Bool
	lastLoggedStatus map[string]domain.EndpointStatus
	lastLogTime      map[string]time.Time
	logMu            sync.RWMutex
}

func NewHTTPHealthChecker(repository domain.EndpointRepository, logger *logger.StyledLogger, client HTTPClient) *HTTPHealthChecker {
	circuitBreaker := NewCircuitBreaker()
	healthClient := NewHealthClient(client, circuitBreaker)

	return &HTTPHealthChecker{
		healthClient:     healthClient,
		repository:       repository,
		logger:           logger,
		stopCh:           make(chan struct{}),
		lastLoggedStatus: make(map[string]domain.EndpointStatus),
		lastLogTime:      make(map[string]time.Time),
	}
}

func NewHTTPHealthCheckerWithDefaults(repository domain.EndpointRepository, logger *logger.StyledLogger) *HTTPHealthChecker {
	client := &http.Client{
		Timeout: DefaultHealthCheckerTimeout,
	}
	return NewHTTPHealthChecker(repository, logger, client)
}

func (c *HTTPHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	return c.healthClient.Check(ctx, endpoint)
}

func (c *HTTPHealthChecker) StartChecking(ctx context.Context) error {
	if c.isRunning.Load() {
		return nil
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints for health checking: %w", err)
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

		c.checkEndpoint(ctx, endpoint)
	}
}

func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint *domain.Endpoint) {
	result, err := c.healthClient.Check(ctx, endpoint)

	oldStatus := endpoint.Status
	newStatus := result.Status
	statusChanged := oldStatus != newStatus

	endpoint.Status = newStatus
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

	// Status change logging: ALWAYS log any status change immediately,
	// only throttle when status remains the same
	endpointKey := endpoint.GetURLString()

	c.logMu.RLock()
	lastLogTime := c.lastLogTime[endpointKey]
	c.logMu.RUnlock()

	if statusChanged {
		// ANY status change gets logged immediately - this is critical for ops
		c.logMu.Lock()
		c.lastLoggedStatus[endpointKey] = newStatus
		c.lastLogTime[endpointKey] = time.Now()
		c.logMu.Unlock()

		// Special handling for initial status discovery
		if oldStatus == domain.StatusUnknown {
			if newStatus == domain.StatusHealthy {
				c.logger.InfoHealthStatus("Endpoint initial status:",
					endpoint.Name,
					newStatus,
					"latency", result.Latency,
					"next_check_in", nextInterval)
			} else {
				// Initial discovery of unhealthy endpoint
				c.logger.WarnWithEndpoint("Endpoint discovered offline:", endpoint.Name,
					"status", newStatus.String(),
					"latency", result.Latency,
					"next_check_in", nextInterval)
			}
		} else if newStatus == domain.StatusHealthy {
			c.logger.InfoHealthStatus("Endpoint recovered:",
				endpoint.Name,
				newStatus,
				"was", oldStatus.String(),
				"latency", result.Latency,
				"next_check_in", nextInterval)
		} else {
			c.logger.WarnWithEndpoint("Endpoint status changed:", endpoint.Name,
				"status", newStatus.String(),
				"was", oldStatus.String(),
				"consecutive_failures", endpoint.ConsecutiveFailures,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		}
	} else if err != nil && time.Since(lastLogTime) > LogThrottleInterval {
		// Same status but still having issues - only log every 2 minutes to avoid spam
		c.logMu.Lock()
		c.lastLogTime[endpointKey] = time.Now()
		c.logMu.Unlock()

		c.logger.WarnWithEndpoint("Endpoint still having issues:", endpoint.Name,
			"status", newStatus.String(),
			"consecutive_failures", endpoint.ConsecutiveFailures,
			"duration", time.Since(lastLogTime).Round(time.Second),
			"next_check_in", nextInterval)
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
		return fmt.Errorf("health checker is not running")
	}

	endpoints, err := c.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	if initial {
		c.logger.Info("Initialising health checks", "endpoints", len(endpoints))
	} else {
		c.logger.Info("Running full health checks", "endpoints", len(endpoints))
	}

	c.logEndpointsTable(endpoints)

	for _, endpoint := range endpoints {
		c.checkEndpoint(ctx, endpoint)
	}

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
		health   string
		priority int
		interval int
		timeout  int
	}

	var entries []endpointEntry
	for _, info := range endpoints {
		entries = append(entries, endpointEntry{
			url:      info.URL.String(),
			name:     info.Name,
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
	tableData := [][]string{
		{"PRIORITY", "NAME", "URL", "HEALTH", "CHECK", "TIMEOUT"},
	}

	for _, entry := range entries {
		tableData = append(tableData, []string{
			fmt.Sprintf("%d", entry.priority),
			entry.name,
			entry.url,
			entry.health,
			fmt.Sprintf("%ds", entry.interval),
			fmt.Sprintf("%ds", entry.timeout),
		})
	}

	c.logger.Info(fmt.Sprintf("Registered endpoints %s", pterm.Style{c.logger.Theme.Counts}.Sprintf("(%d)", len(entries))))
	tableString, _ := pterm.DefaultTable.WithHeaderStyle(c.logger.Theme.Accent).WithHasHeader().WithData(tableData).Srender()
	fmt.Print(tableString)
}
