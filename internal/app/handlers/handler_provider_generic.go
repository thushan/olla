package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
)

// genericProviderModelsHandler returns a handler function for any provider's model listing
func (a *Application) genericProviderModelsHandler(providerType, format string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Normalise provider type
		normalizedProvider := NormaliseProviderType(providerType)

		// Get models from provider
		models, err := a.getProviderModels(ctx, normalizedProvider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Convert to requested format using the existing converter factory
		response, err := a.convertModelsToProviderFormat(models, format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// genericModelShowHandler returns model details for any provider
func (a *Application) genericModelShowHandler(providerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			http.Error(w, "Model name is required", http.StatusBadRequest)
			return
		}

		normalizedProvider := NormaliseProviderType(providerType)

		// For now, return a simple response
		// In the future, this could proxy to the actual provider endpoint
		response := map[string]interface{}{
			"name":     req.Name,
			"provider": normalizedProvider,
			"details":  "Model details would be retrieved from provider",
		}

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		json.NewEncoder(w).Encode(response)
	}
}
