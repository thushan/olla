package profile

import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type OpenAICompatibleResponse struct {
	Object string                  `json:"object"`
	Data   []OpenAICompatibleModel `json:"data"`
}

type OpenAICompatibleModel struct {
	ID      string  `json:"id"`
	Object  string  `json:"object"`
	Created *int64  `json:"created,omitempty"`
	OwnedBy *string `json:"owned_by,omitempty"`
}

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

func (p *OpenAICompatibleProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OpenAICompatibleResponse
	if err := jsoniter.Unmarshal(data, &response); err != nil {
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
		PathIndicators:    []string{"/v1/models"},
	}
}
