package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
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
	StartTime     time.Time
	Config        *config.Config
	server        *http.Server
	logger        *logger.StyledLogger
	registry      *router.RouteRegistry
	repository    *discovery.StaticEndpointRepository
	healthChecker *health.HTTPHealthChecker
	proxyService  ports.ProxyService
	errCh         chan error
}

func New(startTime time.Time, logger *logger.StyledLogger) (*Application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	registry := router.NewRouteRegistry(logger)
	repository := discovery.NewStaticEndpointRepository()
	healthChecker := health.NewHTTPHealthCheckerWithDefaults(repository, logger)

	balancerFactory := balancer.NewFactory()
	selector, err := balancerFactory.Create(DefaultLoadBalancer)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	// Create simple discovery wrapper that implements the interface
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

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Handler:      nil,
	}

	app := &Application{
		StartTime:     startTime,
		Config:        cfg,
		server:        server,
		logger:        logger,
		registry:      registry,
		repository:    repository,
		healthChecker: healthChecker,
		proxyService:  proxyService,
		errCh:         make(chan error, 1),
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
	shutdownCtx, cancel := context.WithTimeout(ctx, a.Config.Server.ShutdownTimeout)
	defer cancel()

	if err := a.healthChecker.StopChecking(shutdownCtx); err != nil {
		a.logger.Error("Failed to stop health checker", "error", err)
	}

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	return nil
}

// simpleDiscovery implements ports.DiscoveryService without the wrapper complexity
type simpleDiscovery struct {
	repository *discovery.StaticEndpointRepository
	endpoints  []config.EndpointConfig
	logger     *logger.StyledLogger
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
