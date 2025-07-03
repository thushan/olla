package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// SecurityAdapters provides middleware for security chain
type SecurityAdapters struct {
	securityChain *ports.SecurityChain
}

// CreateChainMiddleware creates middleware that applies the full security chain
func (s *SecurityAdapters) CreateChainMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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
			next.ServeHTTP(w, r)
		})
	}
}

// CreateRateLimitMiddleware creates middleware that only applies rate limiting
func (s *SecurityAdapters) CreateRateLimitMiddleware() func(http.Handler) http.Handler {
	// For now, just pass through - rate limiting is part of the security chain
	return func(next http.Handler) http.Handler {
		return next
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
) *Application {
	// Create inspector chain
	profileFactory := profile.NewFactory()
	inspectorFactory := inspector.NewFactory(profileFactory, logger)
	inspectorChain := inspectorFactory.CreateChain()
	// Add path inspector
	pathInspector := inspectorFactory.CreatePathInspector()
	inspectorChain.AddInspector(pathInspector)

	// Create security adapters
	securityAdapters := &SecurityAdapters{
		securityChain: securityChain,
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
		server:           server,
		errCh:            make(chan error, 1),
		StartTime:        time.Now(),
	}
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
