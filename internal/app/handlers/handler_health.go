package handlers

import (
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
)

var responseJSON = []byte(`{"status":"healthy"}`)

// healthHandler handles health check requests
func (a *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseJSON)
}
