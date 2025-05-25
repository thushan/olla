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

// StaticDiscoveryService implements ports.DiscoveryService for static endpoints
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

// NewStaticDiscoveryService creates a new static discovery service
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

// GetEndpoints returns all registered endpoints
func (s *StaticDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetAll(ctx)
}

// GetHealthyEndpoints returns only healthy endpoints
func (s *StaticDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetHealthy(ctx)
}

// GetRoutableEndpoints returns endpoints that can receive traffic
func (s *StaticDiscoveryService) GetRoutableEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetRoutable(ctx)
}

// GetHealthyEndpointsWithFallback returns healthy endpoints with graceful degradation
func (s *StaticDiscoveryService) GetHealthyEndpointsWithFallback(ctx context.Context) ([]*domain.Endpoint, error) {
	// First try routable endpoints (healthy, busy, warming)
	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	if len(routable) > 0 {
		return routable, nil
	}

	// Fallback to all endpoints if none are routable
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

// RefreshEndpoints triggers a refresh of the endpoint list from the config
func (s *StaticDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	s.endpointUpdateMu.Lock()
	defer s.endpointUpdateMu.Unlock()

	cfg := s.getConfig()

	currentEndpoints, err := s.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current endpoints: %w", err)
	}

	// Create a map of current endpoints for quick lookup
	currentMap := make(map[string]*domain.Endpoint)
	for _, endpoint := range currentEndpoints {
		currentMap[endpoint.URL.String()] = endpoint
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

		// Get the full health check URL here to avoid having do it each time later
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
			endpoint := &domain.Endpoint{
				Name:                endpointCfg.Name,
				URL:                 endpointURL,
				Priority:            endpointCfg.Priority,
				HealthCheckURL:      healthCheckURL,
				ModelUrl:            modelUrl,
				CheckInterval:       endpointCfg.CheckInterval,
				CheckTimeout:        endpointCfg.CheckTimeout,
				Status:              domain.StatusUnknown,
				ConsecutiveFailures: 0,
				BackoffMultiplier:   1,
				NextCheckTime:       time.Now(),
			}

			if err := s.repository.Add(ctx, endpoint); err != nil {
				return fmt.Errorf("failed to add endpoint %s: %w", key, err)
			}

			s.logger.Info("Added new endpoint", "name", endpoint.Name,
				"endpoint", endpoint.URL.String(),
				"model_url", endpoint.ModelUrl.String(),
				"health_check_url", endpoint.HealthCheckURL.String())
		}
	}

	// Remove endpoints that are no longer in config
	// We do this after processing all new endpoints
	// to avoid removing ones that are being updated
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
	// Update existing endpoint
	existing.Name = cfg.Name
	existing.Priority = cfg.Priority
	existing.HealthCheckURL = healthCheckURL
	existing.ModelUrl = modelUrl
	existing.CheckInterval = cfg.CheckInterval
	existing.CheckTimeout = cfg.CheckTimeout

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
		existing.HealthCheckURL.String() != healthCheckURL.String() ||
		existing.CheckInterval != cfg.CheckInterval ||
		existing.CheckTimeout != cfg.CheckTimeout
}

// performInitialHealthChecks performs synchronous health checks on startup
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

	// Perform health checks concurrently but wait for all to complete
	var wg sync.WaitGroup
	healthCheckResults := make(chan struct {
		endpoint *domain.Endpoint
		result   domain.HealthCheckResult
	}, len(endpoints))

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep *domain.Endpoint) {
			defer wg.Done()

			s.logger.InfoWithEndpoint(" Checking", ep.Name, "url", ep.HealthCheckURL.String())

			result, _ := s.checker.Check(checkCtx, ep)
			healthCheckResults <- struct {
				endpoint *domain.Endpoint
				result   domain.HealthCheckResult
			}{ep, result}
		}(endpoint)
	}

	wg.Wait()
	close(healthCheckResults)

	// Process results
	statusCounts := make(map[domain.EndpointStatus]int)
	for resultData := range healthCheckResults {
		endpoint := resultData.endpoint
		result := resultData.result

		// Update endpoint state
		endpoint.Status = result.Status
		endpoint.LastChecked = time.Now()
		endpoint.LastLatency = result.Latency

		if err := s.repository.UpdateEndpoint(checkCtx, endpoint); err != nil {
			s.logger.ErrorWithEndpoint("Failed to update endpoint status", endpoint.URL.String(), "error", err)
		}

		statusCounts[result.Status]++
	}

	// Show clean summary instead of individual status lines
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

// waitForHealthyEndpoints waits until at least one endpoint becomes routable
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

// Start starts the discovery service and health checker
func (s *StaticDiscoveryService) Start(ctx context.Context) error {
	s.logger.Info("Starting static discovery service...")

	// Initial refresh of endpoints from config
	if err := s.RefreshEndpoints(ctx); err != nil {
		return fmt.Errorf("failed to refresh endpoints: %w", err)
	}

	// Perform initial health checks before starting periodic checks
	if err := s.performInitialHealthChecks(ctx); err != nil {
		s.logger.Warn("Initial health checks failed, continuing with periodic checks",
			"error", err)
	}

	// Start periodic health checking
	if err := s.checker.StartChecking(ctx); err != nil {
		return fmt.Errorf("failed to start health checking: %w", err)
	}

	// If no endpoints were routable initially, wait for periodic checks
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

// Stop stops the discovery service and health checker
func (s *StaticDiscoveryService) Stop(ctx context.Context) error {
	s.logger.Info("Stopping static discovery service...")

	// Stop health checking
	if err := s.checker.StopChecking(ctx); err != nil {
		return fmt.Errorf("failed to stop health checking: %w", err)
	}

	s.logger.Info("Static discovery service stopped successfully")
	return nil
}

// SetInitialHealthTimeout allows configuring the initial health check timeout
func (s *StaticDiscoveryService) SetInitialHealthTimeout(timeout time.Duration) {
	s.initialHealthTimeout = timeout
}

// EndpointStatusResponse represents the JSON structure for endpoint status
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

// GetHealthStatus returns a summary of endpoint health
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
			URL:                 endpoint.URL.String(),
			Priority:            endpoint.Priority,
			Status:              string(endpoint.Status),
			LastChecked:         endpoint.LastChecked,
			LastLatency:         endpoint.LastLatency.String(),
			ConsecutiveFailures: endpoint.ConsecutiveFailures,
			BackoffMultiplier:   endpoint.BackoffMultiplier,
			NextCheckTime:       endpoint.NextCheckTime,
		}
	}
	status["endpoints"] = endpoints

	return status, nil
}
