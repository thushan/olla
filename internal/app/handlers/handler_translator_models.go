package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// translatorModelsHandler returns a handler for listing models in translator-specific format
// This enables translators to expose available models compatible with their API format
// (e.g., Anthropic models endpoint at /olla/anthropic/v1/models)
func (a *Application) translatorModelsHandler(trans translator.RequestTranslator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get unified models from registry - try type assertion or interface check
		var unifiedModels []*domain.UnifiedModel
		var err error

		// Check if registry supports GetUnifiedModels method
		type unifiedModelsGetter interface {
			GetUnifiedModels(ctx context.Context) ([]*domain.UnifiedModel, error)
		}

		if getter, ok := a.modelRegistry.(unifiedModelsGetter); ok {
			unifiedModels, err = getter.GetUnifiedModels(ctx)
			if err != nil {
				a.writeTranslatorModelsError(w, trans, "failed to get unified models", http.StatusInternalServerError)
				return
			}
		} else {
			a.writeTranslatorModelsError(w, trans, "unified models not supported", http.StatusInternalServerError)
			return
		}

		// Filter to only show healthy models
		healthyModels, err := a.filterModelsByHealth(ctx, unifiedModels)
		if err != nil {
			a.writeTranslatorModelsError(w, trans, "failed to filter models by health", http.StatusInternalServerError)
			return
		}

		// Convert to translator-specific format
		// Anthropic format matches the Python reference: {data: [{id, name, created, description, type}]}
		response := a.convertModelsToAnthropicFormat(healthyModels)

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// convertModelsToAnthropicFormat converts unified models to Anthropic API format
// Matches the Python reference implementation format from research/anthropic-proxy.py
func (a *Application) convertModelsToAnthropicFormat(models []*domain.UnifiedModel) map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(models))

	for _, model := range models {
		// Use the first alias as the model ID, or fall back to the unified ID
		modelID := model.ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		}

		// Build model entry in Anthropic format
		entry := map[string]interface{}{
			"id":          modelID,
			"name":        modelID,
			"created":     time.Now().Unix(),
			"description": "Chat model via Olla proxy",
			"type":        "chat",
		}

		data = append(data, entry)
	}

	return map[string]interface{}{
		"data": data,
	}
}

// writeTranslatorModelsError writes error response for models endpoint
func (a *Application) writeTranslatorModelsError(w http.ResponseWriter, trans translator.RequestTranslator, message string, statusCode int) {
	a.logger.Error("Translator models request failed",
		"translator", trans.Name(),
		"error", message,
		"status", statusCode)

	// Use translator's custom error formatting if available
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, err{message: message}, statusCode)
		return
	}

	// Fallback to generic JSON error
	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "models_error",
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}

// err is a simple error implementation for error responses
type err struct {
	message string
}

func (e err) Error() string {
	return e.message
}
