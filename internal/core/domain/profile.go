package domain

const (
	ProfileOllama           = "ollama"
	ProfileLemonade         = "lemonade"
	ProfileLmStudio         = "lm-studio"
	ProfileSGLang           = "sglang"
	ProfileVLLM             = "vllm"
	ProfileOpenAICompatible = "openai-compatible"
	ProfileAuto             = "auto"
)

// PlatformProfile defines how to interact with a specific AI inference platform
type PlatformProfile interface {
	GetName() string
	GetVersion() string

	// GetModelDiscoveryURL returns the URL path for discovering available models
	GetModelDiscoveryURL(baseURL string) string
	// GetHealthCheckPath returns the preferred health check path for this platform
	GetHealthCheckPath() string
	// IsOpenAPICompatible indicates if this platform supports OpenIA-compatible endpoints
	IsOpenAPICompatible() bool
	// GetRequestParsingRules returns rules for extracting model names from requests
	GetRequestParsingRules() RequestParsingRules
	// GetModelResponseFormat returns the expected json response structure for model discovery
	GetModelResponseFormat() ModelResponseFormat
	// GetDetectionHints returns hints for auto-detection for this platform
	GetDetectionHints() DetectionHints
	// ParseModelsResponse parses platform-specific JSON response into an ModelInfo slice
	ParseModelsResponse(data []byte) ([]*ModelInfo, error)
	// GetPaths returns all the paths that this profile allows the proxy to request from
	GetPaths() []string
	// GetPath returns the path at the specified index
	GetPath(index int) string
}

// RequestParsingRules defines how to extract model names from different request types
type RequestParsingRules struct {
	ChatCompletionsPath string // /v1/chat/completions
	CompletionsPath     string // /v1/completions
	GeneratePath        string // /api/generate (ollama specific)
	ModelFieldName      string // "model" or "model_id"
	SupportsStreaming   bool
}

// ModelResponseFormat describes the expected JSON structure for model listing
type ModelResponseFormat struct {
	ResponseType    string
	ModelsFieldPath string
}

// DetectionHints provides patterns for auto-detection of platform types
type DetectionHints struct {
	UserAgentPatterns []string // e.g., ["ollama/"]
	ResponseHeaders   []string // e.g., ["X-ProfileOllama-Version"]
	PathIndicators    []string // e.g., ["/api/tags"]
}
