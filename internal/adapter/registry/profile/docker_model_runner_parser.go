package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

type dockerModelRunnerParser struct{}

// Parse transforms Docker Model Runner /engines/v1/models response into unified ModelInfo structures.
// DMR uses the standard OpenAI-compatible list format, so the structure mirrors llamaCppParser.
func (p *dockerModelRunnerParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response DockerModelRunnerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Docker Model Runner response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     constants.ProviderTypeDockerMR,
			LastSeen: now,
		}

		details := &domain.ModelDetails{}

		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
		}

		// "docker" is the default owned_by value — not a meaningful publisher.
		// Only set Publisher when the field carries real attribution.
		if model.OwnedBy != "" && model.OwnedBy != "docker" {
			details.Publisher = &model.OwnedBy
		}

		// DMR primarily serves GGUF models via its llama.cpp engine. Set the format
		// hint so the unifier can apply correct model-matching logic.
		format := constants.RecipeGGUF
		details.Format = &format

		modelInfo.Details = details

		models = append(models, modelInfo)
	}

	return models, nil
}
