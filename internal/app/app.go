package app

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
	"net/http"
	"sync"
)

// Application represents the Olla application
type Application struct {
	configMu         sync.RWMutex
	config           *config.Config
	server           *http.Server
	logger           *logger.StyledLogger
	registry         *router.RouteRegistry
	discoveryService ports.DiscoveryService
	proxyService     ports.ProxyService
	pluginService    ports.PluginService
	errCh            chan error
}

// New creates a new application instance
func New(logger *logger.StyledLogger) (*Application, error) {

	/**
	 * Slightly annoying but we have to setup the configuration with defaults here
	 * then load it again with viper to allow hot reloading.
	 **/
	registry := router.NewRouteRegistry(logger)
	repository := discovery.NewStaticEndpointRepository()
	healthChecker := health.NewHTTPHealthCheckerWithDefaults(repository, logger)
	discoveryService := discovery.NewStaticDiscoveryService(repository, healthChecker, nil, logger)

	balancerFactory := balancer.NewFactory()
	selector, err := balancerFactory.Create(DefaultLoadBalancer)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	proxyService := proxy.NewService(
		discoveryService,
		selector,
		DefaultProxyConfiguration(),
		logger,
	)

	app := &Application{
		logger:           logger,
		registry:         registry,
		discoveryService: discoveryService,
		proxyService:     proxyService,
		errCh:            make(chan error, 1),
	}

	cfg, err := config.Load(func() {
		// Hot reloading of configuration file
		// this is a bit tricky, inspired by Viper's docs.
		if err := viper.ReadInConfig(); err != nil {
			logger.Error("Failed to re-read config file", "error", err)
			return
		}

		newConfig := config.DefaultConfig()
		if err := viper.Unmarshal(newConfig); err != nil {
			logger.Error("Failed to unmarshal new config", "error", err)
			return
		}

		app.setConfig(newConfig)

		discoveryService.SetConfig(newConfig)
		discoveryService.ReloadConfig()

		proxyService.UpdateConfig(updateProxyConfiguration(newConfig))

	})
	if err != nil {
		return nil, fmt.Errorf("Failed to load configuration: %v\n", err)
	}

	// Now we can set the configuration properly
	app.setConfig(cfg)
	discoveryService.SetConfig(cfg)
	proxyService.UpdateConfig(updateProxyConfiguration(cfg))

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Handler:      nil, // Will be set in Start()
	}
	app.server = server

	return app, nil
}

// Start starts the application
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

// Stop stops the application
func (a *Application) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, a.config.Server.ShutdownTimeout)
	defer cancel()

	// Stop discovery service first
	if err := a.discoveryService.Stop(shutdownCtx); err != nil {
		a.logger.Error("Failed to stop discovery service", "error", err)
	}

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	return nil
}
