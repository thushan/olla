package profile

// LMStudioProfile handles LM Studio's beta API which gives us way more
// model metadata than the OpenAI endpoints. Their /api/v0/models endpoint
// tells us quantization levels, architecture, and whether models are loaded
// into memory - pretty handy for smart routing decisions.
import (
	"fmt"
	"strings"
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
		// LM Studio's beta API gives us the good stuff like memory usage and load state
		"/api/v0/models",
		"/api/v0/chat/completions",
		"/api/v0/completions",
		"/api/v0/embeddings",

		// standard OpenAI endpoints for when apps don't know about LM Studio
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

// InferenceProfile implementation

func (p *LMStudioProfile) GetTimeout() time.Duration {
	// LM Studio runs locally, so models are already loaded
	return 2 * time.Minute
}

func (p *LMStudioProfile) GetMaxConcurrentRequests() int {
	// LM Studio is single-threaded - this is critical!
	return 1
}

func (p *LMStudioProfile) ValidateEndpoint(endpoint *domain.Endpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint cannot be nil")
	}
	if endpoint.URL == nil {
		return fmt.Errorf("endpoint URL cannot be nil")
	}
	return nil
}

func (p *LMStudioProfile) GetDefaultPriority() int {
	// Local LM Studio should be highly preferred
	return 1
}

func (p *LMStudioProfile) GetConfig() *domain.ProfileConfig {
	return &domain.ProfileConfig{
		Name:        "lm_studio",
		Version:     LMStudioProfileVersion,
		DisplayName: "LM Studio",
		Description: "LM Studio local inference server",
		Models: struct {
			CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
			NameFormat         string              `yaml:"name_format"`
		}{
			CapabilityPatterns: map[string][]string{
				"chat":       {"*"},
				"completion": {"*"},
				"embedding":  {"*embed*", "bge-*", "e5-*"},
				"vision":     {"*vision*", "llava*", "bakllava*", "cogvlm*"},
				"code":       {"*code*", "codellama*", "deepseek-coder*", "starcoder*"},
			},
			NameFormat: "{{.Publisher}}/{{.Name}}",
		},
	}
}

func (p *LMStudioProfile) GetModelCapabilities(modelName string, registry domain.ModelRegistry) domain.ModelCapabilities {
	caps := domain.ModelCapabilities{
		ChatCompletion:   true,
		TextGeneration:   true,
		StreamingSupport: true,
		MaxContextLength: 4096, // Conservative default
		MaxOutputTokens:  2048,
	}

	lowerName := strings.ToLower(modelName)

	// Check for embeddings models
	if strings.Contains(lowerName, "embed") ||
		strings.Contains(lowerName, "bge-") ||
		strings.Contains(lowerName, "e5-") {
		caps.Embeddings = true
		caps.ChatCompletion = false
		caps.TextGeneration = false
	}

	// Check for vision models
	if strings.Contains(lowerName, "vision") ||
		strings.Contains(lowerName, "llava") ||
		strings.Contains(lowerName, "cogvlm") {
		caps.VisionUnderstanding = true
	}

	// Check for code models
	if strings.Contains(lowerName, "code") ||
		strings.Contains(lowerName, "starcoder") {
		caps.CodeGeneration = true
	}

	// Get context length from model name if possible
	// TODO: When registry supports GetModel, use it to get actual context length

	// LM Studio models generally support function calling if they're chat models
	if caps.ChatCompletion && !caps.Embeddings {
		caps.FunctionCalling = true
	}

	return caps
}

func (p *LMStudioProfile) IsModelSupported(modelName string, registry domain.ModelRegistry) bool {
	// LM Studio only supports models that are loaded
	// TODO: When registry supports GetModel, check if model state is "loaded"
	return true // Optimistic for now
}

func (p *LMStudioProfile) TransformModelName(fromName string, toFormat string) string {
	// LM Studio uses publisher/model format
	if toFormat == "lm_studio" && !strings.Contains(fromName, "/") {
		// Try to infer publisher from common patterns
		lowerName := strings.ToLower(fromName)
		switch {
		case strings.HasPrefix(lowerName, "llama"):
			return "meta-llama/" + fromName
		case strings.HasPrefix(lowerName, "mistral"):
			return "mistralai/" + fromName
		case strings.HasPrefix(lowerName, "phi"):
			return "microsoft/" + fromName
		}
	}
	return fromName
}

func (p *LMStudioProfile) GetResourceRequirements(modelName string, registry domain.ModelRegistry) domain.ResourceRequirements {
	// TODO: When registry supports GetModel, use actual quantization info
	// Fall back to name-based estimation
	return p.estimateFromName(modelName)
}

func (p *LMStudioProfile) estimateBaseMemory(modelName string) float64 {
	lowerName := strings.ToLower(modelName)

	switch {
	case strings.Contains(lowerName, "70b") || strings.Contains(lowerName, "72b"):
		return 70
	case strings.Contains(lowerName, "65b"):
		return 65
	case strings.Contains(lowerName, "34b") || strings.Contains(lowerName, "33b"):
		return 34
	case strings.Contains(lowerName, "13b") || strings.Contains(lowerName, "14b"):
		return 14
	case strings.Contains(lowerName, "7b") || strings.Contains(lowerName, "8b"):
		return 8
	case strings.Contains(lowerName, "3b"):
		return 3
	default:
		return 7 // Default to 7B
	}
}

func (p *LMStudioProfile) estimateFromName(modelName string) domain.ResourceRequirements {
	baseMem := p.estimateBaseMemory(modelName)
	return domain.ResourceRequirements{
		MinMemoryGB:         baseMem * 0.6, // Assume some quantization
		RecommendedMemoryGB: baseMem * 0.75,
		MinGPUMemoryGB:      baseMem * 0.6,
		RequiresGPU:         false,
		EstimatedLoadTimeMS: 1000, // LM Studio keeps models loaded
	}
}

func (p *LMStudioProfile) GetOptimalConcurrency(modelName string) int {
	// LM Studio is single-threaded!
	return 1
}

func (p *LMStudioProfile) GetRoutingStrategy() domain.RoutingStrategy {
	return domain.RoutingStrategy{
		PreferSameFamily:     true, // Route to similar models if exact match unavailable
		AllowFallback:        true, // Allow compatible models
		MaxRetries:           2,    // Limited retries due to single-threading
		PreferLocalEndpoints: true, // LM Studio is always local
	}
}

func (p *LMStudioProfile) ShouldBatchRequests() bool {
	// Cannot batch due to single-threading
	return false
}

func (p *LMStudioProfile) GetRequestTimeout(modelName string) time.Duration {
	// LM Studio models are pre-loaded, so shorter timeout is fine
	return 60 * time.Second
}
