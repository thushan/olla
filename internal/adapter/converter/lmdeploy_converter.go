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
type LMDeployModelResponse = profile.LMDeployResponse
type LMDeployModelData = profile.LMDeployModel

// LMDeployConverter converts models to LMDeploy-compatible format.
// LMDeploy's /v1/models shape is OpenAI-compatible but without max_model_len
// and with owned_by defaulting to "lmdeploy".
type LMDeployConverter struct {
	*BaseConverter
}

// NewLMDeployConverter creates a new LMDeploy format converter.
func NewLMDeployConverter() ports.ModelResponseConverter {
	return &LMDeployConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeLMDeploy),
	}
}

func (c *LMDeployConverter) GetFormatName() string {
	return constants.ProviderTypeLMDeploy
}

func (c *LMDeployConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.LMDeployModel, 0, len(filtered))
	for _, model := range filtered {
		m := c.convertModel(model)
		if m != nil {
			data = append(data, *m)
		}
	}

	return profile.LMDeployResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *LMDeployConverter) convertModel(model *domain.UnifiedModel) *profile.LMDeployModel {
	now := time.Now().Unix()

	modelID := c.findLMDeployNativeName(model)
	if modelID == "" {
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	m := &profile.LMDeployModel{
		ID:      modelID,
		Object:  "model",
		Created: now,
		OwnedBy: c.determineOwner(modelID),
	}

	// LMDeploy does not expose max_model_len on the wire; omit it here too.

	// Generate standard permissions mirroring the LMDeploy default.
	m.Permission = []profile.LMDeployModelPermission{
		{
			ID:                 "modelperm-olla-" + strings.ReplaceAll(modelID, "/", "-"),
			Object:             "model_permission",
			Created:            now,
			AllowCreateEngine:  false,
			AllowSampling:      true,
			AllowLogprobs:      true,
			AllowSearchIndices: false,
			AllowView:          true,
			AllowFineTuning:    false,
			Organization:       "*",
			IsBlocking:         false,
		},
	}

	return m
}

func (c *LMDeployConverter) findLMDeployNativeName(model *domain.UnifiedModel) string {
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the organisation from org/model-name style IDs,
// defaulting to "lmdeploy" when there is no slash.
func (c *LMDeployConverter) determineOwner(modelID string) string {
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return constants.ProviderTypeLMDeploy
}
