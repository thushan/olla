package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
)

var response = map[string]string{"status": "healthy"}

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(response)
}
