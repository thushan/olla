package profile

import (
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
		ModelNameField:  "id",
		ModelSizeField:  "",
		ModelTypeField:  "object",
	}
}

func (p *OpenAICompatibleProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{},
		ResponseHeaders:   []string{},
		PathIndicators:    []string{"/v1/models"},
	}
}
