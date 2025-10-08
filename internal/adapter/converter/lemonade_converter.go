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
type LemonadeModelResponse = profile.LemonadeResponse
type LemonadeModelData = profile.LemonadeModel

// LemonadeConverter converts models to Lemonade-compatible format with extended metadata
type LemonadeConverter struct {
	*BaseConverter
}

// NewLemonadeConverter creates a new Lemonade format converter
func NewLemonadeConverter() ports.ModelResponseConverter {
	return &LemonadeConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeLemonade),
	}
}

func (c *LemonadeConverter) GetFormatName() string {
	return constants.ProviderTypeLemonade
}

func (c *LemonadeConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.LemonadeModel, 0, len(filtered))
	for _, model := range filtered {
		lemonadeModel := c.convertModel(model)
		if lemonadeModel != nil {
			data = append(data, *lemonadeModel)
		}
	}

	return profile.LemonadeResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *LemonadeConverter) convertModel(model *domain.UnifiedModel) *profile.LemonadeModel {
	// For Lemonade, prefer the native Lemonade name if available from source endpoints
	now := time.Now().Unix()
	modelID := c.findLemonadeNativeName(model)
	if modelID == "" {
		// Fallback to first alias or unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	lemonadeModel := &profile.LemonadeModel{
		ID:      modelID,
		Object:  "model",
		Created: now,
		OwnedBy: c.determineOwner(modelID),
	}

	// Extract Lemonade-specific metadata features
	c.applyMetadataFeatures(model, lemonadeModel)

	return lemonadeModel
}

// findLemonadeNativeName looks for the native Lemonade name from aliases
// This ensures we use the backend's original identifier rather than our unified name
func (c *LemonadeConverter) findLemonadeNativeName(model *domain.UnifiedModel) string {
	// Use base converter to find Lemonade-specific alias
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the organisation from the model ID or defaults to "lemonade"
func (c *LemonadeConverter) determineOwner(modelID string) string {
	// Lemonade models may follow organisation/model-name pattern from checkpoint
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return constants.ProviderTypeLemonade
}

// applyMetadataFeatures applies Lemonade-specific metadata features
// Extracts checkpoint and recipe fields critical for Lemonade runtime
func (c *LemonadeConverter) applyMetadataFeatures(model *domain.UnifiedModel, lemonadeModel *profile.LemonadeModel) {
	if model.Metadata == nil {
		return
	}

	// Checkpoint (HuggingFace model path) - critical for model identification
	if checkpoint, ok := model.Metadata["checkpoint"].(string); ok {
		lemonadeModel.Checkpoint = checkpoint
	}

	// Recipe (inference engine) - determines which backend handles execution
	if recipe, ok := model.Metadata["recipe"].(string); ok {
		lemonadeModel.Recipe = recipe
	}
}
