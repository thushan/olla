package profile

import (
	"fmt"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type OllamaResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Size        *int64         `json:"size,omitempty"`
	Digest      *string        `json:"digest,omitempty"`
	ModifiedAt  *string        `json:"modified_at,omitempty"`
	Description *string        `json:"description,omitempty"`
	Details     *OllamaDetails `json:"details,omitempty"`
	Name        string         `json:"name"`
}

type OllamaDetails struct {
	ParameterSize     *string  `json:"parameter_size,omitempty"`
	QuantizationLevel *string  `json:"quantization_level,omitempty"`
	Family            *string  `json:"family,omitempty"`
	Format            *string  `json:"format,omitempty"`
	ParentModel       *string  `json:"parent_model,omitempty"`
	Families          []string `json:"families,omitempty"`
}

type OllamaProfile struct{}

const (
	OllamaProfileVersion           = "1.0"
	OllamaProfileHealthPathIndex   = 0
	OllamaCompletionsPathIndex     = 1
	OllamaChatCompletionsPathIndex = 2
	OllamaEmbeddingsPathIndex      = 3
	OllamaModelsPathIndex          = 4
)

var ollamaPaths []string

func init() {
	ollamaPaths = []string{
		"/", // ollama returns "Ollama is running" here
		"/api/generate",
		"/api/chat",
		"/api/embeddings",
		"/api/tags", // where ollama lists its models
		"/api/show", // detailed model info including modelfile

		// openai endpoints because everyone expects them
		"/v1/models",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",

		/*
			// Not sure we want to support these yet, but they are
			// documented in the Ollama API docs.
				// Model management
				"/api/create", // Create model from Modelfile
				"/api/pull",   // Download model
				"/api/push",   // Upload model
				"/api/copy",   // Copy model
				"/api/delete", // Delete model

				// Blob management
				"/api/blobs/:digest", // Check blob exists
				"/api/blobs",         // Create blob

				// Process management
				"/api/ps", // List running models

		*/
		// skipping model management endpoints like /api/pull and /api/create
		// because we're a proxy, not ollama's package manager

	}
}

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
	return util.NormaliseBaseURL(baseURL) + ollamaPaths[OllamaModelsPathIndex]
}

func (p *OllamaProfile) GetHealthCheckPath() string {
	return ollamaPaths[OllamaProfileHealthPathIndex]
}

func (p *OllamaProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OllamaProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: ollamaPaths[OllamaChatCompletionsPathIndex],
		CompletionsPath:     ollamaPaths[OllamaCompletionsPathIndex],
		GeneratePath:        ollamaPaths[OllamaCompletionsPathIndex],
		ModelFieldName:      "model",
		SupportsStreaming:   true,
	}
}

func (p *OllamaProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "models",
	}
}

func (p *OllamaProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"ollama/"},
		ResponseHeaders:   []string{"X-ProfileOllama-Version"},
		PathIndicators: []string{
			ollamaPaths[OllamaProfileHealthPathIndex],
			ollamaPaths[OllamaModelsPathIndex],
		},
	}
}

func (p *OllamaProfile) GetPaths() []string {
	return ollamaPaths
}

func (p *OllamaProfile) GetPath(index int) string {
	if index < 0 || index >= len(ollamaPaths) {
		return ""
	}
	return ollamaPaths[index]
}
func (p *OllamaProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OllamaResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Models))
	now := time.Now()

	for _, ollamaModel := range response.Models {
		if ollamaModel.Name == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     ollamaModel.Name,
			LastSeen: now,
		}

		if ollamaModel.Size != nil {
			modelInfo.Size = *ollamaModel.Size
		}

		if ollamaModel.Description != nil {
			modelInfo.Description = *ollamaModel.Description
		}

		if ollamaModel.Details != nil || ollamaModel.Digest != nil || ollamaModel.ModifiedAt != nil {
			modelInfo.Details = createModelDetails(ollamaModel)
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

func createModelDetails(ollamaModel OllamaModel) *domain.ModelDetails {
	details := &domain.ModelDetails{}

	if ollamaModel.Digest != nil {
		details.Digest = ollamaModel.Digest
	}

	if ollamaModel.ModifiedAt != nil {
		if parsedTime := util.ParseTime(*ollamaModel.ModifiedAt); parsedTime != nil {
			details.ModifiedAt = parsedTime
		}
	}

	if ollamaModel.Details != nil {
		if ollamaModel.Details.ParameterSize != nil {
			details.ParameterSize = ollamaModel.Details.ParameterSize
		}
		if ollamaModel.Details.QuantizationLevel != nil {
			details.QuantizationLevel = ollamaModel.Details.QuantizationLevel
		}
		if ollamaModel.Details.Family != nil {
			details.Family = ollamaModel.Details.Family
		}
		if ollamaModel.Details.Format != nil {
			details.Format = ollamaModel.Details.Format
		}
		if ollamaModel.Details.ParentModel != nil {
			details.ParentModel = ollamaModel.Details.ParentModel
		}
		if len(ollamaModel.Details.Families) > 0 {
			details.Families = ollamaModel.Details.Families
		}
	}
	return details
}

// InferenceProfile implementation

func (p *OllamaProfile) GetTimeout() time.Duration {
	// Ollama can take 5+ minutes to load large models from disk
	return 5 * time.Minute
}

func (p *OllamaProfile) GetMaxConcurrentRequests() int {
	// Ollama handles concurrency well, but let's be conservative
	return 10
}

func (p *OllamaProfile) ValidateEndpoint(endpoint *domain.Endpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint cannot be nil")
	}
	if endpoint.URL == nil {
		return fmt.Errorf("endpoint URL cannot be nil")
	}
	return nil
}

