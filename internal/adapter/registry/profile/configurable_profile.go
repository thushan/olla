package profile

import (
	"fmt"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util/pattern"
)

const (
	defaultModelsFieldPath = "data"
	ollamaResponseFormat   = "ollama"
	ollamaModelsFieldPath  = "models"
)

// ConfigurableProfile bridges YAML config to the PlatformProfile interface.
// Much easier for users to write YAML than implement Go interfaces.
type ConfigurableProfile struct {
	config *domain.ProfileConfig

	// each platform returns models in its own special snowflake format
	modelParser ModelResponseParser
}

func NewConfigurableProfile(config *domain.ProfileConfig) *ConfigurableProfile {
	return &ConfigurableProfile{
		config:      config,
		modelParser: getParserForFormat(config.Request.ResponseFormat),
	}
}

func (p *ConfigurableProfile) GetName() string {
	return p.config.Name
}

func (p *ConfigurableProfile) GetVersion() string {
	return p.config.Version
}

func (p *ConfigurableProfile) GetPaths() []string {
	return p.config.API.Paths
}

func (p *ConfigurableProfile) GetPath(index int) string {
	if index < 0 || index >= len(p.config.API.Paths) {
		return ""
	}
	return p.config.API.Paths[index]
}

func (p *ConfigurableProfile) GetModelDiscoveryURL(baseURL string) string {
	// avoid http://localhost//v1/models nonsense
	if baseURL != "" && baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}
	return baseURL + p.config.API.ModelDiscoveryPath
}

func (p *ConfigurableProfile) GetHealthCheckPath() string {
	return p.config.API.HealthCheckPath
}

func (p *ConfigurableProfile) IsOpenAPICompatible() bool {
	return p.config.API.OpenAICompatible
}

func (p *ConfigurableProfile) GetRequestParsingRules() domain.RequestParsingRules {
	rules := p.config.Request.ParsingRules
	return domain.RequestParsingRules{
		ChatCompletionsPath: rules.ChatCompletionsPath,
		CompletionsPath:     rules.CompletionsPath,
		GeneratePath:        rules.GeneratePath,
		ModelFieldName:      rules.ModelFieldName,
		SupportsStreaming:   rules.SupportsStreaming,
	}
}

func (p *ConfigurableProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	// Determine the models field path based on response format
	modelsFieldPath := defaultModelsFieldPath
	if p.config.Request.ResponseFormat == ollamaResponseFormat {
		modelsFieldPath = ollamaModelsFieldPath
	}

	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: modelsFieldPath,
	}
}

func (p *ConfigurableProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: p.config.Detection.UserAgentPatterns,
		ResponseHeaders:   p.config.Detection.Headers,
		PathIndicators:    p.config.Detection.PathIndicators,
	}
}

func (p *ConfigurableProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if p.modelParser == nil {
		return nil, fmt.Errorf("no model parser configured for format: %s", p.config.Request.ResponseFormat)
	}
	return p.modelParser.Parse(data)
}

func (p *ConfigurableProfile) GetTimeout() time.Duration {
	if p.config.Characteristics.Timeout == 0 {
		return 2 * time.Minute
	}
	return p.config.Characteristics.Timeout
}

func (p *ConfigurableProfile) GetMaxConcurrentRequests() int {
	if p.config.Characteristics.MaxConcurrentRequests == 0 {
		return 10
	}
	return p.config.Characteristics.MaxConcurrentRequests
}

func (p *ConfigurableProfile) GetDefaultPriority() int {
	if p.config.Characteristics.DefaultPriority == 0 {
		return 50
	}
	return p.config.Characteristics.DefaultPriority
}

func (p *ConfigurableProfile) GetConfig() *domain.ProfileConfig {
	return p.config
}

func (p *ConfigurableProfile) ValidateEndpoint(endpoint *domain.Endpoint) error {
	if endpoint.URL == nil {
		return fmt.Errorf("%s endpoint requires URL", p.config.Name)
	}

	// ollama defaults to http, but we need to be explicit for safety
	if endpoint.URL.Scheme == "" {
		return fmt.Errorf("%s endpoint URL must include scheme (http:// or https://)", p.config.Name)
	}

	return nil
}

// InferenceProfile implementation

func (p *ConfigurableProfile) GetModelCapabilities(modelName string, registry domain.ModelRegistry) domain.ModelCapabilities {
	caps := domain.ModelCapabilities{
		ChatCompletion:   true, // Default for most models
		TextGeneration:   true, // Default for most models
		StreamingSupport: p.config.Request.ParsingRules.SupportsStreaming,
		MaxContextLength: 4096,
		MaxOutputTokens:  2048,
	}

	// Check capability patterns from config

	// Check for embeddings capability
	if patterns, ok := p.config.Models.CapabilityPatterns["embeddings"]; ok {
		for _, pat := range patterns {
			if pattern.MatchesGlob(modelName, pat) {
				caps.Embeddings = true
				caps.ChatCompletion = false
				caps.TextGeneration = false
				break
			}
		}
	}

	// Check for vision capability
	if patterns, ok := p.config.Models.CapabilityPatterns["vision"]; ok {
		for _, pat := range patterns {
			if pattern.MatchesGlob(modelName, pat) {
				caps.VisionUnderstanding = true
				break
			}
		}
	}

	// Check for code capability
	if patterns, ok := p.config.Models.CapabilityPatterns["code"]; ok {
		for _, pat := range patterns {
			if pattern.MatchesGlob(modelName, pat) {
				caps.CodeGeneration = true
				break
			}
		}
	}

	// Function calling is typically supported by standard chat models
	// but not by specialized models (vision, code, embeddings)
	if !caps.Embeddings && !caps.VisionUnderstanding && !caps.CodeGeneration {
		caps.FunctionCalling = true
	}

	// Check context patterns from config
	for _, contextPattern := range p.config.Models.ContextPatterns {
		if pattern.MatchesGlob(modelName, contextPattern.Pattern) {
			caps.MaxContextLength = contextPattern.Context
			break
		}
	}

	return caps
}

