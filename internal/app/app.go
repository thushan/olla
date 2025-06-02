package app

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/security"
	"net/http"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/adapter/proxy"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

type Application struct {
	StartTime        time.Time
	Config           *config.Config
	server           *http.Server
	logger           *logger.StyledLogger
	registry         *router.RouteRegistry
	modelRegistry    *domain.ModelRegistry
	repository       *discovery.StaticEndpointRepository
	healthChecker    *health.HTTPHealthChecker
	proxyService     ports.ProxyService
	securityServices *security.Services
	securityAdapters *security.Adapters
	errCh            chan error
	shutdownOnce     sync.Once
}

func New(startTime time.Time, logger *logger.StyledLogger) (*Application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	routeRegistry := router.NewRouteRegistry(logger)
	repository := discovery.NewStaticEndpointRepository()
	healthChecker := health.NewHTTPHealthCheckerWithDefaults(repository, logger)

	modelRegistryConfig := registry.RegistryConfig{Type: cfg.ModelRegistry.Type}
	modelRegistry, err := registry.NewModelRegistry(modelRegistryConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create model registry: %w", err)
	}

	balancerFactory := balancer.NewFactory()
	selector, err := balancerFactory.Create(DefaultLoadBalancer)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	discoveryService := &simpleDiscovery{
		repository: repository,
		endpoints:  cfg.Discovery.Static.Endpoints,
		logger:     logger,
	}

	proxyService := proxy.NewService(
		discoveryService,
		selector,
		updateProxyConfiguration(cfg),
		logger,
	)

	securityServices, securityAdapters := security.NewSecurityServices(cfg, logger)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Handler:      nil,
	}

	app := &Application{
		StartTime:        startTime,
		Config:           cfg,
		server:           server,
		logger:           logger,
		registry:         routeRegistry,
		modelRegistry:    &modelRegistry,
		repository:       repository,
		healthChecker:    healthChecker,
		proxyService:     proxyService,
		securityServices: securityServices,
		securityAdapters: securityAdapters,
		errCh:            make(chan error, 1),
	}

	return app, nil
}

func (a *Application) Start(ctx context.Context) error {
	go func() {
		select {
		case err := <-a.errCh:
			a.logger.Error("Server startup error", "error", err)
		case <-ctx.Done():
			return
		}
	}()

	a.startWebServer()

	if err := a.repository.LoadFromConfig(ctx, a.Config.Discovery.Static.Endpoints); err != nil {
		a.logger.Error("Failed to load endpoints from config", "error", err)
		a.errCh <- err
		return err
	}

	if err := a.healthChecker.StartChecking(ctx); err != nil {
		a.logger.Error("Failed to start health checker", "error", err)
		a.errCh <- err
		return err
	}

	if err := a.healthChecker.RunHealthCheck(ctx, true); err != nil {
		a.logger.Warn("Failed to force initial health check", "error", err)
	}

	a.logger.Info("Olla started, waiting for requests...", "bind", a.server.Addr)
	return nil
}

func (a *Application) Stop(ctx context.Context) error {
	var shutdownErr error

	a.shutdownOnce.Do(func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, a.Config.Server.ShutdownTimeout)
		defer cancel()

		if a.securityAdapters != nil {
			a.securityAdapters.Stop()
		}

		if err := a.healthChecker.StopChecking(shutdownCtx); err != nil {
			a.logger.Error("Failed to stop health checker", "error", err)
			shutdownErr = err
		}

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			if shutdownErr != nil {
				a.logger.Error("HTTP server shutdown error", "error", err)
			} else {
				shutdownErr = fmt.Errorf("HTTP server shutdown error: %w", err)
			}
		}
	})

	return shutdownErr
}

type simpleDiscovery struct {
	repository *discovery.StaticEndpointRepository
	logger     *logger.StyledLogger
	endpoints  []config.EndpointConfig
}

func (s *simpleDiscovery) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repository.GetAll(ctx)
}

func (s *simpleDiscovery) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	routable, err := s.repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	if len(routable) > 0 {
		return routable, nil
	}

	s.logger.Warn("No routable endpoints available, falling back to all endpoints")
	return s.repository.GetAll(ctx)
}

func (s *simpleDiscovery) RefreshEndpoints(ctx context.Context) error {
	return s.repository.LoadFromConfig(ctx, s.endpoints)
}
