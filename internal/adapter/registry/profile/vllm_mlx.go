package profile

// VLLMMLXResponse is the response structure from vLLM-MLX /v1/models endpoint.
// vLLM-MLX uses the standard OpenAI-compatible list format.
type VLLMMLXResponse struct {
	Object string         `json:"object"`
	Data   []VLLMMLXModel `json:"data"`
}

// VLLMMLXModel represents a single model in a vLLM-MLX response.
// vLLM-MLX exclusively serves MLX models on Apple Silicon.
type VLLMMLXModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Created int64  `json:"created"`
}
