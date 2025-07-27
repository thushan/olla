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

// unsupportedModelManagementHandler returns 501 for model management operations.
// managing models across distributed instances requires careful orchestration
// to avoid inconsistencies and partial failures
func (a *Application) unsupportedModelManagementHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "model management operations not supported by proxy", http.StatusNotImplemented)
}
