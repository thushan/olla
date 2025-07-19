package profile

import (
	"fmt"
	"strings"
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

// InferenceProfile implementation

func (p *OpenAICompatibleProfile) GetTimeout() time.Duration {
	// Cloud providers typically respond quickly
	return 30 * time.Second
}

func (p *OpenAICompatibleProfile) GetMaxConcurrentRequests() int {
	// Cloud providers can handle many concurrent requests
	return 100
}

func (p *OpenAICompatibleProfile) ValidateEndpoint(endpoint *domain.Endpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint cannot be nil")
	}
	if endpoint.URL == nil {
		return fmt.Errorf("endpoint URL cannot be nil")
	}
	// Many OpenAI compatible APIs require auth
	// TODO: Check API key when endpoint has auth config
	return nil
}

func (p *OpenAICompatibleProfile) GetDefaultPriority() int {
	// Cloud endpoints typically have lower priority than local
	return 5
}

func (p *OpenAICompatibleProfile) GetConfig() *domain.ProfileConfig {
	return &domain.ProfileConfig{
		Name:        "openai_compatible",
		Version:     OpenAICompatibleProfileVersion,
		DisplayName: "OpenAI Compatible",
		Description: "OpenAI API compatible endpoints",
		Models: struct {
			CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
			NameFormat         string              `yaml:"name_format"`
		}{
			CapabilityPatterns: map[string][]string{
				"chat":       {"gpt-*", "claude-*", "llama-*", "mistral-*", "*chat*"},
				"completion": {"text-*", "davinci-*", "curie-*", "babbage-*", "ada-*"},
				"embedding":  {"*embedding*", "text-embedding-*"},
				"vision":     {"gpt-4-vision*", "gpt-4v*", "claude-3-*"},
				"code":       {"*code*", "gpt-4*", "claude-3-*"},
			},
			NameFormat: "{{.Name}}",
		},
	}
}

func (p *OpenAICompatibleProfile) GetModelCapabilities(modelName string, registry domain.ModelRegistry) domain.ModelCapabilities {
	caps := domain.ModelCapabilities{
		ChatCompletion:   true,
		TextGeneration:   true,
		StreamingSupport: true,
		MaxContextLength: 4096, // Conservative default
		MaxOutputTokens:  4096,
	}

	lowerName := strings.ToLower(modelName)

	// GPT-4 variants
	if strings.HasPrefix(lowerName, "gpt-4") {
		caps.FunctionCalling = true
		caps.CodeGeneration = true

		switch {
		case strings.Contains(lowerName, "turbo"):
			caps.MaxContextLength = 128000
		case strings.Contains(lowerName, "32k"):
			caps.MaxContextLength = 32768
		default:
			caps.MaxContextLength = 8192
		}

		if strings.Contains(lowerName, "vision") || strings.Contains(lowerName, "gpt-4v") {
			caps.VisionUnderstanding = true
		}
	}

	// GPT-3.5 variants
	if strings.HasPrefix(lowerName, "gpt-3.5") {
		caps.FunctionCalling = true

		if strings.Contains(lowerName, "16k") {
			caps.MaxContextLength = 16384
		} else {
			caps.MaxContextLength = 4096
		}
	}

	// Claude variants
	if strings.HasPrefix(lowerName, "claude") {
		caps.CodeGeneration = true

		switch {
		case strings.Contains(lowerName, "claude-3"):
			caps.VisionUnderstanding = true
			caps.FunctionCalling = true
			caps.MaxContextLength = 200000
		case strings.Contains(lowerName, "claude-2"):
			caps.MaxContextLength = 100000
		default:
			caps.MaxContextLength = 9000
		}
	}

	// Embeddings models
	if strings.Contains(lowerName, "embedding") {
		caps.Embeddings = true
		caps.ChatCompletion = false
		caps.TextGeneration = false
		caps.FunctionCalling = false
		caps.MaxOutputTokens = 0 // Embeddings don't have text output
		// Keep the default context length of 4096 for embeddings
	}

	// Legacy completion models (but not embeddings)
	if !strings.Contains(lowerName, "embedding") &&
		(strings.Contains(lowerName, "davinci") || strings.Contains(lowerName, "curie") ||
			strings.Contains(lowerName, "babbage") || strings.Contains(lowerName, "ada")) {
		caps.ChatCompletion = false
		caps.FunctionCalling = false
		caps.MaxContextLength = 2048
	}

	return caps
}

func (p *OpenAICompatibleProfile) IsModelSupported(modelName string, registry domain.ModelRegistry) bool {
	// For OpenAI compatible endpoints, check registry if available
	// TODO: When registry supports GetModel, check if model exists
	return true // Optimistic - let the endpoint handle it
}

func (p *OpenAICompatibleProfile) TransformModelName(fromName string, toFormat string) string {
	// Most OpenAI compatible APIs use the same naming
	return fromName
}

func (p *OpenAICompatibleProfile) GetResourceRequirements(modelName string, registry domain.ModelRegistry) domain.ResourceRequirements {
	// Cloud endpoints don't have local resource requirements
	return domain.ResourceRequirements{
		MinMemoryGB:         0,
		RecommendedMemoryGB: 0,
		RequiresGPU:         false,
		MinGPUMemoryGB:      0,
		EstimatedLoadTimeMS: 0, // No load time for cloud APIs
	}
}

func (p *OpenAICompatibleProfile) GetOptimalConcurrency(modelName string) int {
	// Cloud providers can handle high concurrency
	// But we should be reasonable to avoid rate limits
	lowerName := strings.ToLower(modelName)

	if strings.Contains(lowerName, "gpt-4") {
		return 10 // More expensive, limit concurrency
	} else if strings.Contains(lowerName, "claude") {
		return 20
	}
	return 50 // Most models can handle higher concurrency
}

func (p *OpenAICompatibleProfile) GetRoutingStrategy() domain.RoutingStrategy {
	return domain.RoutingStrategy{
		PreferSameFamily:     false, // Cloud APIs typically have specific models
		AllowFallback:        false, // Don't fallback - users expect specific models
		MaxRetries:           3,
		PreferLocalEndpoints: false, // Cloud endpoints are not local
	}
}

func (p *OpenAICompatibleProfile) ShouldBatchRequests() bool {
	// Some cloud APIs support batching, but not universally
	return false
}

func (p *OpenAICompatibleProfile) GetRequestTimeout(modelName string) time.Duration {
	// Cloud APIs should respond quickly
	lowerName := strings.ToLower(modelName)

	if strings.Contains(lowerName, "gpt-4") || strings.Contains(lowerName, "claude") {
		return 60 * time.Second // Larger models might take longer
	}
	return 30 * time.Second
}
