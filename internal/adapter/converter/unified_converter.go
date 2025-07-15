package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// UnifiedModelResponse represents the unified Olla format response
type UnifiedModelResponse struct {
	Object string             `json:"object"`
	Data   []UnifiedModelData `json:"data"`
}

// UnifiedModelData represents a single model in the unified format
type UnifiedModelData struct {
	Olla    *OllaExtensions `json:"olla,omitempty"`
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	OwnedBy string          `json:"owned_by"`
	Created int64           `json:"created"`
}

// OllaExtensions contains Olla-specific model information
type OllaExtensions struct {
	MaxContextLength *int64           `json:"max_context_length,omitempty"`
	Family           string           `json:"family"`
	Variant          string           `json:"variant"`
	ParameterSize    string           `json:"parameter_size"`
	Quantization     string           `json:"quantization"`
	PromptTemplateID string           `json:"prompt_template_id,omitempty"`
	Aliases          []string         `json:"aliases"`
	Availability     []EndpointStatus `json:"availability"`
	Capabilities     []string         `json:"capabilities"`
}

// EndpointStatus represents model availability on an endpoint
type EndpointStatus struct {
	Endpoint string `json:"endpoint"` // Endpoint name only (no URL for security)
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
			Endpoint: ep.EndpointName, // Use endpoint name instead of URL
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
	if !matchesEndpointFilter(model, filters.Endpoint) {
		return false
	}

	if !matchesAvailabilityFilter(model, filters.Available) {
		return false
	}

	if !matchesFamilyFilter(model, filters.Family) {
		return false
	}

	if !matchesTypeFilter(model, filters.Type) {
		return false
	}

	return true
}

func matchesEndpointFilter(model *domain.UnifiedModel, endpoint string) bool {
	if endpoint == "" {
		return true
	}

	for _, ep := range model.SourceEndpoints {
		if ep.EndpointURL == endpoint {
			return true
		}
	}
	return false
}

func matchesAvailabilityFilter(model *domain.UnifiedModel, available *bool) bool {
	if available == nil {
		return true
	}

	isAvailable := false
	for _, ep := range model.SourceEndpoints {
		if ep.State == "loaded" {
			isAvailable = true
			break
		}
	}
	return *available == isAvailable
}

func matchesFamilyFilter(model *domain.UnifiedModel, family string) bool {
	return family == "" || model.Family == family
}

func matchesTypeFilter(model *domain.UnifiedModel, filterType string) bool {
	if filterType == "" {
		return true
	}

	// Check metadata first
	if modelType, ok := model.Metadata["type"].(string); ok && modelType == filterType {
		return true
	}

	// Check capabilities as fallback
	return matchesTypeByCapabilities(model.Capabilities, filterType)
}

func matchesTypeByCapabilities(capabilities []string, filterType string) bool {
	switch filterType {
	case "llm":
		return hasCapability(capabilities, "chat") || hasCapability(capabilities, "completion")
	case "vlm":
		return hasCapability(capabilities, "vision")
	case "embeddings":
		return hasCapability(capabilities, "embedding") || hasCapability(capabilities, "embeddings")
	default:
		return false
	}
}

func hasCapability(capabilities []string, capability string) bool {
	for _, c := range capabilities {
		if c == capability {
			return true
		}
	}
	return false
}