func (p *ConfigurableProfile) IsModelSupported(modelName string, registry domain.ModelRegistry) bool {
	// Configurable profiles are optimistic
	return true
}

func (p *ConfigurableProfile) TransformModelName(fromName string, toFormat string) string {
	// No transformation by default
	return fromName
}

func (p *ConfigurableProfile) GetResourceRequirements(modelName string, registry domain.ModelRegistry) domain.ResourceRequirements {
	// Check if we have resource patterns configured
	if p.config.Resources.ModelSizes == nil && p.config.Resources.Defaults.MinMemoryGB == 0 {
		// No resource config, assume cloud/remote
		return domain.ResourceRequirements{
			MinMemoryGB:         0,
			RecommendedMemoryGB: 0,
			RequiresGPU:         false,
			MinGPUMemoryGB:      0,
			EstimatedLoadTimeMS: 0,
		}
	}

	lowerName := strings.ToLower(modelName)

	// Find matching model size pattern
	var baseReqs *domain.ResourceRequirements
	for _, pattern := range p.config.Resources.ModelSizes {
		for _, pat := range pattern.Patterns {
			if strings.Contains(lowerName, pat) {
				baseReqs = &domain.ResourceRequirements{
					MinMemoryGB:         pattern.MinMemoryGB,
					RecommendedMemoryGB: pattern.RecommendedMemoryGB,
					MinGPUMemoryGB:      pattern.MinGPUMemoryGB,
					RequiresGPU:         p.config.Resources.Defaults.RequiresGPU,
					EstimatedLoadTimeMS: int64(pattern.EstimatedLoadTimeMS),
				}
				break
			}
		}
		if baseReqs != nil {
			break
		}
	}

	// Use defaults if no pattern matched
	if baseReqs == nil {
		baseReqs = &p.config.Resources.Defaults
	}

	// Apply quantization multipliers if configured
	if p.config.Resources.Quantization.Multipliers != nil {
		for quantType, multiplier := range p.config.Resources.Quantization.Multipliers {
			if strings.Contains(lowerName, quantType) {
				baseReqs.MinMemoryGB *= multiplier
				baseReqs.RecommendedMemoryGB *= multiplier
				baseReqs.MinGPUMemoryGB *= multiplier
				break
			}
		}
	}

	return *baseReqs
}

func (p *ConfigurableProfile) GetOptimalConcurrency(modelName string) int {
	// Check if we have concurrency limits configured
	if len(p.config.Resources.ConcurrencyLimits) == 0 {
		return p.GetMaxConcurrentRequests()
	}

	// Get resource requirements for this model
	reqs := p.GetResourceRequirements(modelName, nil)

	// Find the appropriate concurrency limit based on memory requirements
	// Iterate from most restrictive to least restrictive
	for _, limit := range p.config.Resources.ConcurrencyLimits {
		if reqs.MinMemoryGB >= limit.MinMemoryGB {
			return limit.MaxConcurrent
		}
	}

	// Default to max concurrent requests if no limits match
	return p.GetMaxConcurrentRequests()
}

func (p *ConfigurableProfile) GetRoutingStrategy() domain.RoutingStrategy {
	return domain.RoutingStrategy{
		PreferSameFamily:     false,
		AllowFallback:        false,
		MaxRetries:           3,
		PreferLocalEndpoints: false, // TODO: Add IsLocal to config
	}
}

func (p *ConfigurableProfile) ShouldBatchRequests() bool {
	return false
}

func (p *ConfigurableProfile) GetRequestTimeout(modelName string) time.Duration {
	baseTimeout := p.GetTimeout()

	// Check if timeout scaling is configured
	if p.config.Resources.TimeoutScaling.BaseTimeoutSeconds > 0 {
		baseTimeout = time.Duration(p.config.Resources.TimeoutScaling.BaseTimeoutSeconds) * time.Second
	}

	// Add load time buffer if configured
	if p.config.Resources.TimeoutScaling.LoadTimeBuffer {
		reqs := p.GetResourceRequirements(modelName, nil)
		if reqs.EstimatedLoadTimeMS > 0 {
			loadBuffer := time.Duration(reqs.EstimatedLoadTimeMS) * time.Millisecond
			baseTimeout += loadBuffer
		}
	}

	return baseTimeout
}
