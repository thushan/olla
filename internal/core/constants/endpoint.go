package constants

const (
	DefaultHealthCheckEndpoint = "/internal/health"
	DefaultMetricsEndpoint     = "/internal/metrics"
	DefaultOllaProxyPathPrefix = "/olla/"
	DefaultPathPrefix          = "/"

	// PathOllaModelsPrefix is the subtree /olla/models/{id}; the id may itself
	// contain slashes (Hugging Face style org/name ids).
	PathOllaModelsPrefix = "/olla/models/"

	// OpenAI-compatible API paths
	PathV1ChatCompletions = "/v1/chat/completions"
	PathV1Completions     = "/v1/completions"
)
