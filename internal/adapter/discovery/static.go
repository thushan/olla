package discovery

import (
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/thushan/olla/theme"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// StaticEndpointRepository implements domain.EndpointRepository for static endpoints
type StaticEndpointRepository struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex
}

func NewStaticEndpointRepository() *StaticEndpointRepository {
	return &StaticEndpointRepository{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

// GetAll returns all registered endpoints
func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endpoints := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

// GetHealthy returns only healthy endpoints
func (r *StaticEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endpoints := make([]*domain.Endpoint, 0)
	for _, endpoint := range r.endpoints {
		if endpoint.Status == domain.StatusHealthy {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints, nil
}

// UpdateStatus updates the health status of an endpoint
func (r *StaticEndpointRepository) UpdateStatus(ctx context.Context, endpointURL *url.URL, status domain.EndpointStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpointURL.String()
	endpoint, exists := r.endpoints[key]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	endpoint.Status = status
	endpoint.LastChecked = time.Now()
	return nil
}

// Add adds a new endpoint to the repository
func (r *StaticEndpointRepository) Add(ctx context.Context, endpoint *domain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	r.endpoints[key] = endpoint
	return nil
}

// Remove removes an endpoint from the repository
func (r *StaticEndpointRepository) Remove(ctx context.Context, endpointURL *url.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpointURL.String()
	if _, exists := r.endpoints[key]; !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	delete(r.endpoints, key)
	return nil
}

// StaticDiscoveryService implements ports.DiscoveryService for static endpoints
type StaticDiscoveryService struct {
	repository           domain.EndpointRepository
	checker              domain.HealthChecker
	config               *config.Config
	initialHealthTimeout time.Duration
	logger               *slog.Logger
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
	logger *slog.Logger,
) *StaticDiscoveryService {
	return &StaticDiscoveryService{
		repository:           repository,
		checker:              checker,
		config:               config,
		logger:               logger,
		initialHealthTimeout: DefaultInitialHealthTimeout,
	}
}

func validateEndpointConfig(cfg config.EndpointConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	if cfg.HealthCheckURL == "" {
		return fmt.Errorf("health check URL cannot be empty")
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

	// Validate URLs are parseable
	if _, err := url.Parse(cfg.URL); err != nil {
		return fmt.Errorf("invalid endpoint URL %q: %w", cfg.URL, err)
	}

	if _, err := url.Parse(cfg.HealthCheckURL); err != nil {
		return fmt.Errorf("invalid health check URL %q: %w", cfg.HealthCheckURL, err)
	}

	return nil
}

// GetEndpoints returns all registered endpoints
func (s *StaticDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetAll(ctx)
}

// GetHealthyEndpoints returns only healthy endpoints
func (s *StaticDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetHealthy(ctx)
}

// GetHealthyEndpointsWithFallback returns healthy endpoints with graceful degradation
func (s *StaticDiscoveryService) GetHealthyEndpointsWithFallback(ctx context.Context) ([]*domain.Endpoint, error) {
	healthy, err := s.repository.GetHealthy(ctx)
	if err != nil {
		return nil, err
	}

	if len(healthy) == 0 {
		s.logger.Warn("No healthy endpoints available, falling back to all endpoints")
		all, err := s.repository.GetAll(ctx)
		if err != nil {
			return nil, err
		}

		// Still return error if no endpoints exist at all
		if len(all) == 0 {
			return nil, fmt.Errorf("no endpoints configured")
		}

		return all, nil
	}

	return healthy, nil
}

// RefreshEndpoints triggers a refresh of the endpoint list from the config
func (s *StaticDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	currentEndpoints, err := s.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current endpoints: %w", err)
	}

	// Create a map of current endpoints for quick lookup
	currentMap := make(map[string]*domain.Endpoint)
	for _, endpoint := range currentEndpoints {
		currentMap[endpoint.URL.String()] = endpoint
	}

	for _, endpointCfg := range s.config.Discovery.Static.Endpoints {
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

		healthCheckURL, err := url.Parse(endpointCfg.HealthCheckURL)
		if err != nil {
			return fmt.Errorf("invalid health check URL %s: %w", endpointCfg.HealthCheckURL, err)
		}

		key := endpointURL.String()
		if existing, exists := currentMap[key]; exists {
			configChanged := s.hasEndpointConfigChanged(existing, endpointCfg, healthCheckURL)

			// Update existing endpoint
			existing.Name = endpointCfg.Name
			existing.Priority = endpointCfg.Priority
			existing.HealthCheckURL = healthCheckURL
			existing.CheckInterval = endpointCfg.CheckInterval
			existing.CheckTimeout = endpointCfg.CheckTimeout

			if configChanged {
				existing.Status = domain.StatusUnknown
				existing.LastChecked = time.Now()
				s.logger.Info("Endpoint configuration changed, resetting health status",
					"endpoint", existing.URL.String())
			}

			delete(currentMap, key)
		} else {
			endpoint := &domain.Endpoint{
				Name:           endpointCfg.Name,
				URL:            endpointURL,
				Priority:       endpointCfg.Priority,
				HealthCheckURL: healthCheckURL,
				CheckInterval:  endpointCfg.CheckInterval,
				CheckTimeout:   endpointCfg.CheckTimeout,
				Status:         domain.StatusUnknown,
			}

			if err := s.repository.Add(ctx, endpoint); err != nil {
				return fmt.Errorf("failed to add endpoint %s: %w", key, err)
			}

			s.logger.Info("Added new endpoint", "endpoint", endpoint.URL.String())
		}
	}

	// Remove endpoints that are no longer in config
	for key, endpoint := range currentMap {
		if err := s.repository.Remove(ctx, endpoint.URL); err != nil {
			return fmt.Errorf("failed to remove endpoint %s: %w", key, err)
		}
		s.logger.Info("Removed endpoint", "endpoint", endpoint.URL.String())
	}

	return nil
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

	s.logger.Info(fmt.Sprintf("Health checking %s Endpoints",
		pterm.Style{theme.Default().Counts}.Sprintf("(%d)", endpointCount)))

	// Perform health checks concurrently but wait for all to complete
	var wg sync.WaitGroup
	healthCheckResults := make(chan struct {
		endpoint *domain.Endpoint
		status   domain.EndpointStatus
		err      error
	}, len(endpoints))

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep *domain.Endpoint) {
			defer wg.Done()

			s.logger.Info(fmt.Sprintf("Initial health check for %s",
				pterm.Style{theme.Default().HealthCheck}.Sprintf(ep.URL.String())))

			status, err := s.checker.Check(checkCtx, ep)

			healthCheckResults <- struct {
				endpoint *domain.Endpoint
				status   domain.EndpointStatus
				err      error
			}{ep, status, err}
		}(endpoint)
	}

	wg.Wait()
	close(healthCheckResults)

	healthyCount := 0
	unhealthyCount := 0
	unknownCount := 0

	// Process results
	for result := range healthCheckResults {
		if result.err != nil {
			s.logger.Error("Initial health check failed",
				"endpoint", result.endpoint.URL.String(),
				"error", result.err)
		}

		if err := s.repository.UpdateStatus(checkCtx, result.endpoint.URL, result.status); err != nil {
			s.logger.Error("Failed to update endpoint status",
				"endpoint", result.endpoint.URL.String(),
				"error", err)
		}

		switch result.status {
		case domain.StatusHealthy:
			healthyCount++
			s.logger.Info("Endpoint is healthy",
				"endpoint", result.endpoint.URL.String(),
				"status", "healthy")
		case domain.StatusUnhealthy:
			unhealthyCount++
			s.logger.Info("Endpoint is unhealthy",
				"endpoint", result.endpoint.URL.String(),
				"status", "unhealthy")
		default:
			unknownCount++
			s.logger.Warn("Endpoint status unknown",
				"endpoint", result.endpoint.URL.String(),
				"status", "unknown")
		}
	}

	if healthyCount == 0 {
		return fmt.Errorf("no healthy endpoints available after initial health check")
	}

	s.logger.Info("Initial health check complete",
		"healthy", healthyCount,
		"unhealthy", unhealthyCount,
		"unknown", unknownCount)

	return nil
}

// waitForHealthyEndpoints waits until at least one endpoint becomes healthy
func (s *StaticDiscoveryService) waitForHealthyEndpoints(ctx context.Context, maxWait time.Duration) error {
	s.logger.Info("Waiting for healthy endpoints", "max_wait", maxWait)

	timeout := time.NewTimer(maxWait)
	defer timeout.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for healthy endpoints: %w", ctx.Err())
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for healthy endpoints after %v", maxWait)
		case <-ticker.C:
			healthy, err := s.repository.GetHealthy(ctx)
			if err != nil {
				s.logger.Error("Error checking healthy endpoints", "error", err)
				continue
			}

			if len(healthy) > 0 {
				s.logger.Info("Found healthy endpoints, ready to serve traffic",
					"count", len(healthy))
				return nil
			}

			s.logger.Warn("No healthy endpoints yet, waiting...")
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

	// If no endpoints were healthy initially, wait for periodic checks
	healthy, err := s.repository.GetHealthy(ctx)
	if err != nil {
		return fmt.Errorf("failed to check healthy endpoints: %w", err)
	}

	if len(healthy) == 0 {
		s.logger.Info("No initially healthy endpoints, waiting for periodic health checks...")

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

	status := make(map[string]interface{})
	status["total_endpoints"] = len(all)
	status["healthy_endpoints"] = len(healthy)
	status["unhealthy_endpoints"] = len(all) - len(healthy)

	endpoints := make([]map[string]interface{}, len(all))
	for i, endpoint := range all {
		endpoints[i] = map[string]interface{}{
			"name":         endpoint.Name,
			"url":          endpoint.URL.String(),
			"priority":     endpoint.Priority,
			"status":       string(endpoint.Status),
			"last_checked": endpoint.LastChecked,
		}
	}
	status["endpoints"] = endpoints

	return status, nil
}
