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

// list models in translator format (eg /olla/anthropic/v1/models)
func (a *Application) translatorModelsHandler(trans translator.RequestTranslator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// fetch available models from registry
		var unifiedModels []*domain.UnifiedModel
		var err error

		// check if registry supports getting unified models
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

		// skip unhealthy models for endpoint response
		healthyModels, err := a.filterModelsByHealth(ctx, unifiedModels)
		if err != nil {
			a.writeTranslatorModelsError(w, trans, "failed to filter models by health", http.StatusInternalServerError)
			return
		}

		// Anthropic format matches the Python reference: {data: [{id, name, created, description, type}]}
		response := a.convertModelsToAnthropicFormat(healthyModels)

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// convert to anthropic format (matches python reference)
func (a *Application) convertModelsToAnthropicFormat(models []*domain.UnifiedModel) map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(models))

	for _, model := range models {
		// use first alias or fallback to default id
		modelID := model.ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		}

		// map model fields to human-readable fields
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

// send error response for models endpoint
func (a *Application) writeTranslatorModelsError(w http.ResponseWriter, trans translator.RequestTranslator, message string, statusCode int) {
	a.logger.Error("Translator models request failed",
		"translator", trans.Name(),
		"error", message,
		"status", statusCode)

	// use custom error format or fallback to generic
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, err{message: message}, statusCode)
		return
	}

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

// minimal error type for response handling
type err struct {
	message string
}

func (e err) Error() string {
	return e.message
}
