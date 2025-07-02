package services

import (
	"context"
	"fmt"
	"net/http"

	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// DiscoveryService orchestrates endpoint discovery, health monitoring, and model
// cataloguing. It runs initial health checks during startup to ensure endpoints
// are validated before the proxy accepts traffic, preventing "no healthy endpoints"
// errors during the critical startup phase.
type DiscoveryService struct {
	config         *config.DiscoveryConfig
	healthChecker  *health.HTTPHealthChecker
	statsCollector ports.StatsCollector
	statsService   *StatsService
	logger         logger.StyledLogger
	modelDiscovery *discovery.ModelDiscoveryService
	registry       domain.ModelRegistry
	endpointRepo   domain.EndpointRepository
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(
	config *config.DiscoveryConfig,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *DiscoveryService {
	return &DiscoveryService{
		config:         config,
		statsCollector: statsCollector,
		logger:         logger,
	}
}

// Name returns the service name
func (s *DiscoveryService) Name() string {
	return "discovery"
}

// Start initialises discovery components
func (s *DiscoveryService) Start(ctx context.Context) error {
	s.logger.Info("Initialising discovery service")

	if s.statsService != nil {
		s.statsCollector = s.statsService.GetCollector()
	}

	s.registry = registry.NewMemoryModelRegistry(s.logger)

	switch s.config.Type {
	case "static":
		s.endpointRepo = discovery.NewStaticEndpointRepository()
		if err := s.endpointRepo.LoadFromConfig(ctx, s.config.Static.Endpoints); err != nil {
			return fmt.Errorf("failed to load endpoints from config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported discovery type: %s", s.config.Type)
	}

	s.healthChecker = health.NewHTTPHealthCheckerWithDefaults(s.endpointRepo, s.logger)

	if err := s.healthChecker.StartChecking(ctx); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	// Critical: Run initial health check to validate endpoints immediately.
	// This prevents the proxy from starting with no healthy endpoints.
	if err := s.healthChecker.RunHealthCheck(ctx, true); err != nil {
		s.logger.Warn("Failed to force initial health check", "error", err)
	}

	endpoints, err := s.endpointRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	for _, endpoint := range endpoints {
		s.logger.Info("Endpoint registered",
			"name", endpoint.Name,
			"url", endpoint.URLString,
			"priority", endpoint.Priority)
	}

	if s.config.ModelDiscovery.Enabled {
		httpClient := &http.Client{
			Timeout: s.config.ModelDiscovery.Timeout,
		}
		profileFactory := profile.NewFactory()
		client := discovery.NewHTTPModelDiscoveryClient(profileFactory, s.logger, httpClient)
		discoveryConfig := discovery.DiscoveryConfig{
			Interval:          s.config.ModelDiscovery.Interval,
			Timeout:           s.config.ModelDiscovery.Timeout,
			ConcurrentWorkers: s.config.ModelDiscovery.ConcurrentWorkers,
			RetryAttempts:     s.config.ModelDiscovery.RetryAttempts,
			RetryBackoff:      s.config.ModelDiscovery.RetryBackoff,
		}
		s.modelDiscovery = discovery.NewModelDiscoveryService(client, s.endpointRepo, s.registry, discoveryConfig, s.logger)

		if err := s.modelDiscovery.Start(ctx); err != nil {
			return fmt.Errorf("failed to start model discovery: %w", err)
		}

		// Run initial model discovery to populate the catalogue immediately
		if err := s.modelDiscovery.DiscoverAll(ctx); err != nil {
			s.logger.Warn("Initial model discovery failed", "error", err)
		}
	}

	s.logger.Info("Discovery service initialised",
		"type", s.config.Type,
		"endpoints", len(endpoints))

	return nil
}

// Stop gracefully shuts down discovery components
func (s *DiscoveryService) Stop(ctx context.Context) error {
	s.logger.Info("Stopping discovery service")

	if s.healthChecker != nil {
		if err := s.healthChecker.StopChecking(ctx); err != nil {
			s.logger.Warn("Failed to stop health checker", "error", err)
		}
	}

	if s.modelDiscovery != nil {
		if err := s.modelDiscovery.Stop(ctx); err != nil {
			s.logger.Warn("Failed to stop model discovery", "error", err)
		}
	}

	s.logger.Info("Discovery service stopped")
	return nil
}

// Dependencies returns service dependencies
func (s *DiscoveryService) Dependencies() []string {
	return []string{"stats"}
}

// GetRegistry returns the model registry
func (s *DiscoveryService) GetRegistry() domain.ModelRegistry {
	if s.registry == nil {
		panic("model registry not initialised")
	}
	return s.registry
}

// GetEndpointRepository returns the endpoint repository
func (s *DiscoveryService) GetEndpointRepository() domain.EndpointRepository {
	if s.endpointRepo == nil {
		panic("endpoint repository not initialised")
	}
	return s.endpointRepo
}

// GetHealthChecker returns the health checker
func (s *DiscoveryService) GetHealthChecker() *health.HTTPHealthChecker {
	if s.healthChecker == nil {
		panic("health checker not initialised")
	}
	return s.healthChecker
}

// GetDiscoveryService returns itself as a ports.DiscoveryService
func (s *DiscoveryService) GetDiscoveryService() ports.DiscoveryService {
	return s
}

// GetHealthyEndpoints implements ports.DiscoveryService
func (s *DiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	if s.endpointRepo == nil {
		return nil, fmt.Errorf("endpoint repository not initialized")
	}
	return s.endpointRepo.GetHealthy(ctx)
}

// GetEndpoints implements ports.DiscoveryService
func (s *DiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	if s.endpointRepo == nil {
		return nil, fmt.Errorf("endpoint repository not initialized")
	}
	return s.endpointRepo.GetAll(ctx)
}

// RefreshEndpoints implements ports.DiscoveryService
func (s *DiscoveryService) RefreshEndpoints(ctx context.Context) error {
	// For static discovery, endpoints don't change, so this is a no-op
	// In a dynamic discovery implementation, this would refresh from the discovery source
	return nil
}

// SetStatsService sets the stats service dependency
func (s *DiscoveryService) SetStatsService(statsService *StatsService) {
	s.statsService = statsService
}