func (p *OllamaProfile) GetDefaultPriority() int {
	// Local Ollama instances should be preferred
	return 1
}

func (p *OllamaProfile) GetConfig() *domain.ProfileConfig {
	return &domain.ProfileConfig{
		Name:        "ollama",
		Version:     OllamaProfileVersion,
		DisplayName: "Ollama",
		Description: "Local and remote Ollama instances",
		Models: struct {
			CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
			NameFormat         string              `yaml:"name_format"`
		}{
			CapabilityPatterns: map[string][]string{
				"chat":       {"*"},
				"completion": {"*"},
				"embedding":  {"*embedding*", "*embed*"},
				"vision":     {"*vision*", "llava*", "bakllava*"},
				"code":       {"*code*", "codellama*", "deepseek-coder*", "phind-codellama*"},
			},
			NameFormat: "{{.Name}}",
		},
	}
}

func (p *OllamaProfile) GetModelCapabilities(modelName string, registry domain.ModelRegistry) domain.ModelCapabilities {
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
		strings.Contains(lowerName, "embedding") {
		caps.Embeddings = true
		caps.ChatCompletion = false
		caps.TextGeneration = false
	}

	// Check for vision models
	if strings.Contains(lowerName, "vision") ||
		strings.HasPrefix(lowerName, "llava") ||
		strings.HasPrefix(lowerName, "bakllava") {
		caps.VisionUnderstanding = true
	}

	// Check for code models
	if strings.Contains(lowerName, "code") ||
		strings.HasPrefix(lowerName, "codellama") ||
		strings.HasPrefix(lowerName, "deepseek-coder") {
		caps.CodeGeneration = true
	}

	// Adjust context length for known models
	switch {
	case strings.Contains(modelName, "128k"):
		caps.MaxContextLength = 128000
	case strings.Contains(modelName, "100k"):
		caps.MaxContextLength = 100000
	case strings.Contains(modelName, "32k"):
		caps.MaxContextLength = 32768
	case strings.Contains(modelName, "16k"):
		caps.MaxContextLength = 16384
	case strings.Contains(modelName, "8k") || strings.Contains(modelName, ":8b"):
		caps.MaxContextLength = 8192
	}

	// Some models support function calling
	if strings.Contains(lowerName, "mistral") ||
		strings.Contains(lowerName, "mixtral") ||
		strings.Contains(lowerName, "llama3") {
		caps.FunctionCalling = true
	}

	return caps
}

func (p *OllamaProfile) IsModelSupported(modelName string, registry domain.ModelRegistry) bool {
	// Ollama supports any model that's been pulled
	// TODO: When registry supports GetModel, check if model exists
	return true // Optimistic for now
}

func (p *OllamaProfile) TransformModelName(fromName string, toFormat string) string {
	// Ollama uses simple model names
	return fromName
}

