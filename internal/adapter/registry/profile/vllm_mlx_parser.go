package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

type vllmMLXParser struct{}

// Parse transforms vLLM-MLX /v1/models response into unified ModelInfo structures.
// vLLM-MLX exclusively serves MLX models, so the format hint is always set to RecipeMLX.
func (p *vllmMLXParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response VLLMMLXResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse vLLM-MLX response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     constants.ProviderTypeVLLMMLX,
			LastSeen: now,
		}

		details := &domain.ModelDetails{}

		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
		}

		// "vllm-mlx" and "vllm" are default owned_by values with no meaningful attribution.
		if model.OwnedBy != "" && model.OwnedBy != "vllm-mlx" && model.OwnedBy != constants.ProviderTypeVLLM {
			details.Publisher = &model.OwnedBy
		}

		// vLLM-MLX exclusively serves MLX models — set the format hint unconditionally
		// so the unifier can apply correct model-matching logic.
		format := constants.RecipeMLX
		details.Format = &format

		modelInfo.Details = details

		models = append(models, modelInfo)
	}

	return models, nil
}
