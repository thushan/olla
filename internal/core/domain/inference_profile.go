package domain

import "time"

// InferenceProfile extends PlatformProfile with model-aware capabilities.
// We embed PlatformProfile to avoid breaking thousands of lines of existing code
// while laying the groundwork for smarter routing decisions.
type InferenceProfile interface {
	PlatformProfile

	// Ollama can take 5+ minutes to load a 70B model from disk,
	// while OpenAI typically responds in seconds. This matters for
	// circuit breakers and user experience.
	GetTimeout() time.Duration

	// LM Studio chokes on concurrent requests (it's single-threaded),
	// but cloud providers can handle hundreds. We need to respect these limits
	// to avoid overwhelming local instances.
	GetMaxConcurrentRequests() int

	// Some platforms have quirks - like requiring specific headers or
	// API key formats. This gives each profile a chance to validate
	// before we waste time on doomed requests.
	ValidateEndpoint(endpoint *Endpoint) error

	// Local instances should generally be preferred over cloud endpoints
	// (lower latency, no egress costs). This provides sensible defaults
	// when users don't explicitly configure priorities.
	GetDefaultPriority() int

	// Escape hatch for platform-specific settings that don't deserve
	// their own interface method. Prevents interface bloat.
	GetConfig() *ProfileConfig

	// Model Capabilities
	GetModelCapabilities(modelName string, registry ModelRegistry) ModelCapabilities
	IsModelSupported(modelName string, registry ModelRegistry) bool
	TransformModelName(fromName string, toFormat string) string

	// Resource Requirements
	GetResourceRequirements(modelName string, registry ModelRegistry) ResourceRequirements
	GetOptimalConcurrency(modelName string) int

	// Routing Hints
	GetRoutingStrategy() RoutingStrategy
	ShouldBatchRequests() bool
	GetRequestTimeout(modelName string) time.Duration
}

// ModelCapabilities describes what a model can do.
// This helps us avoid sending vision requests to text-only models,
// or function calls to models that don't support them.
type ModelCapabilities struct {
	ChatCompletion      bool
	TextGeneration      bool
	Embeddings          bool
	VisionUnderstanding bool
	CodeGeneration      bool
	FunctionCalling     bool
	StreamingSupport    bool
	MaxContextLength    int64
	MaxOutputTokens     int64
}

// ResourceRequirements tells us what hardware a model needs.
// A 70B model on Ollama needs ~40GB RAM, while GPT-4 needs
// nothing local. This prevents us from routing big models
// to tiny servers.
type ResourceRequirements struct {
	MinMemoryGB         float64
	RecommendedMemoryGB float64
	RequiresGPU         bool
	MinGPUMemoryGB      float64
	EstimatedLoadTimeMS int64
}

// RoutingStrategy guides how we select endpoints.
// Some platforms work better with certain strategies -
// like preferring local Ollama instances over cloud.
type RoutingStrategy struct {
	MaxRetries           int
	PreferSameFamily     bool // Route llama requests to other llama models if exact match unavailable
	AllowFallback        bool // Allow routing to compatible models
	PreferLocalEndpoints bool // Prioritize local over cloud
}
