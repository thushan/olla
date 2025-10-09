package profile

import (
	"fmt"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

type lemonadeParser struct{}

func (p *lemonadeParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LemonadeResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Lemonade response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     "lemonade",
			LastSeen: now,
		}

		// Lemonade provides extended metadata beyond standard OpenAI
		details := &domain.ModelDetails{}
		hasDetails := false

		// Set creation time if available
		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
			hasDetails = true
		}

		// Store checkpoint (HuggingFace model path)
		// Critical for Lemonade's model identification and loading
		if model.Checkpoint != "" {
			details.Checkpoint = &model.Checkpoint
			hasDetails = true

			// Extract publisher from checkpoint path (before first "/")
			// HuggingFace organisation prefix (e.g., "amd" from "amd/model-name")
			if idx := strings.Index(model.Checkpoint, "/"); idx > 0 {
				publisher := model.Checkpoint[:idx]
				details.Publisher = &publisher
			}
		}

		// Store recipe (inference engine identifier)
		// Determines which backend handles execution (oga-cpu, oga-npu, llamacpp, flm)
		if model.Recipe != "" {
			details.Recipe = &model.Recipe
			hasDetails = true

			// Infer format from recipe for compatibility checking
			format := inferFormatFromRecipe(model.Recipe)
			if format != "" {
				details.Format = &format
			}
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

// inferFormatFromRecipe maps Lemonade recipes to model formats
// This allows proper routing and compatibility checking
func inferFormatFromRecipe(recipe string) string {
	if strings.HasPrefix(recipe, "oga-") {
		return constants.RecipeOnnx // ONNX Runtime recipes (oga-cpu, oga-npu, oga-igpu)
	}
	if recipe == constants.RecipeLlamaCpp || recipe == constants.RecipeFLM {
		return constants.RecipeGGUF // GGUF format for llama.cpp and FLM
	}
	return ""
}
