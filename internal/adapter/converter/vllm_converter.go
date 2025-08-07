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
type VLLMModelResponse = profile.VLLMResponse
type VLLMModelData = profile.VLLMModel
type VLLMModelPermission = profile.VLLMModelPermission

// VLLMConverter converts models to vLLM-compatible format with extended metadata
type VLLMConverter struct{}

// NewVLLMConverter creates a new vLLM format converter
func NewVLLMConverter() ports.ModelResponseConverter {
	return &VLLMConverter{}
}

func (c *VLLMConverter) GetFormatName() string {
	return constants.ProviderTypeVLLM
}

func (c *VLLMConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.VLLMModel, 0, len(filtered))
	for _, model := range filtered {
		vllmModel := c.convertModel(model)
		if vllmModel != nil {
			data = append(data, *vllmModel)
		}
	}

	return profile.VLLMResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *VLLMConverter) convertModel(model *domain.UnifiedModel) *profile.VLLMModel {
	// For vLLM, prefer the native vLLM name if available from source endpoints
	modelID := c.findVLLMNativeName(model)
	if modelID == "" {
		// Fallback to first alias or unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	vllmModel := &profile.VLLMModel{
		ID:      modelID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: c.determineOwner(modelID),
		Root:    modelID, // vLLM typically sets root to the model ID
	}

	// Set max context length if available
	if model.MaxContextLength != nil && *model.MaxContextLength > 0 {
		vllmModel.MaxModelLen = model.MaxContextLength
	}

	// Generate default permissions that allow all operations
	vllmModel.Permission = []profile.VLLMModelPermission{
		{
			ID:                 "modelperm-olla-" + strings.ReplaceAll(modelID, "/", "-"),
			Object:             "model_permission",
			Created:            time.Now().Unix(),
			AllowCreateEngine:  false, // Engine creation not applicable in proxy context
			AllowSampling:      true,
			AllowLogprobs:      true,
			AllowSearchIndices: false,
			AllowView:          true,
			AllowFineTuning:    false,
			Organization:       "*",
			IsBlocking:         false,
		},
	}

	return vllmModel
}

// findVLLMNativeName looks for the native vLLM name from aliases
func (c *VLLMConverter) findVLLMNativeName(model *domain.UnifiedModel) string {
	// Check aliases for vLLM source - this reliably identifies vLLM models
	// We don't check NativeName for slashes as other providers (Ollama, etc.) also use them
	for _, alias := range model.Aliases {
		if alias.Source == constants.ProviderTypeVLLM {
			return alias.Name
		}
	}

	return ""
}

// determineOwner extracts the organisation from the model ID or defaults to "vllm"
func (c *VLLMConverter) determineOwner(modelID string) string {
	// vLLM models often follow organisation/model-name pattern
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return constants.ProviderTypeVLLM
}
