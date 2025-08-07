package profile

// VLLMResponse is the response structure from vLLM /v1/models endpoint
type VLLMResponse struct {
	Object string      `json:"object"`
	Data   []VLLMModel `json:"data"`
}

// VLLMModel represents a model in vLLM response with extended metadata
type VLLMModel struct {
	ID          string                `json:"id"`
	Object      string                `json:"object"`
	Created     int64                 `json:"created"`
	OwnedBy     string                `json:"owned_by"`
	Root        string                `json:"root,omitempty"`          // vLLM-specific: root model identifier
	Parent      *string               `json:"parent,omitempty"`        // vLLM-specific: parent model for fine-tuned models
	MaxModelLen *int64                `json:"max_model_len,omitempty"` // vLLM-specific: maximum context length
	Permission  []VLLMModelPermission `json:"permission,omitempty"`    // vLLM-specific: access permissions
}

// VLLMModelPermission represents vLLM's granular permission system
type VLLMModelPermission struct {
	ID                 string  `json:"id"`
	Object             string  `json:"object"`
	Created            int64   `json:"created"`
	Organization       string  `json:"organization"`
	Group              *string `json:"group"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	IsBlocking         bool    `json:"is_blocking"`
}