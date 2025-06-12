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

const (
	LMStudioProfileVersion    = "1.0"
	LMStudioProfileModelsPath = "/api/v0/models"
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
	}
}

func (p *LMStudioProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"lm-studio/"},
		ResponseHeaders:   []string{"X-LMStudio-Version"},
		PathIndicators:    []string{LMStudioProfileModelsPath},
	}
}

func (p *LMStudioProfile) ParseModel(modelData map[string]interface{}) (*domain.ModelInfo, error) {
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

	modelInfo.Details = &domain.ModelDetails{}
	hasDetails := false

	if arch := util.GetString(modelData, "arch"); arch != "" {
		modelInfo.Details.Family = &arch
		hasDetails = true
	}

	if quantization := util.GetString(modelData, "quantization"); quantization != "" {
		modelInfo.Details.QuantizationLevel = &quantization
		hasDetails = true
	}

	if compatType := util.GetString(modelData, "compatibility_type"); compatType != "" {
		modelInfo.Details.Format = &compatType
		hasDetails = true
	}

	if publisher := util.GetString(modelData, "publisher"); publisher != "" {
		modelInfo.Details.ParentModel = &publisher
		hasDetails = true
	}

	if kind := util.GetString(modelData, "type"); kind != "" {
		modelInfo.Details.Type = &kind
		hasDetails = true
	}

	if maxCtx, ok := util.GetFloat64(modelData, "max_context_length"); ok {
		modelInfo.Details.MaxContextLength = &maxCtx
		hasDetails = true
	}

	// Check model state
	// LMStudio:  "loaded","not-loaded"
	if state := util.GetString(modelData, "state"); state != "" {
		modelInfo.Details.State = &state
		hasDetails = true
	}

	if !hasDetails {
		modelInfo.Details = nil
	}

	return modelInfo, nil
}
