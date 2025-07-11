package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// UnifiedModelResponse represents the unified Olla format response
type UnifiedModelResponse struct {
	Object string               `json:"object"`
	Data   []UnifiedModelData   `json:"data"`
}

// UnifiedModelData represents a single model in the unified format
type UnifiedModelData struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`
	Created   int64     `json:"created"`
	OwnedBy   string    `json:"owned_by"`
	Olla      *OllaExtensions `json:"olla,omitempty"`
}

// OllaExtensions contains Olla-specific model information
type OllaExtensions struct {
	Family           string           `json:"family"`
	Variant          string           `json:"variant"`
	ParameterSize    string           `json:"parameter_size"`
	Quantization     string           `json:"quantization"`
	Aliases          []string         `json:"aliases"`
	Availability     []EndpointStatus `json:"availability"`
	Capabilities     []string         `json:"capabilities"`
	MaxContextLength *int64           `json:"max_context_length,omitempty"`
	PromptTemplateID string           `json:"prompt_template_id,omitempty"`
}

// EndpointStatus represents model availability on an endpoint
type EndpointStatus struct {
	Endpoint string `json:"endpoint"`
	URL      string `json:"url"`
	State    string `json:"state"`
}

// UnifiedConverter converts models to the default Olla unified format
type UnifiedConverter struct{}

// NewUnifiedConverter creates a new unified format converter
func NewUnifiedConverter() ports.ModelResponseConverter {
	return &UnifiedConverter{}
}

func (c *UnifiedConverter) GetFormatName() string {
	return "unified"
}

func (c *UnifiedConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)
	
	data := make([]UnifiedModelData, 0, len(filtered))
	for _, model := range filtered {
		data = append(data, c.convertModel(model))
	}

	return UnifiedModelResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *UnifiedConverter) convertModel(model *domain.UnifiedModel) UnifiedModelData {
	availability := make([]EndpointStatus, 0, len(model.SourceEndpoints))
	for _, ep := range model.SourceEndpoints {
		availability = append(availability, EndpointStatus{
			Endpoint: ep.EndpointURL, // This will be replaced with endpoint name in handler
			URL:      ep.EndpointURL,
			State:    ep.State,
		})
	}

	return UnifiedModelData{
		ID:      model.ID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "olla",
		Olla: &OllaExtensions{
			Family:           model.Family,
			Variant:          model.Variant,
			ParameterSize:    model.ParameterSize,
			Quantization:     model.Quantization,
			Aliases:          model.GetAliasStrings(),
			Availability:     availability,
			Capabilities:     model.Capabilities,
			MaxContextLength: model.MaxContextLength,
			PromptTemplateID: model.PromptTemplateID,
		},
	}
}

// filterModels applies the specified filters to the models
func filterModels(models []*domain.UnifiedModel, filters ports.ModelFilters) []*domain.UnifiedModel {
	if filters.Endpoint == "" && filters.Available == nil && filters.Family == "" && filters.Type == "" {
		return models
	}

	filtered := make([]*domain.UnifiedModel, 0, len(models))
	for _, model := range models {
		if !matchesFilters(model, filters) {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered
}

// matchesFilters checks if a model matches all specified filters
func matchesFilters(model *domain.UnifiedModel, filters ports.ModelFilters) bool {
	// Check endpoint filter
	if filters.Endpoint != "" {
		hasEndpoint := false
		for _, ep := range model.SourceEndpoints {
			// Note: This checks URL, but handler will need to map names
			if ep.EndpointURL == filters.Endpoint {
				hasEndpoint = true
				break
			}
		}
		if !hasEndpoint {
			return false
		}
	}

	// Check availability filter
	if filters.Available != nil {
		isAvailable := false
		for _, ep := range model.SourceEndpoints {
			if ep.State == "loaded" {
				isAvailable = true
				break
			}
		}
		if *filters.Available != isAvailable {
			return false
		}
	}

	// Check family filter
	if filters.Family != "" && model.Family != filters.Family {
		return false
	}

	// Check type filter
	if filters.Type != "" {
		hasType := false
		// Check metadata first
		if modelType, ok := model.Metadata["type"].(string); ok && modelType == filters.Type {
			hasType = true
		}
		// Check capabilities as fallback
		if !hasType {
			switch filters.Type {
			case "llm":
				hasType = hasCapability(model.Capabilities, "chat") || hasCapability(model.Capabilities, "completion")
			case "vlm":
				hasType = hasCapability(model.Capabilities, "vision")
			case "embeddings":
				hasType = hasCapability(model.Capabilities, "embedding") || hasCapability(model.Capabilities, "embeddings")
			}
		}
		if !hasType {
			return false
		}
	}

	return true
}

func hasCapability(capabilities []string, cap string) bool {
	for _, c := range capabilities {
		if c == cap {
			return true
		}
	}
	return false
}