package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type OpenAICompatibleResponse struct {
	Object string                  `json:"object"`
	Data   []OpenAICompatibleModel `json:"data"`
}

type OpenAICompatibleModel struct {
	Created *int64  `json:"created,omitempty"`
	OwnedBy *string `json:"owned_by,omitempty"`
	ID      string  `json:"id"`
	Object  string  `json:"object"`
}

const (
	OpenAICompatibleProfileVersion = "1.0"
	OpenAIModelsPathIndex          = 0
	OpenAIChatCompletionsPathIndex = 1
	OpenAICompletionsPathIndex     = 2
)

var openAIPaths []string

func init() {
	openAIPaths = []string{
		"/v1/models",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
		// just the basics - most openai clones only implement these four
	}
}

type OpenAICompatibleProfile struct{}

func NewOpenAICompatibleProfile() *OpenAICompatibleProfile {
	return &OpenAICompatibleProfile{}
}

func (p *OpenAICompatibleProfile) GetName() string {
	return domain.ProfileOpenAICompatible
}

func (p *OpenAICompatibleProfile) GetVersion() string {
	return OpenAICompatibleProfileVersion
}

func (p *OpenAICompatibleProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + openAIPaths[OpenAIModelsPathIndex]
}

func (p *OpenAICompatibleProfile) GetHealthCheckPath() string {
	return openAIPaths[OpenAIModelsPathIndex]
}

func (p *OpenAICompatibleProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OpenAICompatibleProfile) GetPaths() []string {
	return openAIPaths
}

func (p *OpenAICompatibleProfile) GetPath(index int) string {
	if index < 0 || index >= len(openAIPaths) {
		return ""
	}
	return openAIPaths[index]
}

func (p *OpenAICompatibleProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: openAIPaths[OpenAIChatCompletionsPathIndex],
		CompletionsPath:     openAIPaths[OpenAICompletionsPathIndex],
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

func (p *OpenAICompatibleProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OpenAICompatibleResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI compatible response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, openaiModel := range response.Data {
		if openaiModel.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     openaiModel.ID,
			Type:     openaiModel.Object,
			LastSeen: now,
		}

		if openaiModel.Created != nil {
			createdTime := time.Unix(*openaiModel.Created, 0)
			modelInfo.Details = &domain.ModelDetails{
				ModifiedAt: &createdTime,
			}
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

func (p *OpenAICompatibleProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{},
		ResponseHeaders:   []string{},
		PathIndicators:    []string{openAIPaths[OpenAIModelsPathIndex]},
	}
}
