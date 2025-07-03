package services

import (
	"context"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/app/handlers"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// HTTPService manages the HTTP server lifecycle and route registration. It coordinates
// with other services to ensure the server only starts accepting requests after all
// dependencies are initialised and health checks have completed.
type HTTPService struct {
	config           *config.ServerConfig
	fullConfig       *config.Config
	server           *http.Server
	proxyService     ports.ProxyService
	statsCollector   ports.StatsCollector
	modelRegistry    domain.ModelRegistry
	securityChain    *ports.SecurityChain
	logger           logger.StyledLogger
	application      *handlers.Application
	discoveryService ports.DiscoveryService
	repository       domain.EndpointRepository
	statsService     *StatsService
	proxySvc         *ProxyServiceWrapper
	discoverySvc     *DiscoveryService
	securitySvc      *SecurityService
}

// NewHTTPService creates a new HTTP service
func NewHTTPService(
	config *config.ServerConfig,
	fullConfig *config.Config,
	logger logger.StyledLogger,
) *HTTPService {
	return &HTTPService{
		config:     config,
		fullConfig: fullConfig,
		logger:     logger,
	}
}

// Name returns the service name
func (s *HTTPService) Name() string {
	return "http"
}

// Start initialises and starts the HTTP server
func (s *HTTPService) Start(ctx context.Context) error {
	s.logger.Info("Initialising HTTP service")

	// Resolve service dependencies now that all services are started
	if s.statsService != nil {
		s.statsCollector = s.statsService.GetCollector()
	}
	if s.proxySvc != nil {
		s.proxyService = s.proxySvc.GetProxyService()
	}
	if s.discoverySvc != nil {
		s.modelRegistry = s.discoverySvc.GetRegistry()
		s.discoveryService = s.discoverySvc.GetDiscoveryService()
		s.repository = s.discoverySvc.GetEndpointRepository()
	}
	if s.securitySvc != nil {
		s.securityChain = s.securitySvc.GetSecurityChain()
	}

	readTimeout := s.config.ReadTimeout
	writeTimeout := s.config.WriteTimeout
	idleTimeout := s.config.IdleTimeout

	s.application = handlers.NewApplication(
		ctx,
		s.fullConfig,
		s.proxyService,
		s.statsCollector,
		s.modelRegistry,
		s.discoveryService,
		s.repository,
		s.securityChain,
		s.logger,
	)

	s.application.RegisterRoutes()

	// Wire routes with security middleware
	mux := http.NewServeMux()
	routeRegistry := s.application.GetRouteRegistry()
	securityAdapters := s.application.GetSecurityAdapters()
	routeRegistry.WireUpWithSecurityChain(mux, securityAdapters)

	s.server = &http.Server{
		Addr:         s.config.GetAddress(),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	go func() {
		s.logger.Info("HTTP server listening",
			"address", s.server.Addr,
			"readTimeout", readTimeout,
			"writeTimeout", writeTimeout,
			"idleTimeout", idleTimeout)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Brief pause ensures the listener is established before returning
	time.Sleep(100 * time.Millisecond)

	// Signal readiness - critical for operations teams monitoring startup
	s.logger.Info("Olla started, waiting for requests...", "bind", s.server.Addr)

	return nil
}

// Stop gracefully shuts down the HTTP server
func (s *HTTPService) Stop(ctx context.Context) error {
	s.logger.Info(" Stopping HTTP server...")
	defer func() {
		s.logger.ResetLine()
		s.logger.InfoWithStatus("Stopping HTTP server", "OK")
	}()

	if s.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTP server shutdown error", "error", err)
			return err
		}
	}
	return nil
}

// Dependencies returns service dependencies
func (s *HTTPService) Dependencies() []string {
	return []string{"proxy", "security"}
}

// SetStatsService sets the stats service dependency
func (s *HTTPService) SetStatsService(statsService *StatsService) {
	s.statsService = statsService
}

// SetProxyService sets the proxy service dependency
func (s *HTTPService) SetProxyService(proxyService *ProxyServiceWrapper) {
	s.proxySvc = proxyService
}

// SetDiscoveryService sets the discovery service dependency
func (s *HTTPService) SetDiscoveryService(discoveryService *DiscoveryService) {
	s.discoverySvc = discoveryService
}

// SetSecurityService sets the security service dependency
func (s *HTTPService) SetSecurityService(securityService *SecurityService) {
	s.securitySvc = securityService
}

// SetDependencies sets all dependencies at once
func (s *HTTPService) SetDependencies(stats *StatsService, proxy *ProxyServiceWrapper, discovery *DiscoveryService, security *SecurityService) {
	s.statsService = stats
	s.proxySvc = proxy
	s.discoverySvc = discovery
	s.securitySvc = security
}
