package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/logger"
	"sync"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

type StaticDiscoveryService struct {
	repository           domain.EndpointRepository
	checker              domain.HealthChecker
	endpoints            []config.EndpointConfig
	initialHealthTimeout time.Duration
	logger               *logger.StyledLogger
}

const (
	DefaultInitialHealthTimeout  = 30 * time.Second
	DefaultWaitForHealthyTimeout = 30 * time.Second
	MinHealthCheckInterval       = time.Second
	MaxHealthCheckTimeout        = 30 * time.Second
)

func NewStaticDiscoveryService(
	repository domain.EndpointRepository,
	checker domain.HealthChecker,
	endpoints []config.EndpointConfig,
	logger *logger.StyledLogger,
) *StaticDiscoveryService {
	return &StaticDiscoveryService{
		repository:           repository,
		checker:              checker,
		endpoints:            endpoints,
		logger:               logger,
		initialHealthTimeout: DefaultInitialHealthTimeout,
	}
}

func (s *StaticDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetAll(ctx)
}

func (s *StaticDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetHealthy(ctx)
}

func (s *StaticDiscoveryService) GetRoutableEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetRoutable(ctx)
}

func (s *StaticDiscoveryService) GetHealthyEndpointsWithFallback(ctx context.Context) ([]*domain.Endpoint, error) {
	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	if len(routable) > 0 {
		return routable, nil
	}

	s.logger.Warn("No routable endpoints available, falling back to all endpoints")
	all, err := s.repository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no endpoints configured")
	}

	return all, nil
}

func (s *StaticDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	_, err := s.repository.UpsertFromConfig(ctx, s.endpoints)
	return err
}

func (s *StaticDiscoveryService) performInitialHealthChecks(ctx context.Context) error {
	s.logger.Info("Performing initial health checks...")

	checkCtx, cancel := context.WithTimeout(ctx, s.initialHealthTimeout)
	defer cancel()

	endpoints, err := s.repository.GetAll(checkCtx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints for initial health check: %w", err)
	}

	endpointCount := len(endpoints)
	if endpointCount == 0 {
		s.logger.Warn("No endpoints configured for health checking")
		return nil
	}

	s.logger.InfoWithCount("Health checking Endpoints", endpointCount)

	const batchSize = 5
	batches := make([][]*domain.Endpoint, 0, (endpointCount+batchSize-1)/batchSize)
	for i := 0; i < len(endpoints); i += batchSize {
		end := i + batchSize
		if end > len(endpoints) {
			end = len(endpoints)
		}
		batches = append(batches, endpoints[i:end])
	}

	healthCheckResults := make(chan struct {
		endpoint *domain.Endpoint
		result   domain.HealthCheckResult
	}, len(endpoints))

	var wg sync.WaitGroup

	for _, batch := range batches {
		wg.Add(1)
		go func(batch []*domain.Endpoint) {
			defer wg.Done()
			for _, endpoint := range batch {
				s.logger.InfoWithEndpoint(" Checking", endpoint.Name, "url", endpoint.HealthCheckURLString)

				result, _ := s.checker.Check(checkCtx, endpoint)
				healthCheckResults <- struct {
					endpoint *domain.Endpoint
					result   domain.HealthCheckResult
				}{endpoint, result}
			}
		}(batch)
	}

	wg.Wait()
	close(healthCheckResults)

	statusCounts := make(map[domain.EndpointStatus]int)
	for resultData := range healthCheckResults {
		endpoint := resultData.endpoint
		result := resultData.result

		endpoint.Status = result.Status
		endpoint.LastChecked = time.Now()
		endpoint.LastLatency = result.Latency

		if err := s.repository.UpdateEndpoint(checkCtx, endpoint); err != nil {
			s.logger.ErrorWithEndpoint("Failed to update endpoint status", endpoint.GetURLString(), "error", err)
		}

		statusCounts[result.Status]++
	}

	s.logger.Info("Initial health check results:")
	for _, endpoint := range endpoints {
		s.logger.InfoHealthStatus("", endpoint.Name, endpoint.Status,
			"latency", endpoint.LastLatency.Round(time.Millisecond))
	}

	healthy := statusCounts[domain.StatusHealthy]
	routable := healthy + statusCounts[domain.StatusBusy] + statusCounts[domain.StatusWarming]

	if routable == 0 {
		return fmt.Errorf("no routable endpoints available after initial health check")
	}

	s.logger.Info("Health check summary",
		"healthy", healthy,
		"busy", statusCounts[domain.StatusBusy],
		"warming", statusCounts[domain.StatusWarming],
		"offline", statusCounts[domain.StatusOffline],
		"unhealthy", statusCounts[domain.StatusUnhealthy],
		"unknown", statusCounts[domain.StatusUnknown])

	return nil
}

