package handlers

import (
	"encoding/json"
	"net/http"
)

// lmstudioModelsHandler returns models in ollama-compatible format.
// lm studio adopted ollama's api structure for compatibility
// endpoint: GET /olla/lmstudio/api/v1/tags
func (a *Application) lmstudioModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "lmstudio")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// lm studio uses ollama format for this endpoint
	response, err := a.convertModelsToProviderFormat(models, "ollama")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

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

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// lm studio focuses on local model serving without centralised management.
// this simplifies deployment but limits remote administration capabilities
