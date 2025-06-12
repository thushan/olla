package profile

import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	OllamaProfileVersion             = "1.0"
	OllamaProfileHealthPath          = "/"
	OllamaProfileGeneratePath        = "/api/generate"
	OllamaProfileChatCompletionsPath = "/v1/chat/completions"
	OllamaProfileCompletionsPath     = "/v1/completions"
	OllamaProfileModelModelsPath     = "/api/tags"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type OllamaResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Name        string         `json:"name"`
	Size        *int64         `json:"size,omitempty"`
	Digest      *string        `json:"digest,omitempty"`
	ModifiedAt  *string        `json:"modified_at,omitempty"`
	Description *string        `json:"description,omitempty"`
	Details     *OllamaDetails `json:"details,omitempty"`
}

type OllamaDetails struct {
	ParameterSize     *string  `json:"parameter_size,omitempty"`
	QuantizationLevel *string  `json:"quantization_level,omitempty"`
	Family            *string  `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
	Format            *string  `json:"format,omitempty"`
	ParentModel       *string  `json:"parent_model,omitempty"`
}

type OllamaProfile struct{}

func NewOllamaProfile() *OllamaProfile {
	return &OllamaProfile{}
}

func (p *OllamaProfile) GetName() string {
	return domain.ProfileOllama
}

func (p *OllamaProfile) GetVersion() string {
	return OllamaProfileVersion
}

func (p *OllamaProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + OllamaProfileModelModelsPath
}

func (p *OllamaProfile) GetHealthCheckPath() string {
	return OllamaProfileHealthPath
}

func (p *OllamaProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OllamaProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: "/v1/chat/completions",
		CompletionsPath:     "/v1/completions",
		GeneratePath:        "/api/generate",
		ModelFieldName:      "model",
		SupportsStreaming:   true,
	}
}

func (p *OllamaProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "models",
	}
}

func (p *OllamaProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"ollama/"},
		ResponseHeaders:   []string{"X-ProfileOllama-Version"},
		PathIndicators: []string{
			OllamaProfileModelModelsPath,
			OllamaProfileGeneratePath,
		},
	}
}

func (p *OllamaProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OllamaResponse
	if err := jsoniter.Unmarshal(data, &response); err != nil {
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
			modelInfo.Details = &domain.ModelDetails{}

			if ollamaModel.Digest != nil {
				modelInfo.Details.Digest = ollamaModel.Digest
			}

			if ollamaModel.ModifiedAt != nil {
				if parsedTime := util.ParseTime(*ollamaModel.ModifiedAt); parsedTime != nil {
					modelInfo.Details.ModifiedAt = parsedTime
				}
			}

			if ollamaModel.Details != nil {
				if ollamaModel.Details.ParameterSize != nil {
					modelInfo.Details.ParameterSize = ollamaModel.Details.ParameterSize
				}
				if ollamaModel.Details.QuantizationLevel != nil {
					modelInfo.Details.QuantizationLevel = ollamaModel.Details.QuantizationLevel
				}
				if ollamaModel.Details.Family != nil {
					modelInfo.Details.Family = ollamaModel.Details.Family
				}
				if ollamaModel.Details.Format != nil {
					modelInfo.Details.Format = ollamaModel.Details.Format
				}
				if ollamaModel.Details.ParentModel != nil {
					modelInfo.Details.ParentModel = ollamaModel.Details.ParentModel
				}
				if len(ollamaModel.Details.Families) > 0 {
					modelInfo.Details.Families = ollamaModel.Details.Families
				}
			}
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
