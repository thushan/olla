package profile

import (
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type LMStudioProfile struct{}

func NewLMStudioProfile() *LMStudioProfile {
	return &LMStudioProfile{}
}

func (p *LMStudioProfile) GetName() string {
	return domain.ProfileLmStudio
}

func (p *LMStudioProfile) GetVersion() string {
	return "1.0"
}

func (p *LMStudioProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + "/v1/models"
}

func (p *LMStudioProfile) GetHealthCheckPath() string {
	return "/v1/models"
}

func (p *LMStudioProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *LMStudioProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: "/v1/chat/completions",
		CompletionsPath:     "/v1/completions",
		GeneratePath:        "",
		ModelFieldName:      "model",
		SupportsStreaming:   true,
	}
}

func (p *LMStudioProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "data",
		ModelNameField:  "id",
		ModelSizeField:  "",
		ModelTypeField:  "object",
	}
}

func (p *LMStudioProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"lm-studio/"},
		ResponseHeaders:   []string{"X-LMStudio-Version"},
		PathIndicators:    []string{"/v1/models"},
	}
}
