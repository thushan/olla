package profile

/*

	LMStudioProfile implements the WIP of their API.

	Reference:
	- https://lmstudio.ai/docs/app/api/endpoints/rest#get-apiv0models [10-06-2025]

	GET /api/v0/models
	{
	  "object": "list",
	  "data": [
		{
		  "id": "qwen2-vl-7b-instruct",
		  "object": "model",
		  "type": "vlm",
		  "publisher": "mlx-community",
		  "arch": "qwen2_vl",
		  "compatibility_type": "mlx",
		  "quantization": "4bit",
		  "state": "not-loaded",
		  "max_context_length": 32768
		},
		{
		  "id": "meta-llama-3.1-8b-instruct",
		  "object": "model",
		  "type": "llm",
		  "publisher": "lmstudio-community",
		  "arch": "llama",
		  "compatibility_type": "gguf",
		  "quantization": "Q4_K_M",
		  "state": "not-loaded",
		  "max_context_length": 131072
		},
		{
		  "id": "text-embedding-nomic-embed-text-v1.5",
		  "object": "model",
		  "type": "embeddings",
		  "publisher": "nomic-ai",
		  "arch": "nomic-bert",
		  "compatibility_type": "gguf",
		  "quantization": "Q4_0",
		  "state": "not-loaded",
		  "max_context_length": 2048
		}
	  ]
	}

*/
import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	LMStudioProfileVersion    = "1.0"
	LMStudioProfileModelsPath = "/api/v0/models"
)

type LMStudioResponse struct {
	Object string          `json:"object"`
	Data   []LMStudioModel `json:"data"`
}

type LMStudioModel struct {
	Type              *string `json:"type,omitempty"`
	Publisher         *string `json:"publisher,omitempty"`
	Arch              *string `json:"arch,omitempty"`
	CompatibilityType *string `json:"compatibility_type,omitempty"`
	Quantization      *string `json:"quantization,omitempty"`
	State             *string `json:"state,omitempty"`
	MaxContextLength  *int64  `json:"max_context_length,omitempty"`
	ID                string  `json:"id"`
	Object            string  `json:"object"`
}

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
	}
}

func (p *LMStudioProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"lm-studio/"},
		ResponseHeaders:   []string{"X-LMStudio-Version"},
		PathIndicators:    []string{LMStudioProfileModelsPath},
	}
}

func (p *LMStudioProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LMStudioResponse
	if err := jsoniter.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse LM Studio response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, lmModel := range response.Data {
		if lmModel.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     lmModel.ID,
			Type:     lmModel.Object,
			LastSeen: now,
		}

		hasDetails := false
		details := &domain.ModelDetails{}

		if lmModel.Arch != nil && *lmModel.Arch != "" {
			details.Family = lmModel.Arch
			hasDetails = true
		}

		if lmModel.Quantization != nil && *lmModel.Quantization != "" {
			details.QuantizationLevel = lmModel.Quantization
			hasDetails = true
		}

		if lmModel.CompatibilityType != nil && *lmModel.CompatibilityType != "" {
			details.Format = lmModel.CompatibilityType
			hasDetails = true
		}

		if lmModel.Publisher != nil && *lmModel.Publisher != "" {
			details.ParentModel = lmModel.Publisher
			hasDetails = true
		}

		if lmModel.Type != nil && *lmModel.Type != "" {
			details.Type = lmModel.Type
			hasDetails = true
		}

		if lmModel.MaxContextLength != nil {
			details.MaxContextLength = lmModel.MaxContextLength
			hasDetails = true
		}

		if lmModel.State != nil && *lmModel.State != "" {
			details.State = lmModel.State
			hasDetails = true
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
