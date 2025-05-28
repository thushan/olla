package app

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
	"net/http"
)

type Application struct {
	config           *config.Config
	server           *http.Server
	logger           *logger.StyledLogger
	registry         *router.RouteRegistry
	discoveryService ports.DiscoveryService
	proxyService     ports.ProxyService
	pluginService    ports.PluginService
	errCh            chan error
}

func New(logger *logger.StyledLogger) (*Application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	registry := router.NewRouteRegistry(logger)
	repository := discovery.NewStaticEndpointRepository()
	healthChecker := health.NewHTTPHealthCheckerWithDefaults(repository, logger)
	discoveryService := discovery.NewStaticDiscoveryService(repository, healthChecker, cfg.Discovery.Static.Endpoints, logger)

	balancerFactory := balancer.NewFactory()
	selector, err := balancerFactory.Create(DefaultLoadBalancer)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
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
		config:           cfg,
		server:           server,
		logger:           logger,
		registry:         registry,
		discoveryService: discoveryService,
		proxyService:     proxyService,
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

	if err := a.discoveryService.Start(ctx); err != nil {
		a.logger.Error("discovery service startup error", "error", err)
		a.errCh <- err
	}

	a.logger.Info("Olla started", "bind", a.server.Addr)
	return nil
}

func (a *Application) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, a.config.Server.ShutdownTimeout)
	defer cancel()

	if err := a.discoveryService.Stop(shutdownCtx); err != nil {
		a.logger.Error("Failed to stop discovery service", "error", err)
	}

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	return nil
}