package domain

// ProviderMetrics contains metrics extracted from provider responses
// Uses fixed-size types to minimise memory usage in high-frequency scenarios
type ProviderMetrics struct {

	// Provider-specific metadata
	Model        string `json:"model,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	// Token counts from provider
	InputTokens  int32 `json:"input_tokens,omitempty"`
	OutputTokens int32 `json:"output_tokens,omitempty"`
	TotalTokens  int32 `json:"total_tokens,omitempty"`

	// Timing metrics in milliseconds
	TTFTMs       int32 `json:"ttft_ms,omitempty"`       // Time to first token
	TotalMs      int32 `json:"total_ms,omitempty"`      // Total generation time
	ModelLoadMs  int32 `json:"model_load_ms,omitempty"` // Model loading time (cold starts)
	PromptMs     int32 `json:"prompt_ms,omitempty"`     // Prompt processing time
	GenerationMs int32 `json:"generation_ms,omitempty"` // Token generation time

	// Throughput metrics
	TokensPerSecond float32 `json:"tokens_per_sec,omitempty"`

	IsComplete bool `json:"is_complete,omitempty"`
}

// MetricsExtractionConfig defines how to extract metrics from provider responses
type MetricsExtractionConfig struct {

	// JSONPath expressions for extracting raw values
	Paths map[string]string `yaml:"paths"`

	// Simple math expressions for calculated metrics
	Calculations map[string]string `yaml:"calculations"`

	// Header names to extract values from
	Headers map[string]string `yaml:"headers,omitempty"`
	Source  string            `yaml:"source"` // "response_body" or "response_headers"
	Format  string            `yaml:"format"` // "json" for now

	Enabled bool `yaml:"enabled"`
}

// MetricsConfig wraps the extraction configuration
type MetricsConfig struct {
	Extraction MetricsExtractionConfig `yaml:"extraction"`
}
