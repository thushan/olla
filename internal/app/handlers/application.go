package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// SecurityAdapters provides middleware for security chain
type SecurityAdapters struct {
	securityChain *ports.SecurityChain
	logger        logger.StyledLogger
}

// CreateChainMiddleware creates middleware that applies the full security chain with enhanced logging
func (s *SecurityAdapters) CreateChainMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Chain the middleware: logging -> access logging -> security -> handler
		withLogging := middleware.EnhancedLoggingMiddleware(s.logger)(next)
		withAccessLogging := middleware.AccessLoggingMiddleware(s.logger)(withLogging)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.securityChain != nil {
				// Create security request from HTTP request
				secReq := ports.SecurityRequest{
					ClientID:      r.RemoteAddr, // This would normally be extracted better
					Endpoint:      r.URL.Path,
					Method:        r.Method,
					BodySize:      r.ContentLength,
					HeaderSize:    0, // Would need to calculate
					Headers:       r.Header,
					IsHealthCheck: r.URL.Path == "/internal/health",
				}

				result, err := s.securityChain.Validate(r.Context(), secReq)
				if err != nil || !result.Allowed {
					// Write appropriate error response
					http.Error(w, "Security validation failed", http.StatusForbidden)
					return
				}
			}
			withAccessLogging.ServeHTTP(w, r)
		})
	}
}

// CreateRateLimitMiddleware creates middleware that only applies rate limiting with enhanced logging
func (s *SecurityAdapters) CreateRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Apply enhanced logging for non-proxy routes as well
		withLogging := middleware.EnhancedLoggingMiddleware(s.logger)(next)
		withAccessLogging := middleware.AccessLoggingMiddleware(s.logger)(withLogging)
		return withAccessLogging
	}
}

// Application holds all the dependencies needed for the HTTP handlers
type Application struct {
	Config           *config.Config
	logger           logger.StyledLogger
	proxyService     ports.ProxyService
	statsCollector   ports.StatsCollector
	modelRegistry    domain.ModelRegistry
	discoveryService ports.DiscoveryService
	repository       domain.EndpointRepository
	inspectorChain   *inspector.Chain
	securityAdapters *SecurityAdapters
	routeRegistry    *router.RouteRegistry
	converterFactory *converter.ConverterFactory
	profileFactory   profile.ProfileFactory
	server           *http.Server
	errCh            chan error
	StartTime        time.Time
}

// NewApplication creates a new Application instance with all required dependencies
func NewApplication(
	ctx context.Context,
	cfg *config.Config,
	proxyService ports.ProxyService,
	statsCollector ports.StatsCollector,
	modelRegistry domain.ModelRegistry,
	discoveryService ports.DiscoveryService,
	repository domain.EndpointRepository,
	securityChain *ports.SecurityChain,
	logger logger.StyledLogger,
) (*Application, error) {
	// Create inspector chain
	profileFactory, err := profile.NewFactoryWithDefaults()
	if err != nil {
		// Try to create factory with empty profile dir (uses built-in profiles)
		profileFactory, err = profile.NewFactory("")
		if err != nil {
			logger.Error("Failed to create profile factory", "error", err)
			return nil, fmt.Errorf("cannot initialize profile factory: %w", err)
		}
		logger.Warn("Failed to load profile configurations, using built-in profiles", "error", err)
	}
	inspectorFactory := inspector.NewFactory(profileFactory, logger)
	inspectorChain := inspectorFactory.CreateChain()
	// Add path inspector
	pathInspector := inspectorFactory.CreatePathInspector()
	inspectorChain.AddInspector(pathInspector)
	// Add body inspector for model extraction
	bodyInspector, err := inspectorFactory.CreateBodyInspector()
	if err != nil {
		return nil, fmt.Errorf("failed to create body inspector: %w", err)
	}
	inspectorChain.AddInspector(bodyInspector)

	// Create security adapters
	securityAdapters := &SecurityAdapters{
		securityChain: securityChain,
		logger:        logger,
	}

	// Create route registry
	routeRegistry := router.NewRouteRegistry(logger)

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.Server.GetAddress(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return &Application{
		Config:           cfg,
		logger:           logger,
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		modelRegistry:    modelRegistry,
		discoveryService: discoveryService,
		repository:       repository,
		inspectorChain:   inspectorChain,
		securityAdapters: securityAdapters,
		routeRegistry:    routeRegistry,
		profileFactory:   profileFactory,
		converterFactory: converter.NewConverterFactory(),
		server:           server,
		errCh:            make(chan error, 1),
		StartTime:        time.Now(),
	}, nil
}

// GetRouteRegistry returns the route registry for wiring up routes
func (a *Application) GetRouteRegistry() *router.RouteRegistry {
	return a.routeRegistry
}

// GetSecurityAdapters returns the security adapters for middleware
func (a *Application) GetSecurityAdapters() *SecurityAdapters {
	return a.securityAdapters
}

// GetServer returns the HTTP server instance
func (a *Application) GetServer() *http.Server {
	return a.server
}

func (a *Application) RegisterRoutes() {
	a.registerRoutes()
}
