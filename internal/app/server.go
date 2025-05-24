package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/thushan/olla/internal/adapter/discovery"
	"net/http"
)

const (
	ContentTypeJSON   = "application/json"
	ContentTypeText   = "text/plain"
	ContentTypeHeader = "Content-Type"
)

func (a *Application) startWebServer() {
	a.logger.Info("Starting WebServer...", "host", a.config.Server.Host, "port", a.config.Server.Port)

	mux := http.NewServeMux()

	a.registerRoutes()
	a.registry.WireUp(mux)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP server error", "error", err)
			a.errCh <- err
		}
	}()

	a.server.Handler = mux
	a.logger.Info("Started WebServer", "bind", a.server.Addr)
}

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	response := map[string]string{"status": "healthy"}
	_ = json.NewEncoder(w).Encode(response)
}

// statusHandler handles endpoint status requests
func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get discovery service status if it implements the method
	if ds, ok := a.discoveryService.(*discovery.StaticDiscoveryService); ok {
		status, err := ds.GetHealthStatus(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get status: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set(ContentTypeHeader, ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
		return
	}

	// Fallback response
	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"message": "Status endpoint available"}
	_ = json.NewEncoder(w).Encode(response)
}

// proxyHandler handles Ollama API proxy requests
func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusNotImplemented)

	response := map[string]string{
		"message": "Ollama proxy not yet implemented",
		"path":    r.URL.Path,
	}
	_ = json.NewEncoder(w).Encode(response)
}
