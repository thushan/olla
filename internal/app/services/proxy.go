package services

import (
	"context"
	"fmt"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/metrics"
	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// ProxyServiceWrapper adapts the core proxy implementation to the service lifecycle
// model. It manages the creation of load balancers and proxy engines, ensuring they
// receive validated endpoints from the discovery service.
type ProxyServiceWrapper struct {
	config           *config.ProxyConfig
	proxyService     ports.ProxyService
	loadBalancer     domain.EndpointSelector
	endpointRepo     domain.EndpointRepository
	discoveryService ports.DiscoveryService
	statsCollector   ports.StatsCollector
	metricsExtractor ports.MetricsExtractor
	statsService     *StatsService
	discoverySvc     *DiscoveryService
	securityService  *SecurityService
	logger           logger.StyledLogger
}

// NewProxyServiceWrapper creates a new proxy service wrapper
func NewProxyServiceWrapper(
	config *config.ProxyConfig,
	logger logger.StyledLogger,
) *ProxyServiceWrapper {
	return &ProxyServiceWrapper{
		config: config,
		logger: logger,
	}
}

// Name returns the service name
func (s *ProxyServiceWrapper) Name() string {
	return "proxy"
}

// Start initialises the proxy service and load balancer
func (s *ProxyServiceWrapper) Start(ctx context.Context) error {
	s.logger.Info("Initialising proxy service")

	// Get dependencies from services
	if s.statsService != nil {
		collector, err := s.statsService.GetCollector()
		if err != nil {
			return fmt.Errorf("failed to get stats collector: %w", err)
		}
		s.statsCollector = collector
	}
	if s.discoverySvc != nil {
		repo, err := s.discoverySvc.GetEndpointRepository()
		if err != nil {
			return fmt.Errorf("failed to get endpoint repository: %w", err)
		}
		s.endpointRepo = repo

		discService, err := s.discoverySvc.GetDiscoveryService()
		if err != nil {
			return fmt.Errorf("failed to get discovery service: %w", err)
		}
		s.discoveryService = discService
	}

	// Create load balancer
	balancerFactory := balancer.NewFactory(s.statsCollector)
	var err error
	s.loadBalancer, err = balancerFactory.Create(s.config.LoadBalancer)
	if err != nil {
		return fmt.Errorf("failed to create load balancer: %w", err)
	}
	s.logger.Info("Load balancer created", "type", s.config.LoadBalancer)

	// Create proxy configuration
	proxyConfig := s.createProxyConfiguration()

	// Create a discovery service adapter
	s.discoveryService = &endpointRepositoryAdapter{repo: s.endpointRepo}

	// Create metrics extractor with profile factory
	s.logger.Info("Creating metrics extractor...")
	profileFactory, err := profile.NewFactoryWithDefaults()
	if err != nil {
		s.logger.Warn("Failed to create profile factory for metrics extraction", "error", err)
		// Continue without metrics extraction
		profileFactory = nil
	} else {
		s.logger.Info("Profile factory created successfully", 
			"profiles", profileFactory.GetAvailableProfiles())
	}

	var metricsExtractor ports.MetricsExtractor
	if profileFactory != nil {
		metricsExtractor, err = metrics.NewExtractor(profileFactory, s.logger)
		if err != nil {
			s.logger.Warn("Failed to create metrics extractor", "error", err)
			// Continue without metrics extraction
			metricsExtractor = nil
		} else {
			s.logger.Info("Metrics extractor initialized successfully")
			// Pre-validate profiles for metrics extraction
			for _, profileName := range profileFactory.GetAvailableProfiles() {
				if inferenceProfile, err := profileFactory.GetProfile(profileName); err == nil {
					if extractor, ok := metricsExtractor.(*metrics.Extractor); ok {
						if err := extractor.ValidateProfile(inferenceProfile); err != nil {
							s.logger.Debug("Profile metrics validation failed", 
								"profile", profileName, "error", err)
						}
					}
				}
			}
		}
	}
	s.metricsExtractor = metricsExtractor

	// Create proxy service
	proxyFactory := proxy.NewFactory(s.statsCollector, metricsExtractor, s.logger)
	s.proxyService, err = proxyFactory.Create(s.config.Engine, s.discoveryService, s.loadBalancer, proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to create proxy service: %w", err)
	}

	s.logger.Info("Proxy service initialised",
		"engine", s.config.Engine,
		"profile", s.config.Profile,
		"balancer", s.config.LoadBalancer)

	return nil
}

// Stop gracefully shuts down the proxy service
func (s *ProxyServiceWrapper) Stop(ctx context.Context) error {
	s.logger.Info(" Stopping proxy service")

	// Most proxy implementations don't need explicit cleanup
	// but we provide the hook for future extensions

	defer func() {
		s.logger.ResetLine()
		s.logger.InfoWithStatus("Stopping proxy service", "OK")
	}()
	return nil
}

// Dependencies returns service dependencies
func (s *ProxyServiceWrapper) Dependencies() []string {
	return []string{"discovery", "security", "stats"}
}

// createProxyConfiguration builds engine-specific configuration. The proxy factory
// will augment this with engine-specific settings (e.g., connection pool parameters
// for Olla engine).
func (s *ProxyServiceWrapper) createProxyConfiguration() *proxy.Configuration {
	return &proxy.Configuration{
		ProxyPrefix:         "",
		ConnectionTimeout:   s.config.ConnectionTimeout,
		ConnectionKeepAlive: 30 * time.Second,
		ResponseTimeout:     s.config.ResponseTimeout,
		ReadTimeout:         s.config.ReadTimeout,
		StreamBufferSize:    s.config.StreamBufferSize,
	}
}

// GetProxyService returns the underlying proxy service
func (s *ProxyServiceWrapper) GetProxyService() (ports.ProxyService, error) {
	if s.proxyService == nil {
		return nil, fmt.Errorf("proxy service not initialised")
	}
	return s.proxyService, nil
}

// GetLoadBalancer returns the load balancer
func (s *ProxyServiceWrapper) GetLoadBalancer() (domain.EndpointSelector, error) {
	if s.loadBalancer == nil {
		return nil, fmt.Errorf("load balancer not initialised")
	}
	return s.loadBalancer, nil
}

// endpointRepositoryAdapter provides interface adaptation between the domain repository
// and the discovery service interface expected by the proxy layer.
type endpointRepositoryAdapter struct {
	repo domain.EndpointRepository
}

func (a *endpointRepositoryAdapter) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return a.repo.GetAll(ctx)
}

func (a *endpointRepositoryAdapter) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return a.repo.GetHealthy(ctx)
}

func (a *endpointRepositoryAdapter) RefreshEndpoints(ctx context.Context) error {
	// No-op for static endpoints
	return nil
}

func (a *endpointRepositoryAdapter) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	// Update endpoint status in repository
	return a.repo.UpdateEndpoint(ctx, endpoint)
}

// SetStatsService sets the stats service dependency
func (s *ProxyServiceWrapper) SetStatsService(statsService *StatsService) {
	s.statsService = statsService
}

// SetDiscoveryService sets the discovery service dependency
func (s *ProxyServiceWrapper) SetDiscoveryService(discoveryService *DiscoveryService) {
	s.discoverySvc = discoveryService
}

// SetSecurityService sets the security service dependency
func (s *ProxyServiceWrapper) SetSecurityService(securityService *SecurityService) {
	s.securityService = securityService
}
