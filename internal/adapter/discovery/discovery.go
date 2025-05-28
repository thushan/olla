package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/logger"
	"net/url"
	"sync"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

type StaticDiscoveryService struct {
	repository           domain.EndpointRepository
	checker              domain.HealthChecker
	configMu             sync.RWMutex
	endpointUpdateMu     sync.Mutex
	config               *config.Config
	initialHealthTimeout time.Duration
	logger               *logger.StyledLogger
	configLoader         func() *config.Config
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
	config *config.Config,
	logger *logger.StyledLogger,
) *StaticDiscoveryService {
	return &StaticDiscoveryService{
		repository:           repository,
		checker:              checker,
		config:               config,
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
	// Try routable endpoints first
	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	if len(routable) > 0 {
		return routable, nil
	}

	// Fallback to all endpoints
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
	s.endpointUpdateMu.Lock()
	defer s.endpointUpdateMu.Unlock()

	cfg := s.getConfig()

	currentEndpoints, err := s.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current endpoints: %w", err)
	}

	currentMap := make(map[string]*domain.Endpoint)
	for _, endpoint := range currentEndpoints {
		currentMap[endpoint.GetURLString()] = endpoint
	}

	for _, endpointCfg := range cfg.Discovery.Static.Endpoints {
		if err := validateEndpointConfig(endpointCfg); err != nil {
			s.logger.Error("Invalid endpoint configuration",
				"name", endpointCfg.Name,
				"url", endpointCfg.URL,
				"error", err)
			continue
		}

		endpointURL, err := url.Parse(endpointCfg.URL)
		if err != nil {
			return fmt.Errorf("invalid endpoint URL %s: %w", endpointCfg.URL, err)
		}

		healthCheckPath, err := url.Parse(endpointCfg.HealthCheckURL)
		if err != nil {
			return fmt.Errorf("invalid health check URL %s: %w", endpointCfg.HealthCheckURL, err)
		}

		modelPath, err := url.Parse(endpointCfg.ModelURL)
		if err != nil {
			return fmt.Errorf("invalid model URL %s: %w", endpointCfg.ModelURL, err)
		}

		// Resolve URLs properly
		healthCheckURL := endpointURL.ResolveReference(healthCheckPath)
		modelUrl := endpointURL.ResolveReference(modelPath)

		key := endpointURL.String()
		if existing, exists := currentMap[key]; exists {
			configChanged := s.hasEndpointConfigChanged(existing, endpointCfg, healthCheckURL)
			oldName := existing.Name
			hasNameChanged := existing.Name != endpointCfg.Name

			s.updateExistingEndpoint(existing, endpointCfg, healthCheckURL, modelUrl, configChanged, hasNameChanged, oldName)
			delete(currentMap, key)
		} else {
			endpoint, err := domain.NewEndpoint(
				endpointCfg.Name,
				endpointCfg.URL,
				healthCheckURL.String(),
				modelUrl.String(),
				endpointCfg.Priority,
				endpointCfg.CheckInterval,
				endpointCfg.CheckTimeout,
			)
			if err != nil {
				return fmt.Errorf("failed to create endpoint %s: %w", key, err)
			}

			if err := s.repository.Add(ctx, endpoint); err != nil {
				return fmt.Errorf("failed to add endpoint %s: %w", key, err)
			}

			s.logger.Info("Added new endpoint", "name", endpoint.Name,
				"endpoint", endpoint.GetURLString(),
				"model_url", endpoint.ModelURLString,
				"health_check_url", endpoint.HealthCheckURLString)
		}
	}

	// Remove endpoints no longer in config
	for key, endpoint := range currentMap {
		if err := s.repository.Remove(ctx, endpoint.URL); err != nil {
			return fmt.Errorf("failed to remove endpoint %s: %w", key, err)
		}
		s.logger.InfoWithEndpoint("Removed endpoint", endpoint.Name)
	}

	return nil
}

func (s *StaticDiscoveryService) updateExistingEndpoint(
	existing *domain.Endpoint,
	cfg config.EndpointConfig,
	healthCheckURL, modelUrl *url.URL,
	configChanged, hasNameChanged bool,
	oldName string,
) {
	existing.Name = cfg.Name
	existing.Priority = cfg.Priority
	existing.HealthCheckURL = healthCheckURL
	existing.ModelUrl = modelUrl
	existing.CheckInterval = cfg.CheckInterval
	existing.CheckTimeout = cfg.CheckTimeout
	existing.HealthCheckURLString = healthCheckURL.String()
	existing.ModelURLString = modelUrl.String()

	if configChanged {
		if hasNameChanged {
			s.logger.InfoWithEndpoint("Endpoint configuration changed for", oldName, "to", cfg.Name)
		} else {
			s.logger.InfoWithEndpoint("Endpoint configuration changed for", oldName)
		}

		existing.Status = domain.StatusUnknown
		existing.LastChecked = time.Now()
		existing.ConsecutiveFailures = 0
		existing.BackoffMultiplier = 1
		existing.NextCheckTime = time.Now()
	}
}

func (s *StaticDiscoveryService) hasEndpointConfigChanged(existing *domain.Endpoint, cfg config.EndpointConfig, healthCheckURL *url.URL) bool {
	return existing.Name != cfg.Name ||
		existing.Priority != cfg.Priority ||
		existing.HealthCheckURLString != healthCheckURL.String() ||
		existing.CheckInterval != cfg.CheckInterval ||
		existing.CheckTimeout != cfg.CheckTimeout
}

// Batch health checks for better performance
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

	// Smaller batches to prevent goroutine explosion
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
