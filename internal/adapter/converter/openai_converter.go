package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// OpenAIModelResponse represents the OpenAI-compatible format response
type OpenAIModelResponse struct {
	Object string            `json:"object"`
	Data   []OpenAIModelData `json:"data"`
}

// OpenAIModelData represents a single model in OpenAI format
type OpenAIModelData struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Created int64  `json:"created"`
}

// OpenAIConverter converts models to OpenAI-compatible format
type OpenAIConverter struct {
	*BaseConverter
}

// NewOpenAIConverter creates a new OpenAI format converter
func NewOpenAIConverter() ports.ModelResponseConverter {
	return &OpenAIConverter{
		BaseConverter: NewBaseConverter("openai"),
	}
}

func (c *OpenAIConverter) GetFormatName() string {
	return "openai"
}

func (c *OpenAIConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]OpenAIModelData, 0, len(filtered))
	for _, model := range filtered {
		data = append(data, c.convertModel(model))
	}

	return OpenAIModelResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *OpenAIConverter) convertModel(model *domain.UnifiedModel) OpenAIModelData {
	// OLLA-85: [Unification] Models with different digests fail to unify correctly.
	// we need to use first alas as ID for routing compatibility
	// to make sure the returned model ID can be used for requests
	modelID := model.ID
	if len(model.Aliases) > 0 {
		modelID = model.Aliases[0].Name
	}

	return OpenAIModelData{
		ID:      modelID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "olla",
	}
}
