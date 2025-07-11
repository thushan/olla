package converter

import (
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// LMStudioModelResponse represents the LM Studio-compatible format response
type LMStudioModelResponse struct {
	Object string             `json:"object"`
	Data   []LMStudioModelData `json:"data"`
}

// LMStudioModelData represents a single model in LM Studio format
type LMStudioModelData struct {
	ID               string  `json:"id"`
	Object           string  `json:"object"`
	Type             string  `json:"type"`
	Publisher        string  `json:"publisher,omitempty"`
	Arch             string  `json:"arch,omitempty"`
	Quantization     string  `json:"quantization,omitempty"`
	State            string  `json:"state"`
	MaxContextLength *int64  `json:"max_context_length,omitempty"`
}

// LMStudioConverter converts models to LM Studio-compatible format
type LMStudioConverter struct{}

// NewLMStudioConverter creates a new LM Studio format converter
func NewLMStudioConverter() ports.ModelResponseConverter {
	return &LMStudioConverter{}
}

func (c *LMStudioConverter) GetFormatName() string {
	return "lmstudio"
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
	// Find the LM Studio-specific alias or endpoint
	var lmstudioName string
	var lmstudioEndpoint *domain.SourceEndpoint
	
	// First, look for an LM Studio source in aliases
	for _, alias := range model.Aliases {
		if alias.Source == "lmstudio" {
			lmstudioName = alias.Name
			break
		}
	}
	
	// Find corresponding LM Studio endpoint
	for i := range model.SourceEndpoints {
		ep := &model.SourceEndpoints[i]
		if lmstudioName != "" && ep.NativeName == lmstudioName {
			lmstudioEndpoint = ep
			break
		}
	}
	
	// If no LM Studio-specific data, skip this model
	if lmstudioName == "" {
		return nil
	}
	
	// Determine model type
	modelType := "llm"
	if t, ok := model.Metadata["type"].(string); ok {
		modelType = t
	} else if hasCapability(model.Capabilities, "vision") {
		modelType = "vlm"
	} else if hasCapability(model.Capabilities, "embedding") || hasCapability(model.Capabilities, "embeddings") {
		modelType = "embeddings"
	}
	
	// Extract publisher from metadata or model name
	publisher := ""
	if p, ok := model.Metadata["publisher"].(string); ok {
		publisher = p
	} else if v, ok := model.Metadata["vendor"].(string); ok {
		publisher = v
	}
	
	// Determine state from endpoint
	state := "not-loaded"
	if lmstudioEndpoint != nil {
		state = lmstudioEndpoint.State
	}
	
	return &LMStudioModelData{
		ID:               lmstudioName,
		Object:           "model",
		Type:             modelType,
		Publisher:        publisher,
		Arch:             model.Family,
		Quantization:     denormalizeQuantization(model.Quantization),
		State:            state,
		MaxContextLength: model.MaxContextLength,
	}
}