func (p *OllamaProfile) GetResourceRequirements(modelName string, registry domain.ModelRegistry) domain.ResourceRequirements {
	reqs := domain.ResourceRequirements{
		RequiresGPU:         false, // CPU fallback is always available
		EstimatedLoadTimeMS: 5000,  // 5s default
	}

	// Extract parameter size from model name
	lowerName := strings.ToLower(modelName)

	// Estimate based on parameter count
	switch {
	case strings.Contains(lowerName, "70b") || strings.Contains(lowerName, "72b"):
		reqs.MinMemoryGB = 40
		reqs.RecommendedMemoryGB = 48
		reqs.MinGPUMemoryGB = 40
		reqs.EstimatedLoadTimeMS = 300000 // 5 minutes
	case strings.Contains(lowerName, "65b"):
		reqs.MinMemoryGB = 35
		reqs.RecommendedMemoryGB = 40
		reqs.MinGPUMemoryGB = 35
		reqs.EstimatedLoadTimeMS = 240000 // 4 minutes
	case strings.Contains(lowerName, "34b") || strings.Contains(lowerName, "33b") || strings.Contains(lowerName, "30b"):
		reqs.MinMemoryGB = 20
		reqs.RecommendedMemoryGB = 24
		reqs.MinGPUMemoryGB = 20
		reqs.EstimatedLoadTimeMS = 120000 // 2 minutes
	case strings.Contains(lowerName, "13b") || strings.Contains(lowerName, "14b"):
		reqs.MinMemoryGB = 10
		reqs.RecommendedMemoryGB = 16
		reqs.MinGPUMemoryGB = 10
		reqs.EstimatedLoadTimeMS = 60000 // 1 minute
	case strings.Contains(lowerName, "7b") || strings.Contains(lowerName, "8b"):
		reqs.MinMemoryGB = 6
		reqs.RecommendedMemoryGB = 8
		reqs.MinGPUMemoryGB = 6
		reqs.EstimatedLoadTimeMS = 30000 // 30s
	case strings.Contains(lowerName, "3b"):
		reqs.MinMemoryGB = 3
		reqs.RecommendedMemoryGB = 4
		reqs.MinGPUMemoryGB = 3
		reqs.EstimatedLoadTimeMS = 15000 // 15s
	case strings.Contains(lowerName, "1b") || strings.Contains(lowerName, "1.5b"):
		reqs.MinMemoryGB = 2
		reqs.RecommendedMemoryGB = 3
		reqs.MinGPUMemoryGB = 2
		reqs.EstimatedLoadTimeMS = 10000 // 10s
	default:
		// Default for unknown models
		reqs.MinMemoryGB = 4
		reqs.RecommendedMemoryGB = 8
		reqs.MinGPUMemoryGB = 4
	}

	// Adjust for quantization levels
	switch {
	case strings.Contains(lowerName, "q4"):
		reqs.MinMemoryGB *= 0.5
		reqs.RecommendedMemoryGB *= 0.5
		reqs.MinGPUMemoryGB *= 0.5
	case strings.Contains(lowerName, "q5"):
		reqs.MinMemoryGB *= 0.625
		reqs.RecommendedMemoryGB *= 0.625
		reqs.MinGPUMemoryGB *= 0.625
	case strings.Contains(lowerName, "q6"):
		reqs.MinMemoryGB *= 0.75
		reqs.RecommendedMemoryGB *= 0.75
		reqs.MinGPUMemoryGB *= 0.75
	case strings.Contains(lowerName, "q8"):
		reqs.MinMemoryGB *= 0.875
		reqs.RecommendedMemoryGB *= 0.875
		reqs.MinGPUMemoryGB *= 0.875
	}

	return reqs
}

func (p *OllamaProfile) GetOptimalConcurrency(modelName string) int {
	// Ollama can handle multiple requests, but large models benefit from limiting concurrency
	reqs := p.GetResourceRequirements(modelName, nil)

	switch {
	case reqs.MinMemoryGB > 30:
		return 1 // Very large models should process one at a time
	case reqs.MinMemoryGB > 15:
		return 2
	case reqs.MinMemoryGB > 8:
		return 4
	default:
		return 8 // Small models can handle more concurrent requests
	}
}

func (p *OllamaProfile) GetRoutingStrategy() domain.RoutingStrategy {
	return domain.RoutingStrategy{
		PreferSameFamily:     true, // If llama3 isn't available, try other llama models
		AllowFallback:        true, // Allow routing to compatible models
		MaxRetries:           3,
		PreferLocalEndpoints: true, // Ollama is typically local
	}
}

func (p *OllamaProfile) ShouldBatchRequests() bool {
	// Ollama doesn't have native batching support
	return false
}

func (p *OllamaProfile) GetRequestTimeout(modelName string) time.Duration {
	// Adjust timeout based on model size
	reqs := p.GetResourceRequirements(modelName, nil)

	// Base timeout of 30s, plus extra time based on model size
	baseTimeout := 30 * time.Second
	loadBuffer := time.Duration(reqs.EstimatedLoadTimeMS) * time.Millisecond

	return baseTimeout + loadBuffer
}
