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
type VLLMMLXResponse = profile.VLLMMLXResponse
type VLLMMLXModel = profile.VLLMMLXModel

// VLLMMLXConverter converts unified models to vLLM-MLX-compatible format.
// vLLM-MLX serves MLX models on Apple Silicon and uses the standard OpenAI-compatible list format.
type VLLMMLXConverter struct {
	*BaseConverter
}

// NewVLLMMLXConverter creates a new vLLM-MLX format converter.
func NewVLLMMLXConverter() ports.ModelResponseConverter {
	return &VLLMMLXConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeVLLMMLX),
	}
}

func (c *VLLMMLXConverter) GetFormatName() string {
	return constants.ProviderTypeVLLMMLX
}

func (c *VLLMMLXConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.VLLMMLXModel, 0, len(filtered))
	for _, model := range filtered {
		mlxModel := c.convertModel(model)
		if mlxModel != nil {
			data = append(data, *mlxModel)
		}
	}

	return profile.VLLMMLXResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *VLLMMLXConverter) convertModel(model *domain.UnifiedModel) *profile.VLLMMLXModel {
	modelID := c.findVLLMMLXNativeName(model)
	if modelID == "" {
		// Fall back to first alias, then unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	return &profile.VLLMMLXModel{
		ID:      modelID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: c.determineOwner(modelID),
	}
}

// findVLLMMLXNativeName looks for the native vLLM-MLX name from aliases.
func (c *VLLMMLXConverter) findVLLMMLXNativeName(model *domain.UnifiedModel) string {
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the namespace from a HuggingFace-style model ID
// (e.g. "mlx-community/Model-Name" -> "mlx-community"), defaulting to "vllm-mlx"
// when the ID has no "/" separator.
func (c *VLLMMLXConverter) determineOwner(modelID string) string {
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return constants.ProviderTypeVLLMMLX
}
