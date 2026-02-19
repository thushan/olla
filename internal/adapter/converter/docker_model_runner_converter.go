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
type DockerModelRunnerResponse = profile.DockerModelRunnerResponse
type DockerModelRunnerModel = profile.DockerModelRunnerModel

// DockerModelRunnerConverter converts unified models to Docker Model Runner-compatible format
type DockerModelRunnerConverter struct {
	*BaseConverter
}

// NewDockerModelRunnerConverter creates a new Docker Model Runner format converter
func NewDockerModelRunnerConverter() ports.ModelResponseConverter {
	return &DockerModelRunnerConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeDockerMR),
	}
}

func (c *DockerModelRunnerConverter) GetFormatName() string {
	return constants.ProviderTypeDockerMR
}

func (c *DockerModelRunnerConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]profile.DockerModelRunnerModel, 0, len(filtered))
	for _, model := range filtered {
		dmrModel := c.convertModel(model)
		if dmrModel != nil {
			data = append(data, *dmrModel)
		}
	}

	return profile.DockerModelRunnerResponse{
		Object: "list",
		Data:   data,
	}, nil
}

func (c *DockerModelRunnerConverter) convertModel(model *domain.UnifiedModel) *profile.DockerModelRunnerModel {
	now := time.Now().Unix()
	modelID := c.findDMRNativeName(model)
	if modelID == "" {
		// Fall back to first alias, then unified ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		} else {
			modelID = model.ID
		}
	}

	return &profile.DockerModelRunnerModel{
		ID:      modelID,
		Object:  "model",
		Created: now,
		OwnedBy: c.determineOwner(modelID),
	}
}

// findDMRNativeName looks for the native Docker Model Runner name from aliases
func (c *DockerModelRunnerConverter) findDMRNativeName(model *domain.UnifiedModel) string {
	alias, found := c.BaseConverter.FindProviderAlias(model)
	if found {
		return alias
	}
	return ""
}

// determineOwner extracts the organisation from the model ID's namespace prefix,
// defaulting to "docker" when the ID has no namespace (i.e. no "/" separator).
func (c *DockerModelRunnerConverter) determineOwner(modelID string) string {
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return "docker"
}
