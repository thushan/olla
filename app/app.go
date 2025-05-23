package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/thushan/olla/internal/config"
	"log"
	"log/slog"
	"net/http"
)

// Application represents the Olla application
type Application struct {
	config *config.Config
	server *http.Server
	logger *slog.Logger
}

// New creates a new application instance
func New(cfg *config.Config, logger *slog.Logger) (*Application, error) {

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Handler:      nil, // Will be set in Start()
	}

	return &Application{
		config: cfg,
		server: server,
		logger: logger,
	}, nil
}

// Start starts the application
func (a *Application) Start(ctx context.Context) error {

	a.logger.Info("Starting WebServer...", "host", a.config.Server.Host, "port", a.config.Server.Port)

	router := http.NewServeMux()
	router.HandleFunc("/health", a.healthHandler)

	a.server.Handler = router

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v\n", err)
		}
	}()

	a.logger.Info("Started WebServer", "bind", a.server.Addr)
	a.logger.Info("Endpoints enabled", slog.Group("/health",
		"info", "Health check endpoint"), slog.Group("/ma",
		"info", "Default Ollama endpoint"))
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

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"healthy"}`)
}
