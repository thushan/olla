package app

import (
	"encoding/json"
	"net/http"
)

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	response := map[string]string{"status": "healthy"}
	_ = json.NewEncoder(w).Encode(response)
}
