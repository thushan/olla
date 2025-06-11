package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type OpenAICompatibleProfile struct{}

func NewOpenAICompatibleProfile() *OpenAICompatibleProfile {
	return &OpenAICompatibleProfile{}
}

func (p *OpenAICompatibleProfile) GetName() string {
	return domain.ProfileOpenAICompatible
}

func (p *OpenAICompatibleProfile) GetVersion() string {
	return "1.0"
}

func (p *OpenAICompatibleProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + "/v1/models"
}

func (p *OpenAICompatibleProfile) GetHealthCheckPath() string {
	return "/v1/models"
}

func (p *OpenAICompatibleProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OpenAICompatibleProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: "/v1/chat/completions",
		CompletionsPath:     "/v1/completions",
		GeneratePath:        "",
		ModelFieldName:      "model",
		SupportsStreaming:   true,
	}
}

func (p *OpenAICompatibleProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "data",
	}
}

func (p *OpenAICompatibleProfile) ParseModel(modelData map[string]interface{}) (*domain.ModelInfo, error) {
	modelInfo := &domain.ModelInfo{
		LastSeen: time.Now(),
	}

	if name := util.GetString(modelData, "id"); name != "" {
		modelInfo.Name = name
	} else {
		return nil, fmt.Errorf("model name is required")
	}

	if objType := util.GetString(modelData, "object"); objType != "" {
		modelInfo.Type = objType
	}

	// OpenAI-compatible APIs typically provide minimal metadata
	// but we'll extract what's available for now
	if created, ok := util.GetFloat64(modelData, "created"); ok {
		createdTime := time.Unix(created, 0)
		modelInfo.Details = &domain.ModelDetails{
			ModifiedAt: &createdTime,
		}
	}

	return modelInfo, nil
}

func (p *OpenAICompatibleProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{},
		ResponseHeaders:   []string{},
		PathIndicators:    []string{"/v1/models"},
	}
}
