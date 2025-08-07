package converter

import (
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// VLLMModelResponse represents the vLLM-specific format response
type VLLMModelResponse struct {
	Object string          `json:"object"`
	Data   []VLLMModelData `json:"data"`
}

// VLLMModelData represents a single model in vLLM format with extended metadata
type VLLMModelData struct {
	MaxModelLen *int64                `json:"max_model_len,omitempty"` // vLLM-specific: maximum context length
	Parent      *string               `json:"parent,omitempty"`        // vLLM-specific: parent model for fine-tuned models
	ID          string                `json:"id"`
	Object      string                `json:"object"`
	OwnedBy     string                `json:"owned_by"`
	Root        string                `json:"root,omitempty"`       // vLLM-specific: root model identifier
	Permission  []VLLMModelPermission `json:"permission,omitempty"` // vLLM-specific: access permissions
	Created     int64                 `json:"created"`
}

// VLLMModelPermission represents vLLM's granular permission system
type VLLMModelPermission struct {
	Group              *string `json:"group"`
	ID                 string  `json:"id"`
	Object             string  `json:"object"`
	Organization       string  `json:"organization"`
	Created            int64   `json:"created"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	IsBlocking         bool    `json:"is_blocking"`
}

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

	data := make([]VLLMModelData, 0, len(filtered))
	for _, model := range filtered {
		vllmModel := c.convertModel(model)
		if vllmModel != nil {
			data = append(data, *vllmModel)
		}
	}

	return VLLMModelResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *VLLMConverter) convertModel(model *domain.UnifiedModel) *VLLMModelData {
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

	vllmModel := &VLLMModelData{
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
	vllmModel.Permission = []VLLMModelPermission{
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

// findVLLMNativeName looks for the native vLLM name from source endpoints
func (c *VLLMConverter) findVLLMNativeName(model *domain.UnifiedModel) string {
	for _, endpoint := range model.SourceEndpoints {
		// Check if this is from a vLLM endpoint based on the native name format
		if strings.Contains(endpoint.NativeName, "/") {
			// vLLM models typically use organisation/model-name format
			return endpoint.NativeName
		}
	}

	// Check aliases for vLLM source
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
