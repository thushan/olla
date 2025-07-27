package handlers

import (
	"encoding/json"
	"net/http"
)

// ollamaModelsHandler returns models from all healthy ollama instances
// endpoint: GET /olla/ollama/api/tags
func (a *Application) ollamaModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "ollama")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := a.convertModelsToProviderFormat(models, "ollama")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ollamaModelShowHandler handles model detail requests.
// endpoint: POST /olla/ollama/api/show
//
// aggregating model details across instances presents challenges:
// - modelfiles may differ between instances
// - version conflicts need resolution
// - parameter reconciliation is non-trivial
func (a *Application) ollamaModelShowHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "model show not supported in multi-instance proxy", http.StatusNotImplemented)
}

// ollamaRunningModelsHandler returns currently loaded/running models.
// endpoint: GET /olla/ollama/api/list
//
// this would need to aggregate running models across all ollama instances
// which adds complexity around state synchronisation
func (a *Application) ollamaRunningModelsHandler(w http.ResponseWriter, r *http.Request) {
	// running model tracking across instances is complex
	// would need to poll each instance and merge results
	http.Error(w, "running models list not supported in multi-instance proxy", http.StatusNotImplemented)
}

// ollamaOpenAIModelsHandler returns models in openai-compatible format.
// ollama experimental compatibility layer for openai clients
// endpoint: GET /olla/ollama/v1/models
func (a *Application) ollamaOpenAIModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := a.getProviderModels(ctx, "ollama")
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

// unsupportedModelManagementHandler returns 501 for model management operations.
// managing models across distributed instances requires careful orchestration
// to avoid inconsistencies and partial failures
func (a *Application) unsupportedModelManagementHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "model management operations not supported by proxy", http.StatusNotImplemented)
}
