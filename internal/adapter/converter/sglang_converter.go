package converter

import (
	"strings"
	"time"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Type aliases for backward compatibility with tests
type SGLangModelResponse = profile.SGLangResponse
type SGLangModelData = profile.SGLangModel

// SGLangConverter converts models to SGLang-compatible format with extended metadata
type SGLangConverter struct {
	*BaseConverter
}

// NewSGLangConverter creates a new SGLang format converter
func NewSGLangConverter() ports.ModelResponseConverter {
	return &SGLangConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeSGLang),
	}
}

func (c *SGLangConverter) GetFormatName() string {
	return constants.ProviderTypeSGLang
}

func (c *SGLangConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.SGLangModel, 0, len(filtered))
	for _, model := range filtered {
		sglangModel := c.convertModel(model)
		if sglangModel != nil {
			data = append(data, *sglangModel)
		}
	}

	return profile.SGLangResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *SGLangConverter) convertModel(model *domain.UnifiedModel) *profile.SGLangModel {
	// For SGLang, prefer the native SGLang name if available from source endpoints
	now := time.Now().Unix()
	modelID := c.findSGLangNativeName(model)
	if modelID == "" {
		// Fallback to first alias or unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	sglangModel := &profile.SGLangModel{
		ID:      modelID,
		Object:  "model",
		Created: now,
		OwnedBy: c.determineOwner(modelID),
		Root:    modelID, // SGLang typically sets root to the model ID
	}

	// Set max context length if available
	if model.MaxContextLength != nil && *model.MaxContextLength > 0 {
		sglangModel.MaxModelLen = model.MaxContextLength
	}

	// Set parent model if available in metadata
	if parentModel := c.getParentModel(model); parentModel != "" {
		sglangModel.Parent = &parentModel
	}

	// SGLang-specific features from metadata
	c.applyMetadataFeatures(model, sglangModel)

	return sglangModel
}

// findSGLangNativeName looks for the native SGLang name from aliases
func (c *SGLangConverter) findSGLangNativeName(model *domain.UnifiedModel) string {
	// Use base converter to find SGLang-specific alias
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the organisation from the model ID or defaults to "sglang"
func (c *SGLangConverter) determineOwner(modelID string) string {
	// SGLang models often follow organisation/model-name pattern
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return constants.ProviderTypeSGLang
}

// getParentModel attempts to find parent model information
func (c *SGLangConverter) getParentModel(model *domain.UnifiedModel) string {
	// Check metadata first
	if parentModel := c.ExtractMetadataString(model.Metadata, "parent_model"); parentModel != "" {
		return parentModel
	}

	return ""
}

// applyMetadataFeatures applies SGLang-specific features from model metadata
func (c *SGLangConverter) applyMetadataFeatures(model *domain.UnifiedModel, sglangModel *profile.SGLangModel) {
	// Vision capability detection from capabilities or metadata
	if c.HasCapability(model.Capabilities, "vision") || c.ExtractMetadataBool(model.Metadata, "supports_vision") {
		supportsVision := true
		sglangModel.SupportsVision = &supportsVision
	}

	// RadixAttention cache size from metadata
	if cacheSize := c.extractMetadataInt64(model.Metadata, "radix_cache_size"); cacheSize > 0 {
		sglangModel.RadixCacheSize = &cacheSize
	}

	// Speculative decoding support from metadata
	if specDecoding := c.ExtractMetadataBool(model.Metadata, "speculative_decoding"); specDecoding {
		sglangModel.SpecDecoding = &specDecoding
	}

	// Frontend Language support from metadata
	if frontendEnabled := c.ExtractMetadataBool(model.Metadata, "frontend_enabled"); frontendEnabled {
		sglangModel.FrontendEnabled = &frontendEnabled
	}
}

// extractMetadataInt64 safely extracts an int64 value from metadata
func (c *SGLangConverter) extractMetadataInt64(metadata map[string]interface{}, key string) int64 {
	if val, ok := metadata[key].(int64); ok {
		return val
	}
	if val, ok := metadata[key].(int); ok {
		return int64(val)
	}
	if val, ok := metadata[key].(float64); ok {
		return int64(val)
	}
	return 0
}
