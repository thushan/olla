package converter

import (
	"github.com/thushan/olla/internal/core/domain"
)

// BaseConverter provides common functionality for all converters
type BaseConverter struct {
	providerType string
}

// NewBaseConverter creates a new base converter
func NewBaseConverter(providerType string) *BaseConverter {
	return &BaseConverter{
		providerType: providerType,
	}
}

// FindProviderAlias finds the provider-specific alias for a unified model
func (b *BaseConverter) FindProviderAlias(model *domain.UnifiedModel) (string, bool) {
	for _, alias := range model.Aliases {
		if alias.Source == b.providerType {
			return alias.Name, true
		}
	}
	return "", false
}

// FindProviderEndpoint finds the provider-specific endpoint for a unified model
func (b *BaseConverter) FindProviderEndpoint(model *domain.UnifiedModel, providerName string) *domain.SourceEndpoint {
	for i := range model.SourceEndpoints {
		ep := &model.SourceEndpoints[i]
		if providerName != "" && ep.NativeName == providerName {
			return ep
		}
	}
	return nil
}

// ExtractMetadataString safely extracts a string value from metadata
func (b *BaseConverter) ExtractMetadataString(metadata map[string]interface{}, key string) string {
	if val, ok := metadata[key].(string); ok {
		return val
	}
	return ""
}

// ExtractMetadataInt safely extracts an int value from metadata
func (b *BaseConverter) ExtractMetadataInt(metadata map[string]interface{}, key string) int {
	if val, ok := metadata[key].(int); ok {
		return val
	}
	if val, ok := metadata[key].(float64); ok {
		return int(val)
	}
	return 0
}

// ExtractMetadataBool safely extracts a bool value from metadata
func (b *BaseConverter) ExtractMetadataBool(metadata map[string]interface{}, key string) bool {
	if val, ok := metadata[key].(bool); ok {
		return val
	}
	return false
}

// GetEndpointDiskSize returns the disk size from endpoint or model
func (b *BaseConverter) GetEndpointDiskSize(model *domain.UnifiedModel, endpoint *domain.SourceEndpoint) int64 {
	if endpoint != nil && endpoint.DiskSize > 0 {
		return endpoint.DiskSize
	}
	return model.DiskSize
}

// DetermineModelState determines the model state from endpoint
func (b *BaseConverter) DetermineModelState(endpoint *domain.SourceEndpoint, defaultState string) string {
	if endpoint != nil && endpoint.State != "" {
		return endpoint.State
	}
	return defaultState
}

// HasCapability checks if a model has a specific capability
func (b *BaseConverter) HasCapability(capabilities []string, capability string) bool {
	return hasCapability(capabilities, capability)
}

// DetermineModelType determines the model type based on capabilities and metadata
func (b *BaseConverter) DetermineModelType(model *domain.UnifiedModel, defaultType string) string {
	// Check metadata first
	if modelType := b.ExtractMetadataString(model.Metadata, "type"); modelType != "" {
		return modelType
	}

	// Check capabilities
	if b.HasCapability(model.Capabilities, "vision") {
		return "vlm"
	}
	if b.HasCapability(model.Capabilities, "embedding") || b.HasCapability(model.Capabilities, "embeddings") {
		return "embeddings"
	}

	return defaultType
}

// ConversionHelper groups all helper methods for use in concrete converters
type ConversionHelper struct {
	*BaseConverter
	Model    *domain.UnifiedModel
	Endpoint *domain.SourceEndpoint
	Alias    string
}

// NewConversionHelper creates a helper for converting a specific model
func (b *BaseConverter) NewConversionHelper(model *domain.UnifiedModel) *ConversionHelper {
	alias, _ := b.FindProviderAlias(model)
	endpoint := b.FindProviderEndpoint(model, alias)

	return &ConversionHelper{
		BaseConverter: b,
		Model:         model,
		Alias:         alias,
		Endpoint:      endpoint,
	}
}

// ShouldSkip returns true if this model should be skipped for this provider
func (h *ConversionHelper) ShouldSkip() bool {
	return h.Alias == ""
}

// GetDiskSize returns the appropriate disk size for the model
func (h *ConversionHelper) GetDiskSize() int64 {
	return h.GetEndpointDiskSize(h.Model, h.Endpoint)
}

// GetState returns the model state
func (h *ConversionHelper) GetState(defaultState string) string {
	return h.DetermineModelState(h.Endpoint, defaultState)
}

// GetModelType returns the determined model type
func (h *ConversionHelper) GetModelType(defaultType string) string {
	return h.DetermineModelType(h.Model, defaultType)
}

// GetMetadataString extracts a string from model metadata
func (h *ConversionHelper) GetMetadataString(key string) string {
	return h.ExtractMetadataString(h.Model.Metadata, key)
}

// GetMetadataInt extracts an int from model metadata
func (h *ConversionHelper) GetMetadataInt(key string) int {
	return h.ExtractMetadataInt(h.Model.Metadata, key)
}

// GetMetadataBool extracts a bool from model metadata
func (h *ConversionHelper) GetMetadataBool(key string) bool {
	return h.ExtractMetadataBool(h.Model.Metadata, key)
}
