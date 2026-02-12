package domain

import "time"

// ProfileConfig is loaded from YAML files so users can add support for
// new inference platforms without touching Go code. Much easier than
// submitting PRs for every new LLM server that pops up.
type ProfileConfig struct {

	// Metrics extraction configuration for provider responses
	Metrics     MetricsConfig `yaml:"metrics,omitempty"`
	Name        string        `yaml:"name"`
	Version     string        `yaml:"version"`
	DisplayName string        `yaml:"display_name"`
	Description string        `yaml:"description"`

	Detection struct {
		Headers           []string `yaml:"headers"`
		UserAgentPatterns []string `yaml:"user_agent_patterns"`
		ResponsePatterns  []string `yaml:"response_patterns"`
		PathIndicators    []string `yaml:"path_indicators"`
		DefaultPorts      []int    `yaml:"default_ports"`
	} `yaml:"detection"`

	Request struct {
		ModelFieldPaths []string `yaml:"model_field_paths"`
		ResponseFormat  string   `yaml:"response_format"`
		ParsingRules    struct {
			ChatCompletionsPath string `yaml:"chat_completions_path"`
			CompletionsPath     string `yaml:"completions_path"`
			GeneratePath        string `yaml:"generate_path"`
			ModelFieldName      string `yaml:"model_field_name"`
			SupportsStreaming   bool   `yaml:"supports_streaming"`
		} `yaml:"parsing_rules"`
	} `yaml:"request"`

	Models struct {
		CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
		NameFormat         string              `yaml:"name_format"`
		ContextPatterns    []ContextPattern    `yaml:"context_patterns"`
	} `yaml:"models"`

	// Routing configuration for URL prefix mapping
	Routing struct {
		Prefixes []string `yaml:"prefixes"`
	} `yaml:"routing"`

	API struct {
		// AnthropicSupport declares whether this backend natively speaks the
		// Anthropic Messages API. When present and enabled, the translator layer
		// can skip the Anthropic-to-OpenAI conversion and forward requests directly.
		// Nil means the backend has no native Anthropic support (the common case).
		AnthropicSupport   *AnthropicSupportConfig `yaml:"anthropic_support,omitempty"`
		ModelDiscoveryPath string                  `yaml:"model_discovery_path"`
		HealthCheckPath    string                  `yaml:"health_check_path"`
		Paths              []string                `yaml:"paths"`
		OpenAICompatible   bool                    `yaml:"openai_compatible"`
	} `yaml:"api"`

	Resources struct {
		Quantization struct {
			Multipliers map[string]float64 `yaml:"multipliers"`
		} `yaml:"quantization"`
		ModelSizes        []ModelSizePattern        `yaml:"model_sizes"`
		ConcurrencyLimits []ConcurrencyLimitPattern `yaml:"concurrency_limits"`
		Defaults          ResourceRequirements      `yaml:"defaults"`
		TimeoutScaling    TimeoutScaling            `yaml:"timeout_scaling"`
	} `yaml:"resources"`

	// PathIndices allows configuring which paths serve specific purposes
	PathIndices struct {
		Health          int `yaml:"health"`
		Models          int `yaml:"models"`
		Completions     int `yaml:"completions"`
		ChatCompletions int `yaml:"chat_completions"`
		Embeddings      int `yaml:"embeddings"`
	} `yaml:"path_indices"`

	Characteristics struct {
		Timeout               time.Duration `yaml:"timeout"`
		MaxConcurrentRequests int           `yaml:"max_concurrent_requests"`
		DefaultPriority       int           `yaml:"default_priority"`
		StreamingSupport      bool          `yaml:"streaming_support"`
	} `yaml:"characteristics"`
}

// ModelSizePattern defines resource requirements for models matching specific patterns
type ModelSizePattern struct {
	Patterns            []string `yaml:"patterns"`
	MinMemoryGB         float64  `yaml:"min_memory_gb"`
	RecommendedMemoryGB float64  `yaml:"recommended_memory_gb"`
	MinGPUMemoryGB      float64  `yaml:"min_gpu_memory_gb"`
	EstimatedLoadTimeMS int      `yaml:"estimated_load_time_ms"`
}

// ConcurrencyLimitPattern defines how many concurrent requests a model can handle based on its memory requirements
type ConcurrencyLimitPattern struct {
	MinMemoryGB   float64 `yaml:"min_memory_gb"`
	MaxConcurrent int     `yaml:"max_concurrent"`
}

// TimeoutScaling configures dynamic timeout adjustment based on model characteristics
type TimeoutScaling struct {
	BaseTimeoutSeconds int  `yaml:"base_timeout_seconds"`
	LoadTimeBuffer     bool `yaml:"load_time_buffer"` // adds estimated_load_time_ms to timeout
}

// ContextPattern maps model name patterns to context window sizes
type ContextPattern struct {
	Pattern string `yaml:"pattern"`
	Context int64  `yaml:"context"`
}

// AnthropicSupportConfig declares native Anthropic Messages API support for a
// backend platform. This enables the passthrough optimisation: when a backend
// natively understands the Anthropic wire format, requests can be forwarded
// directly without the costly Anthropic-to-OpenAI-and-back translation.
//
// Example YAML (in a profile's api section):
//
//	api:
//	  anthropic_support:
//	    enabled: true
//	    messages_path: "/v1/messages"
//	    token_count: true
//	    min_version: "2023-06-01"
//	    limitations:
//	      - "no_extended_thinking"
//	      - "max_tokens_4096"
type AnthropicSupportConfig struct {
	// MessagesPath is the backend path that accepts Anthropic Messages API
	// requests (e.g. "/v1/messages"). Required when Enabled is true.
	MessagesPath string `yaml:"messages_path"`

	// MinVersion is the minimum anthropic-version header value the backend
	// requires. If the incoming request specifies an older version, the
	// translator falls back to the translation path. Use the standard
	// Anthropic version date format (e.g. "2023-06-01").
	MinVersion string `yaml:"min_version,omitempty"`

	// Limitations lists Anthropic features this backend does NOT support.
	// Used by CanPassthrough to decide whether a particular request can be
	// sent directly or must go through translation instead.
	// Common values: "no_extended_thinking", "no_tool_use", "no_vision",
	// "max_tokens_4096".
	Limitations []string `yaml:"limitations,omitempty"`

	// Enabled controls whether passthrough is active for this backend.
	// Defaults to false so existing profiles remain unaffected.
	Enabled bool `yaml:"enabled"`

	// TokenCount indicates the backend supports the Anthropic token counting
	// endpoint. When true, token count requests can also be passed through.
	TokenCount bool `yaml:"token_count,omitempty"`
}

// HasLimitation reports whether the backend declares a specific limitation.
// Callers use this to check whether a request feature (e.g. extended thinking)
// is unsupported before attempting passthrough.
func (c *AnthropicSupportConfig) HasLimitation(limitation string) bool {
	if c == nil {
		return false
	}
	for _, l := range c.Limitations {
		if l == limitation {
			return true
		}
	}
	return false
}

// SupportsPassthrough is a convenience check that the config is non-nil and
// explicitly enabled. Safe to call on a nil receiver.
func (c *AnthropicSupportConfig) SupportsPassthrough() bool {
	return c != nil && c.Enabled
}
