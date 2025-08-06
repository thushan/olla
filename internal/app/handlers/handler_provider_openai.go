package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
)

// openaiModelsHandler returns models from endpoints that implement openai's api.
// these are typically local inference servers (localai, text-generation-webui, etc)
// that chose openai's api format for compatibility
// endpoint: GET /olla/openai/v1/models
func (a *Application) openaiModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "openai")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := a.convertModelsToProviderFormat(models, "openai")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// many local inference servers implement openai's api for drop-in compatibility.
// this standardisation simplifies client integration across different backends
