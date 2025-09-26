package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

type sglangParser struct{}

func (p *sglangParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response SGLangResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse SGLang response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     "sglang", // Mark as SGLang model for proper handling
			LastSeen: now,
		}

		// SGLang provides richer metadata than standard OpenAI
		details := &domain.ModelDetails{}
		hasDetails := false

		// Extract max context length - crucial for SGLang models
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
		if model.OwnedBy != "" && model.OwnedBy != "sglang" {
			details.Publisher = &model.OwnedBy
			hasDetails = true
		}

		// Set parent model if this is a fine-tuned model
		if model.Parent != nil {
			details.ParentModel = model.Parent
			hasDetails = true
		}

		// SGLang-specific features are captured as model capabilities
		// Vision capability detection can be inferred from model name patterns
		// RadixAttention cache, speculative decoding, and frontend language support
		// are SGLang platform features rather than model-specific metadata

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
