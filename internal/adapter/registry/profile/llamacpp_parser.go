package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

type llamaCppParser struct{}

// Parse transforms llama.cpp /v1/models response into unified ModelInfo structures
// llama.cpp serves GGUF models exclusively and uses OpenAI-compatible format
func (p *llamaCppParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		// looks like there's no models available
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LlamaCppResponse
	const LlamaCpp = "llamacpp"

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse llama.cpp response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		// SCOUT-86: Failure when llamacpp has no models
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     LlamaCpp,
			LastSeen: now,
		}

		// llama.cpp provides OpenAI-compatible metadata
		details := &domain.ModelDetails{}

		// Set creation time if available
		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
		}

		// Extract publisher from OwnedBy field
		// Skip "llamacpp" as it's the default value, not an actual publisher
		if model.OwnedBy != "" && model.OwnedBy != LlamaCpp {
			details.Publisher = &model.OwnedBy
		}

		// llama.cpp exclusively serves GGUF format
		// This is a defining architectural characteristic
		format := constants.RecipeGGUF
		details.Format = &format
		modelInfo.Details = details

		models = append(models, modelInfo)
	}

	return models, nil
}
