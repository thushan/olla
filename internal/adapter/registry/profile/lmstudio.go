package profile

import (
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	LMStudioProfileVersion    = "1.0"
	LMStudioProfileModelsPath = "/v1/models"
)

type LMStudioProfile struct{}

func NewLMStudioProfile() *LMStudioProfile {
	return &LMStudioProfile{}
}

func (p *LMStudioProfile) GetName() string {
	return domain.ProfileLmStudio
}

func (p *LMStudioProfile) GetVersion() string {
	return LMStudioProfileVersion
}

func (p *LMStudioProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + LMStudioProfileModelsPath
}

func (p *LMStudioProfile) GetHealthCheckPath() string {
	return LMStudioProfileModelsPath
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
		PathIndicators:    []string{LMStudioProfileModelsPath},
	}
}
