package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// OpenAIModelResponse represents the OpenAI-compatible format response
type OpenAIModelResponse struct {
	Object string           `json:"object"`
	Data   []OpenAIModelData `json:"data"`
}

// OpenAIModelData represents a single model in OpenAI format
type OpenAIModelData struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIConverter converts models to OpenAI-compatible format
type OpenAIConverter struct{}

// NewOpenAIConverter creates a new OpenAI format converter
func NewOpenAIConverter() ports.ModelResponseConverter {
	return &OpenAIConverter{}
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
	return OpenAIModelData{
		ID:      model.ID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "olla",
	}
}