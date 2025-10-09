package profile

/*
	Lemonade profile for AMD's Lemonade model registry.

	Reference (02/10/2025):
	- Lemonade API documentation: https://lemonade-server.ai/docs/server/server_spec/#start-the-rest-api-server

*/
// LemonadeResponse represents the response structure from Lemonade /api/v1/models endpoint
type LemonadeResponse struct {
	Object string          `json:"object"`
	Data   []LemonadeModel `json:"data"`
}

// LemonadeModel represents a model in Lemonade response with Lemonade-specific metadata
type LemonadeModel struct {
	// Standard OpenAI-compatible fields
	ID      string `json:"id"`       // mOdel identifier (e.g., "Qwen2.5-0.5B-Instruct-CPU")
	Object  string `json:"object"`   // always "model"
	OwnedBy string `json:"owned_by"` // always "lemonade"

	// Lemonade-specific fields (critical for local inference routing)
	Checkpoint string `json:"checkpoint"` // HuggingFace model path (e.g., "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx")
	Recipe     string `json:"recipe"`     // Inference engine (e.g., "oga-cpu", "oga-npu", "llamacpp", "flm")
	Created    int64  `json:"created"`    // Unix timestamp
}
