package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/constants"
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

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ollamaModelShowHandler proxies /api/show to the backend that hosts the requested model.
// endpoint: POST /olla/ollama/api/show
//
// Reconciling model details across instances is not feasible: modelfiles, templates, and
// tensor metadata differ per-node with no canonical merge strategy. Returning one node's
// real answer is correct — LangFlow and similar clients only need a single coherent response,
// not an aggregate. providerProxyHandler handles endpoint selection and body rewriting.
func (a *Application) ollamaModelShowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read the body to extract the model name, then restore it so the downstream
	// proxy sees an intact body. The same read-restore idiom is used in body_inspector.go
	// and injectStickyKey — each stage restores independently.
	var bodyBytes []byte
	if r.Body != nil {
		var readErr error
		bodyBytes, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
	}

	var req struct {
		Model string `json:"model"`
	}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			http.Error(w, "invalid request body: expected JSON with 'model' field", http.StatusBadRequest)
			return
		}
	}

	if req.Model == "" {
		http.Error(w, "model name is required", http.StatusBadRequest)
		return
	}

	// Verify Olla knows about this model before forwarding. If the registry is the wrong
	// type or unavailable we fall through to the proxy anyway — better to let the backend
	// return a 404 than refuse known-good requests due to a registry type mismatch.
	if unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry); ok {
		if _, err := unifiedRegistry.GetUnifiedModel(ctx, req.Model); err != nil {
			http.Error(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
			return
		}
	}

	// Delegate to the provider proxy. r.URL.Path is still the full /olla/ollama/api/show
	// at this point — providerProxyHandler extracts the provider from that path, so it
	// must remain unmodified.
	a.providerProxyHandler(w, r)
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

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// unsupportedModelManagementHandler returns 501 for model management operations.
// managing models across distributed instances requires careful orchestration
// to avoid inconsistencies and partial failures
func (a *Application) unsupportedModelManagementHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "model management operations not supported by proxy", http.StatusNotImplemented)
}
