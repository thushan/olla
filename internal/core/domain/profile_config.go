package domain

import "time"

// ProfileConfig is loaded from YAML files so users can add support for
// new inference platforms without touching Go code. Much easier than
// submitting PRs for every new LLM server that pops up.
type ProfileConfig struct {
	Models struct {
		CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
		NameFormat         string              `yaml:"name_format"`
		ContextPatterns    []ContextPattern    `yaml:"context_patterns"`
	} `yaml:"models"`

	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	DisplayName string `yaml:"display_name"`
	Description string `yaml:"description"`

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

	API struct {
		ModelDiscoveryPath string   `yaml:"model_discovery_path"`
		HealthCheckPath    string   `yaml:"health_check_path"`
		Paths              []string `yaml:"paths"`
		OpenAICompatible   bool     `yaml:"openai_compatible"`
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
