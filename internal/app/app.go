package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/router"
	"log/slog"
	"net/http"
)

// Application represents the Olla application
type Application struct {
	config   *config.Config
	server   *http.Server
	logger   *slog.Logger
	registry *router.RouteRegistry
	errCh    chan error
}

// New creates a new application instance
func New(cfg *config.Config, logger *slog.Logger) (*Application, error) {

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Handler:      nil, // Will be set in Start()
	}

	registry := router.NewRouteRegistry(logger)

	return &Application{
		config:   cfg,
		server:   server,
		logger:   logger,
		registry: registry,
		errCh:    make(chan error, 1),
	}, nil
}

// Start starts the application
func (a *Application) Start(ctx context.Context) error {

	a.logger.Info("Starting WebServer...", "host", a.config.Server.Host, "port", a.config.Server.Port)

	mux := http.NewServeMux()

	a.registerRoutes()
	a.registry.WireUp(mux)

	a.server.Handler = mux

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP server error", "error", err)
			a.errCh <- err
		}
	}()

	go func() {
		select {
		case err := <-a.errCh:
			a.logger.Error("Server startup error", "error", err)
		case <-ctx.Done():
			return
		}
	}()

	a.logger.Info("Started WebServer", "bind", a.server.Addr)
	return nil
}

// Stop stops the application
func (a *Application) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, a.config.Server.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	return nil
}

func (a *Application) registerRoutes() {
	a.registry.RegisterWithMethod("/proxy", a.proxyHandler, "Ollama API proxy endpoint (default)", "POST")
	a.registry.RegisterWithMethod("/ma", a.proxyHandler, "Ollama API proxy endpoint (mirror)", "POST")
	a.registry.RegisterWithMethod("/internal/health", a.healthHandler, "Health check endpoint", "GET")
}

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]string{"status": "healthy"}
	json.NewEncoder(w).Encode(response)
}

// proxyHandler handles Ollama API proxy requests
func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)

	response := map[string]string{
		"message": "Ollama proxy not yet implemented",
		"path":    r.URL.Path,
	}
	json.NewEncoder(w).Encode(response)
}
