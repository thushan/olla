package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/thushan/olla/internal/app/handlers"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	// defaultReadHeaderTimeout protects the inbound server against Slowloris-style
	// attacks. A legitimate client sends its full header set well within 10 s; slow
	// backends are handled by proxy.ConnectionTimeout which applies much later.
	defaultReadHeaderTimeout = 10 * time.Second
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
		collector, err := s.statsService.GetCollector()
		if err != nil {
			return fmt.Errorf("failed to get stats collector: %w", err)
		}
		s.statsCollector = collector
	}
	if s.proxySvc != nil {
		proxyService, err := s.proxySvc.GetProxyService()
		if err != nil {
			return fmt.Errorf("failed to get proxy service: %w", err)
		}
		s.proxyService = proxyService
	}
	if s.discoverySvc != nil {
		registry, err := s.discoverySvc.GetRegistry()
		if err != nil {
			return fmt.Errorf("failed to get model registry: %w", err)
		}
		s.modelRegistry = registry

		discoveryService, err := s.discoverySvc.GetDiscoveryService()
		if err != nil {
			return fmt.Errorf("failed to get discovery service: %w", err)
		}
		s.discoveryService = discoveryService

		repository, err := s.discoverySvc.GetEndpointRepository()
		if err != nil {
			return fmt.Errorf("failed to get endpoint repository: %w", err)
		}
		s.repository = repository
	}
	if s.securitySvc != nil {
		chain, err := s.securitySvc.GetSecurityChain()
		if err != nil {
			return fmt.Errorf("failed to get security chain: %w", err)
		}
		s.securityChain = chain
	}

	readTimeout := s.config.ReadTimeout
	readHeaderTimeout := s.config.ReadHeaderTimeout
	if readHeaderTimeout == 0 {
		readHeaderTimeout = defaultReadHeaderTimeout
	}
	writeTimeout := s.config.WriteTimeout
	idleTimeout := s.config.IdleTimeout

	app, err := handlers.NewApplication(
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
	if err != nil {
		return fmt.Errorf("failed to create application handler: %w", err)
	}
	s.application = app

	// Wire sticky session stats if enabled - proxySvc holds the wrapper.
	if s.proxySvc != nil {
		s.application.SetStickyStatsFn(s.proxySvc.StickyStats)
	}

	// Wire real security adapters so non-proxy routes get size validation.
	// This is separate from the security chain (which handles proxy routes) so
	// non-proxy routes are protected without requiring full chain enforcement.
	if s.securitySvc != nil {
		if adapters, adapterErr := s.securitySvc.GetAdapters(); adapterErr == nil && adapters != nil {
			s.application.SetSecurityAdapters(adapters)
		}
	}

	s.application.RegisterRoutes()

	// Wire routes with security middleware
	mux := http.NewServeMux()
	routeRegistry := s.application.GetRouteRegistry()
	securityAdapters := s.application.GetSecurityAdapters()
	routeRegistry.WireUpWithSecurityChain(mux, securityAdapters)

	var root http.Handler = mux
	root = applyCORS(root, s.fullConfig.Server.Cors)

	if s.fullConfig.Server.Cors.Enabled {
		s.logger.Info("CORS enabled",
			"allowed_origins", s.fullConfig.Server.Cors.AllowedOrigins,
			"allow_credentials", s.fullConfig.Server.Cors.AllowCredentials)
	}

	addr := s.config.GetAddress()

	ln, err := bindListener(ctx, addr)
	if err != nil {
		return err
	}

	s.server = &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	s.logger.Info("HTTP server listening",
		"address", addr,
		"readTimeout", readTimeout,
		"readHeaderTimeout", readHeaderTimeout,
		"writeTimeout", writeTimeout,
		"idleTimeout", idleTimeout)

	go func() {
		if serr := s.server.Serve(ln); serr != nil && !errors.Is(serr, http.ErrServerClosed) {
			s.logger.Error("HTTP server error", "error", serr)
		}
	}()

	s.logger.Info("Olla started, waiting for requests...", "bind", addr)

	s.printWarnings()
	s.printDashboardURL(addr)
	return nil
}

// printDashboardURL logs a standout line pointing at the admin dashboard once
// the server is otherwise fully up. It runs after printWarnings so it is the
// last thing an operator sees at the bottom of the startup log, mirroring the
// InfoWithStatus "[ OK ]" treatment Stop() already uses for its loudest line.
// Skipped silently if the bind address can't be turned into a URL - this is
// cosmetic, not worth failing startup over.
func (s *HTTPService) printDashboardURL(bindAddr string) {
	if !s.fullConfig.Dashboard.Enabled {
		return
	}

	url, ok := dashboardURL(bindAddr)
	if !ok {
		return
	}

	s.logger.Info(fmt.Sprintf("Admin dashboard ready - %s", url))
}

// dashboardURL derives a clickable dashboard URL from a listener bind address.
// An unspecified host (0.0.0.0, ::, [::], or an empty host) isn't something a
// browser can open, so it's substituted with localhost. IPv6 literals are
// bracketed for a valid URL authority.
func dashboardURL(bindAddr string) (string, bool) {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil || port == "" {
		return "", false
	}

	// net.JoinHostPort brackets IPv6 literals itself, so displayHost is left
	// unbracketed here regardless of address family.
	displayHost := host
	if addr, perr := netip.ParseAddr(host); host == "" || (perr == nil && addr.IsUnspecified()) {
		displayHost = "localhost"
	}

	return fmt.Sprintf("http://%s/internal/ui/", net.JoinHostPort(displayHost, port)), true
}

// applyCORS wraps the root handler with CORS as the outermost layer so that
// browser preflight (OPTIONS) requests are answered directly by rs/cors and
// never reach the per-route security chain (rate-limiting, access logging).
// This is correct CORS behaviour: preflights carry no credentials or body and
// never reach a backend, so letting the security chain return 401/403/429 to
// a preflight probe would break legitimate browser clients. Actual GET/POST
// requests pass through rs/cors and then traverse the full security chain as
// normal. Kept as a seam so the enable/disable wiring is testable without
// standing up the full HTTP server.
func applyCORS(handler http.Handler, cfg config.CorsConfig) http.Handler {
	if !cfg.Enabled {
		return handler
	}
	return middleware.NewCORS(cfg).Handler(handler)
}

// bindListener binds a TCP listener on addr synchronously so that any
// port-in-use error surfaces immediately to the caller. Using ListenConfig
// means the listener also respects context cancellation during startup.
func bindListener(ctx context.Context, addr string) (net.Listener, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind %s: %w", addr, err)
	}
	return ln, nil
}

func (s *HTTPService) printWarnings() {
	if s.fullConfig.Translators.Anthropic.Inspector.Enabled {
		s.logger.Warn("Anthropic Inspector is ENABLED. DO NOT USE IN PROD.", "more_info", "https://thushan.github.io/olla/notes/anthropic-inspector/")
	}
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
