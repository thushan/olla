package converter

import (
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// LMStudioModelResponse represents the LM Studio-compatible format response
type LMStudioModelResponse struct {
	Object string              `json:"object"`
	Data   []LMStudioModelData `json:"data"`
}

// LMStudioModelData represents a single model in LM Studio format
type LMStudioModelData struct {
	MaxContextLength *int64 `json:"max_context_length,omitempty"`
	ID               string `json:"id"`
	Object           string `json:"object"`
	Type             string `json:"type"`
	Publisher        string `json:"publisher,omitempty"`
	Arch             string `json:"arch,omitempty"`
	Quantization     string `json:"quantization,omitempty"`
	State            string `json:"state"`
}

// LMStudioConverter converts models to LM Studio-compatible format
type LMStudioConverter struct {
	*BaseConverter
}

// NewLMStudioConverter creates a new LM Studio format converter
func NewLMStudioConverter() ports.ModelResponseConverter {
	return &LMStudioConverter{
		BaseConverter: NewBaseConverter(constants.ProviderPrefixLMStudio1),
	}
}

func (c *LMStudioConverter) GetFormatName() string {
	return constants.ProviderPrefixLMStudio1
}

func (c *LMStudioConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]LMStudioModelData, 0, len(filtered))
	for _, model := range filtered {
		lmModel := c.convertModel(model)
		if lmModel != nil {
			data = append(data, *lmModel)
		}
	}

	return LMStudioModelResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *LMStudioConverter) convertModel(model *domain.UnifiedModel) *LMStudioModelData {
	helper := c.BaseConverter.NewConversionHelper(model)

	if helper.ShouldSkip() {
		return nil
	}

	// Determine model type
	modelType := helper.GetModelType("llm")

	// Extract publisher from metadata
	publisher := helper.GetMetadataString("publisher")
	if publisher == "" {
		publisher = helper.GetMetadataString("vendor")
	}

	return &LMStudioModelData{
		ID:               helper.Alias,
		Object:           "model",
		Type:             modelType,
		Publisher:        publisher,
		Arch:             model.Family,
		Quantization:     denormalizeQuantization(model.Quantization),
		State:            helper.GetState("not-loaded"),
		MaxContextLength: model.MaxContextLength,
	}
}
