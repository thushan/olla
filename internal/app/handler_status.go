package app

import (
	"encoding/json"
	"fmt"
	"github.com/thushan/olla/internal/adapter/discovery"
	"net/http"
)

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
