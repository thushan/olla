package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// SecurityAdapters provides middleware for security chain
type SecurityAdapters struct {
	securityChain    *ports.SecurityChain
	securityAdapters *security.Adapters // nil when security is not configured
	logger           logger.StyledLogger
}

// CreateChainMiddleware creates middleware that applies the full security chain with enhanced logging.
// When concrete security adapters are available (production path), we delegate to them so that
// per-validator status codes (429 rate-limit, 413 body-too-large) are preserved. The abstract
// securityChain path is kept as a fallback for test contexts where only the chain is wired.
func (s *SecurityAdapters) CreateChainMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Wrap with logging so every proxy request is recorded regardless of which
		// security path runs below.
		withAccessLogging := middleware.CombinedLoggingMiddleware(s.logger)(next)

		if s.securityAdapters != nil {
			// Delegate to the concrete adapter chain. It sets the correct status codes
			// (429 + Retry-After/X-RateLimit-* for rate limiting, 413 for oversized
			// bodies) rather than flattening everything to 403.
			concreteChain := s.securityAdapters.CreateChainMiddleware()(withAccessLogging)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				concreteChain.ServeHTTP(w, r)
			})
		}

		// Fallback: abstract chain only (e.g. unit tests that inject securityChain
		// directly without wiring the full security.Adapters).
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.securityChain != nil {
				secReq := ports.SecurityRequest{
					ClientID:      r.RemoteAddr,
					Endpoint:      r.URL.Path,
					Method:        r.Method,
					BodySize:      r.ContentLength,
					HeaderSize:    0,
					Headers:       r.Header,
					IsHealthCheck: r.URL.Path == "/internal/health",
				}

				result, err := s.securityChain.Validate(r.Context(), secReq)
				if err != nil || !result.Allowed {
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
		return middleware.CombinedLoggingMiddleware(s.logger)(next)
	}
}

// CreateSizeMiddleware returns a middleware that enforces request-size limits on
// non-proxy routes. Delegates to the real security adapter when available; returns
// a pass-through when security is not configured (e.g. in tests).
func (s *SecurityAdapters) CreateSizeMiddleware() func(http.Handler) http.Handler {
	if s.securityAdapters != nil {
		return s.securityAdapters.CreateSizeMiddleware()
	}
	return func(next http.Handler) http.Handler { return next }
}

// Application holds all the dependencies needed for the HTTP handlers
type Application struct {
	Config             *config.Config
	logger             logger.StyledLogger
	proxyService       ports.ProxyService
	statsCollector     ports.StatsCollector
	modelRegistry      domain.ModelRegistry
	discoveryService   ports.DiscoveryService
	repository         domain.EndpointRepository
	inspectorChain     *inspector.Chain
	securityAdapters   *SecurityAdapters
	routeRegistry      *router.RouteRegistry
	converterFactory   *converter.ConverterFactory
	profileFactory     profile.ProfileFactory
	profileLookup      translator.ProfileLookup
	translatorRegistry *translator.Registry
	// stickyStatsFn is non-nil when sticky sessions are enabled. Stored as a
	// closure so the handler layer does not need to import the balancer package.
	stickyStatsFn func() *balancer.StickyStats
	aliasResolver *registry.AliasResolver
	server        *http.Server
	errCh         chan error
	StartTime     time.Time
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

	// translator registry avoids bloating app struct
	translatorRegistry := translator.NewRegistry(logger)

	// Register Anthropic translator if enabled
	if cfg.Translators.Anthropic.Enabled {
		// Validate configuration before registering
		if err := cfg.Translators.Anthropic.Validate(); err != nil {
			logger.Error("Invalid Anthropic translator configuration", "error", err)
			return nil, fmt.Errorf("invalid Anthropic translator config: %w", err)
		}

		anthropicTranslator, err := anthropic.NewTranslator(logger, cfg.Translators.Anthropic)
		if err != nil {
			return nil, fmt.Errorf("failed to create Anthropic translator: %w", err)
		}
		translatorRegistry.Register("anthropic", anthropicTranslator)

		logger.Info("Registered Anthropic translator",
			"max_message_size", cfg.Translators.Anthropic.MaxMessageSize)
	} else {
		logger.Info("Anthropic translator disabled via configuration")
	}

	// Create alias resolver for model name aliasing (nil if no aliases configured)
	aliasResolver := registry.NewAliasResolver(cfg.ModelAliases, logger)
	if aliasResolver != nil {
		logger.Info("Model aliases configured", "alias_count", len(cfg.ModelAliases))
	}

	// Use profile factory directly as it implements the ProfileLookup interface
	// The Factory.GetAnthropicSupport method provides the required functionality
	profileLookup := profileFactory

	return &Application{
		Config:             cfg,
		logger:             logger,
		proxyService:       proxyService,
		statsCollector:     statsCollector,
		modelRegistry:      modelRegistry,
		discoveryService:   discoveryService,
		repository:         repository,
		inspectorChain:     inspectorChain,
		securityAdapters:   securityAdapters,
		routeRegistry:      routeRegistry,
		profileFactory:     profileFactory,
		profileLookup:      profileLookup,
		converterFactory:   converter.NewConverterFactory(),
		translatorRegistry: translatorRegistry,
		aliasResolver:      aliasResolver,
		server:             server,
		errCh:              make(chan error, 1),
		StartTime:          time.Now(),
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

// get translator registry for handlers/routes
func (a *Application) GetTranslatorRegistry() *translator.Registry {
	return a.translatorRegistry
}

// GetProfileLookup returns the profile lookup adapter for accessing backend profiles
func (a *Application) GetProfileLookup() translator.ProfileLookup {
	return a.profileLookup
}

// SetStickyStatsFn wires in the sticky session stats provider after construction.
// Called by HTTPService when sticky sessions are enabled.
func (a *Application) SetStickyStatsFn(fn func() *balancer.StickyStats) {
	a.stickyStatsFn = fn
}

// SetSecurityAdapters wires the real security.Adapters into the handlers-layer
// SecurityAdapters so non-proxy routes gain size validation. Called by HTTPService
// after the SecurityService has finished initialising.
func (a *Application) SetSecurityAdapters(adapters *security.Adapters) {
	if a.securityAdapters != nil {
		a.securityAdapters.securityAdapters = adapters
	}
}

func (a *Application) RegisterRoutes() {
	a.registerRoutes()
}
