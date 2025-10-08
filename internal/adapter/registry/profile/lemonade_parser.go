package profile

import (
	"fmt"
	"strings"
	"time"

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
			Type:     "lemonade", // Mark as Lemonade model for proper handling
			LastSeen: now,
		}

		// Lemonade provides local inference metadata beyond standard OpenAI
		details := &domain.ModelDetails{}
		hasDetails := false

		// Set creation time if available
		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
			hasDetails = true
		}

		// Extract publisher from checkpoint path (before first "/")
		// This gives us the HuggingFace organisation (e.g., "amd" from "amd/model-name")
		if model.Checkpoint != "" {
			if idx := strings.Index(model.Checkpoint, "/"); idx > 0 {
				publisher := model.Checkpoint[:idx]
				details.Publisher = &publisher
				hasDetails = true
			}
		}

		// Infer format from recipe - critical for understanding model compatibility
		if model.Recipe != "" {
			format := inferFormatFromRecipe(model.Recipe)
			if format != "" {
				details.Format = &format
				hasDetails = true
			}
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

// inferFormatFromRecipe determines the model format from the recipe
// Lemonade recipes map directly to inference engines and their supported formats
func inferFormatFromRecipe(recipe string) string {
	if strings.HasPrefix(recipe, "oga-") {
		return "onnx" // OGA (ONNX Runtime) recipes use ONNX format
	}
	if recipe == "llamacpp" || recipe == "flm" {
		return "gguf" // llamacpp and FLM use GGUF format
	}
	return ""
}

// inferHardwareFromRecipe determines the target hardware from the recipe
// This helps route models to appropriate endpoints with matching hardware
func inferHardwareFromRecipe(recipe string) string {
	switch recipe {
	case "oga-cpu":
		return "CPU"
	case "oga-npu":
		return "NPU" // AMD Ryzen AI Neural Processing Unit
	case "oga-igpu":
		return "GPU" // Integrated GPU via DirectML
	case "llamacpp", "flm":
		return "GPU" // Typically runs on GPU when available
	default:
		return ""
	}
}

// inferCapabilitiesFromName extracts capabilities from the model name
// Lemonade follows standard model naming conventions for capability detection
func inferCapabilitiesFromName(name string) []string {
	capabilities := make([]string, 0)
	nameLower := strings.ToLower(name)

	// Instruction-following models (chat-capable)
	if strings.Contains(nameLower, "instruct") || strings.Contains(nameLower, "chat") || strings.Contains(nameLower, "-it-") {
		capabilities = append(capabilities, "chat")
	}

	// Coding models
	if strings.Contains(nameLower, "code") || strings.Contains(nameLower, "coder") || strings.Contains(nameLower, "devstral") {
		capabilities = append(capabilities, "code")
	}

	// Vision models
	if strings.Contains(nameLower, "vl-") || strings.Contains(nameLower, "vision") || strings.Contains(nameLower, "scout") {
		capabilities = append(capabilities, "vision")
	}

	// Reasoning models
	if strings.Contains(nameLower, "deepseek-r1") || strings.Contains(nameLower, "cogito") {
		capabilities = append(capabilities, "reasoning")
	}

	// Embedding models
	if strings.Contains(nameLower, "embed") {
		capabilities = append(capabilities, "embeddings")
	}

	// Reranking models
	if strings.Contains(nameLower, "rerank") {
		capabilities = append(capabilities, "reranking")
	}

	return capabilities
}
