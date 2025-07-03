package services

import (
	"context"
	"fmt"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/proxy"
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
		s.statsCollector = s.statsService.GetCollector()
	}
	if s.discoverySvc != nil {
		s.endpointRepo = s.discoverySvc.GetEndpointRepository()
		s.discoveryService = s.discoverySvc.GetDiscoveryService()
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

	// Create proxy service
	proxyFactory := proxy.NewFactory(s.statsCollector, s.logger)
	s.proxyService, err = proxyFactory.Create(s.config.Engine, s.discoveryService, s.loadBalancer, proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to create proxy service: %w", err)
	}

	s.logger.Info("Proxy service initialised",
		"engine", s.config.Engine,
		"loadBalancer", s.config.LoadBalancer)

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
func (s *ProxyServiceWrapper) GetProxyService() ports.ProxyService {
	if s.proxyService == nil {
		panic("proxy service not initialised")
	}
	return s.proxyService
}

// GetLoadBalancer returns the load balancer
func (s *ProxyServiceWrapper) GetLoadBalancer() domain.EndpointSelector {
	if s.loadBalancer == nil {
		panic("load balancer not initialised")
	}
	return s.loadBalancer
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
