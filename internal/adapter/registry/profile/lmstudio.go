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

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
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

const (
	LMStudioProfileVersion           = "1.0"
	LMStudioModelsPathIndex          = 0
	LMStudioChatCompletionsPathIndex = 1
	LMStudioCompletionsPathIndex     = 2
	LMStudioEmbeddingsPathIndex      = 3
)

var lmstudioPaths []string

func init() {
	lmstudioPaths = []string{
		// LM Studio native API (beta) [10-06-2025]
		// src: https://lmstudio.ai/docs/app/api/endpoints/rest#get-apiv0models
		"/api/v0/models",           // Enhanced model info with stats
		"/api/v0/chat/completions", // Chat with enhanced stats
		"/api/v0/completions",      // Text completion with stats
		"/api/v0/embeddings",       // Embeddings

		// OpenAI compatibility layer
		"/v1/models",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
	}
}
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
	return util.NormaliseBaseURL(baseURL) + lmstudioPaths[LMStudioModelsPathIndex]
}

func (p *LMStudioProfile) GetHealthCheckPath() string {
	return lmstudioPaths[LMStudioModelsPathIndex]
}

func (p *LMStudioProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *LMStudioProfile) GetPaths() []string {
	return lmstudioPaths
}

func (p *LMStudioProfile) GetPath(index int) string {
	if index < 0 || index >= len(lmstudioPaths) {
		return ""
	}
	return lmstudioPaths[index]
}

func (p *LMStudioProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: lmstudioPaths[LMStudioChatCompletionsPathIndex],
		CompletionsPath:     lmstudioPaths[LMStudioCompletionsPathIndex],
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
		PathIndicators:    []string{lmstudioPaths[LMStudioModelsPathIndex]},
	}
}

func (p *LMStudioProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LMStudioResponse
	if err := json.Unmarshal(data, &response); err != nil {
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
			details.Families = []string{*lmModel.Arch}
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
