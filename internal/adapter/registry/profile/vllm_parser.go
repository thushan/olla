package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

type vllmParser struct{}

func (p *vllmParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response VLLMResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse vLLM response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     "vllm", // Mark as vLLM model for proper handling
			LastSeen: now,
		}

		// vLLM provides richer metadata than standard OpenAI
		details := &domain.ModelDetails{}
		hasDetails := false

		// Extract max context length - crucial for vLLM models
		if model.MaxModelLen != nil && *model.MaxModelLen > 0 {
			details.MaxContextLength = model.MaxModelLen
			hasDetails = true
		}

		// Set creation time if available
		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
			hasDetails = true
		}

		// Extract publisher from OwnedBy field
		if model.OwnedBy != "" && model.OwnedBy != "vllm" {
			details.Publisher = &model.OwnedBy
			hasDetails = true
		}

		// Set parent model if this is a fine-tuned model
		if model.Parent != nil {
			details.ParentModel = model.Parent
			hasDetails = true
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
