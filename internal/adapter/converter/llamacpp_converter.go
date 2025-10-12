package converter

import (
	"time"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Type aliases for backward compatibility with tests
type LlamaCppResponse = profile.LlamaCppResponse
type LlamaCppModel = profile.LlamaCppModel

// LlamaCppConverter converts models to llama.cpp OpenAI-compatible format
type LlamaCppConverter struct {
	*BaseConverter
}

// NewLlamaCppConverter creates a new llama.cpp format converter
func NewLlamaCppConverter() ports.ModelResponseConverter {
	return &LlamaCppConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeLlamaCpp),
	}
}

func (c *LlamaCppConverter) GetFormatName() string {
	return constants.ProviderTypeLlamaCpp
}

func (c *LlamaCppConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.LlamaCppModel, 0, len(filtered))
	for _, model := range filtered {
		llamaModel := c.convertModel(model)
		if llamaModel != nil {
			data = append(data, *llamaModel)
		}
	}

	return profile.LlamaCppResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *LlamaCppConverter) convertModel(model *domain.UnifiedModel) *profile.LlamaCppModel {
	// llama.cpp uses simple OpenAI-compatible format with minimal fields
	now := time.Now().Unix()
	modelID := c.findLlamaCppNativeName(model)
	if modelID == "" {
		// Fallback to first alias or unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	llamaModel := &profile.LlamaCppModel{
		ID:      modelID,
		Object:  "model",
		Created: now,
		OwnedBy: c.determineOwner(modelID),
	}

	return llamaModel
}

// findLlamaCppNativeName looks for the native llama.cpp name from aliases
func (c *LlamaCppConverter) findLlamaCppNativeName(model *domain.UnifiedModel) string {
	// Use base converter to find llamacpp-specific alias
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the organisation from the model ID or defaults to "llamacpp"
func (c *LlamaCppConverter) determineOwner(modelID string) string {
	// Use shared owner extraction logic from BaseConverter
	// Handles slash-separated (org/model) and hyphenated (org-model) formats
	return ExtractOwnerFromModelID(modelID, constants.ProviderTypeLlamaCpp)
}
