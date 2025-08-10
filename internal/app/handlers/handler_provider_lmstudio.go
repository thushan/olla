package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
)

// lmstudioOpenAIModelsHandler provides models in openai format.
// lm studio supports multiple api formats for broader client compatibility
// endpoints: GET /olla/lmstudio/v1/models or /olla/lmstudio/api/v1/models
func (a *Application) lmstudioOpenAIModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "lmstudio")
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

// lmstudioEnhancedModelsHandler returns models using lm studio's beta api format.
// endpoint: GET /olla/lmstudio/api/v0/models
//
// the v0 api provides enhanced model metadata including stats and states
func (a *Application) lmstudioEnhancedModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "lmstudio")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// use lmstudio format converter for enhanced metadata
	response, err := a.convertModelsToProviderFormat(models, "lmstudio")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// lm studio focuses on local model serving without centralised management.
// this simplifies deployment but limits remote administration capabilities
