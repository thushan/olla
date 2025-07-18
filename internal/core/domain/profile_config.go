package domain

import "time"

// ProfileConfig is loaded from YAML files so users can add support for
// new inference platforms without touching Go code. Much easier than
// submitting PRs for every new LLM server that pops up.
type ProfileConfig struct {
	Models struct {
		CapabilityPatterns map[string][]string `yaml:"capability_patterns"`
		NameFormat         string              `yaml:"name_format"`
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