func (s *StaticDiscoveryService) waitForHealthyEndpoints(ctx context.Context, maxWait time.Duration) error {
	s.logger.Info("Waiting for routable endpoints", "max_wait", maxWait)

	timeout := time.NewTimer(maxWait)
	defer timeout.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for routable endpoints: %w", ctx.Err())
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for routable endpoints after %v", maxWait)
		case <-ticker.C:
			routable, err := s.repository.GetRoutable(ctx)
			if err != nil {
				s.logger.Error("Error checking routable endpoints", "error", err)
				continue
			}

			if len(routable) > 0 {
				s.logger.Info("Found routable endpoints, ready to serve traffic",
					"count", len(routable))
				return nil
			}

			s.logger.Warn("No routable endpoints yet, waiting...")
		}
	}
}

func (s *StaticDiscoveryService) Start(ctx context.Context) error {
	s.logger.Info("Starting static discovery service...")

	if err := s.RefreshEndpoints(ctx); err != nil {
		return fmt.Errorf("failed to refresh endpoints: %w", err)
	}

	if err := s.performInitialHealthChecks(ctx); err != nil {
		s.logger.Warn("Initial health checks failed, continuing with periodic checks",
			"error", err)
	}

	if err := s.checker.StartChecking(ctx); err != nil {
		return fmt.Errorf("failed to start health checking: %w", err)
	}

	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return fmt.Errorf("failed to check routable endpoints: %w", err)
	}

	if len(routable) == 0 {
		s.logger.Info("No initially routable endpoints, waiting for periodic health checks...")

		if err := s.waitForHealthyEndpoints(ctx, DefaultWaitForHealthyTimeout); err != nil {
			s.logger.Warn("Proxy will start but may not be able to serve requests initially",
				"error", err)
		}
	}

	s.logger.Info("Static discovery service started successfully")
	return nil
}

func (s *StaticDiscoveryService) Stop(ctx context.Context) error {
	s.logger.Info("Stopping static discovery service...")

	if err := s.checker.StopChecking(ctx); err != nil {
		return fmt.Errorf("failed to stop health checking: %w", err)
	}

	s.logger.Info("Static discovery service stopped successfully")
	return nil
}

func (s *StaticDiscoveryService) SetInitialHealthTimeout(timeout time.Duration) {
	s.initialHealthTimeout = timeout
}

type EndpointStatusResponse struct {
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	Priority            int       `json:"priority"`
	Status              string    `json:"status"`
	LastChecked         time.Time `json:"last_checked"`
	LastLatency         string    `json:"last_latency"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	BackoffMultiplier   int       `json:"backoff_multiplier"`
	NextCheckTime       time.Time `json:"next_check_time"`
}

func (s *StaticDiscoveryService) GetHealthStatus(ctx context.Context) (map[string]interface{}, error) {
	all, err := s.repository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	healthy, err := s.repository.GetHealthy(ctx)
	if err != nil {
		return nil, err
	}

	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	status := make(map[string]interface{})
	status["total_endpoints"] = len(all)
	status["healthy_endpoints"] = len(healthy)
	status["routable_endpoints"] = len(routable)
	status["unhealthy_endpoints"] = len(all) - len(routable)

	endpoints := make([]EndpointStatusResponse, len(all))
	for i, endpoint := range all {
		endpoints[i] = EndpointStatusResponse{
			Name:                endpoint.Name,
			URL:                 endpoint.GetURLString(),
			Priority:            endpoint.Priority,
			Status:              endpoint.Status.String(),
			LastChecked:         endpoint.LastChecked,
			LastLatency:         endpoint.LastLatency.String(),
			ConsecutiveFailures: endpoint.ConsecutiveFailures,
			BackoffMultiplier:   endpoint.BackoffMultiplier,
			NextCheckTime:       endpoint.NextCheckTime,
		}
	}
	status["endpoints"] = endpoints
	status["cache"] = s.repository.GetCacheStats()

	return status, nil
}

func validateEndpointConfig(cfg config.EndpointConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	if cfg.HealthCheckURL == "" {
		return fmt.Errorf("health check URL cannot be empty")
	}

	if cfg.ModelURL == "" {
		return fmt.Errorf("model URL cannot be empty")
	}

	if cfg.CheckInterval < MinHealthCheckInterval {
		return fmt.Errorf("check_interval too short: minimum %v, got %v", MinHealthCheckInterval, cfg.CheckInterval)
	}

	if cfg.CheckTimeout >= cfg.CheckInterval {
		return fmt.Errorf("check_timeout (%v) must be less than check_interval (%v)", cfg.CheckTimeout, cfg.CheckInterval)
	}

	if cfg.CheckTimeout > MaxHealthCheckTimeout {
		return fmt.Errorf("check_timeout too long: maximum %v, got %v", MaxHealthCheckTimeout, cfg.CheckTimeout)
	}

	if cfg.Priority < 0 {
		return fmt.Errorf("priority must be non-negative, got %d", cfg.Priority)
	}

	return nil
}