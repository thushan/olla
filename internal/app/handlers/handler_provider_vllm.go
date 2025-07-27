package handlers

import (
	"encoding/json"
	"net/http"
)

// vllmModelsHandler returns models from vllm endpoints
// endpoint: GET /olla/vllm/v1/models
func (a *Application) vllmModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "vllm")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// vllm implements openai-compatible api
	response, err := a.convertModelsToProviderFormat(models, "openai")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// vllm optimises for high-throughput inference with techniques like
// continuous batching and paged attention. api compatibility with
// openai reduces integration complexity for existing applications
