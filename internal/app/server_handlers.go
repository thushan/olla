package app

import (
	"encoding/json"
	"fmt"
	"github.com/thushan/olla/internal/adapter/discovery"
	"net/http"
)

func (a *Application) registerRoutes() {
	a.registry.RegisterWithMethod("/proxy", a.proxyHandler, "Ollama API proxy endpoint (default)", "POST")
	a.registry.RegisterWithMethod("/ma", a.proxyHandler, "Ollama API proxy endpoint (mirror)", "POST")
	a.registry.RegisterWithMethod("/internal/health", a.healthHandler, "Health check endpoint", "GET")
	a.registry.RegisterWithMethod("/internal/status", a.statusHandler, "Endpoint status", "GET")
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
