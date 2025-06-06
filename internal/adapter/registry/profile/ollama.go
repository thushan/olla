package profile

import (
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
		ModelNameField:  "name",
		ModelSizeField:  "size",
		ModelTypeField:  "",
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
