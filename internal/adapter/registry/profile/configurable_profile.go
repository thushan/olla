package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
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
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "data", // openai convention that most follow
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
	// Default capabilities for configurable profiles
	return domain.ModelCapabilities{
		ChatCompletion:   true,
		TextGeneration:   true,
		StreamingSupport: p.config.Request.ParsingRules.SupportsStreaming,
		MaxContextLength: 4096,
		MaxOutputTokens:  2048,
	}
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
	// Assume cloud/remote by default
	return domain.ResourceRequirements{
		MinMemoryGB:         0,
		RecommendedMemoryGB: 0,
		RequiresGPU:         false,
		MinGPUMemoryGB:      0,
		EstimatedLoadTimeMS: 0,
	}
}

func (p *ConfigurableProfile) GetOptimalConcurrency(modelName string) int {
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
	return p.GetTimeout()
}
