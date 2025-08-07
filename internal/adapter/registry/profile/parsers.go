package profile

import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type ModelResponseParser interface {
	Parse(data []byte) ([]*domain.ModelInfo, error)
}

func getParserForFormat(format string) ModelResponseParser {
	switch format {
	case constants.ProviderTypeOllama:
		return &ollamaParser{}
	case constants.ProviderPrefixLMStudio1:
		return &lmStudioParser{}
	case constants.ProviderTypeVLLM:
		return &vllmParser{}
	case constants.ProviderTypeOpenAI:
		return &openAIParser{}
	default:
		// openai format is the de facto standard
		return &openAIParser{}
	}
}

type ollamaParser struct{}

func (p *ollamaParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OllamaResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Models))
	now := time.Now()

	for _, ollamaModel := range response.Models {
		if ollamaModel.Name == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     ollamaModel.Name,
			LastSeen: now,
		}

		if ollamaModel.Size != nil {
			modelInfo.Size = *ollamaModel.Size
		}

		if ollamaModel.Description != nil {
			modelInfo.Description = *ollamaModel.Description
		}

		if ollamaModel.Details != nil || ollamaModel.Digest != nil || ollamaModel.ModifiedAt != nil {
			modelInfo.Details = createOllamaModelDetails(ollamaModel)
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

func createOllamaModelDetails(ollamaModel OllamaModel) *domain.ModelDetails {
	details := &domain.ModelDetails{}

	if ollamaModel.Digest != nil {
		details.Digest = ollamaModel.Digest
	}

	if ollamaModel.ModifiedAt != nil {
		if parsedTime := util.ParseTime(*ollamaModel.ModifiedAt); parsedTime != nil {
			details.ModifiedAt = parsedTime
		}
	}

	if ollamaModel.Details != nil {
		if ollamaModel.Details.ParameterSize != nil {
			details.ParameterSize = ollamaModel.Details.ParameterSize
		}
		if ollamaModel.Details.QuantizationLevel != nil {
			details.QuantizationLevel = ollamaModel.Details.QuantizationLevel
		}
		if ollamaModel.Details.Family != nil {
			details.Family = ollamaModel.Details.Family
		}
		if ollamaModel.Details.Format != nil {
			details.Format = ollamaModel.Details.Format
		}
		if ollamaModel.Details.ParentModel != nil {
			details.ParentModel = ollamaModel.Details.ParentModel
		}
		if len(ollamaModel.Details.Families) > 0 {
			details.Families = ollamaModel.Details.Families
		}
	}
	return details
}

type lmStudioParser struct{}

func (p *lmStudioParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LMStudioResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse LM Studio response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     model.Object, // "model" from the response
			LastSeen: now,
		}

		// Extract any available metadata from the model
		details := &domain.ModelDetails{}
		hasDetails := false

		if model.Type != nil {
			details.Type = model.Type
			hasDetails = true
		}

		if model.Publisher != nil {
			details.Publisher = model.Publisher
			details.ParentModel = model.Publisher // lmstudio quirk
			hasDetails = true
		}

		if model.Quantization != nil {
			details.QuantizationLevel = model.Quantization
			hasDetails = true
		}

		if model.Arch != nil {
			details.Family = model.Arch
			// Also populate Families array for compatibility
			details.Families = []string{*model.Arch}
			hasDetails = true
		}

		if model.CompatibilityType != nil {
			details.Format = model.CompatibilityType // another lmstudio quirk
			hasDetails = true
		}

		if model.State != nil {
			details.State = model.State
			hasDetails = true
		}

		if model.MaxContextLength != nil {
			details.MaxContextLength = model.MaxContextLength
			hasDetails = true
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

type openAIParser struct{}

func (p *openAIParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OpenAICompatibleResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI-compatible response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     model.Object, // always "model" in openai responses
			LastSeen: now,
		}

		// openai is stingy with metadata
		if (model.Created != nil && *model.Created > 0) || model.OwnedBy != nil {
			details := &domain.ModelDetails{}

			if model.Created != nil && *model.Created > 0 {
				createdTime := time.Unix(*model.Created, 0)
				details.ModifiedAt = &createdTime
			}

			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
