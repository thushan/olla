package profile

// VLLMResponse is the response structure from vLLM /v1/models endpoint
type VLLMResponse struct {
	Object string      `json:"object"`
	Data   []VLLMModel `json:"data"`
}

// VLLMModel represents a model in vLLM response with extended metadata
type VLLMModel struct {
	MaxModelLen *int64                `json:"max_model_len,omitempty"` // vLLM-specific: max context length
	Parent      *string               `json:"parent,omitempty"`        // vLLM-specific: parent for fine-tuned models
	OwnedBy     string                `json:"owned_by"`
	ID          string                `json:"id"`
	Object      string                `json:"object"`
	Root        string                `json:"root,omitempty"`       // vLLM-specific: root model ID
	Permission  []VLLMModelPermission `json:"permission,omitempty"` // vLLM-specific: permissions array
	Created     int64                 `json:"created"`
}

// VLLMModelPermission represents granular permissions in vLLM
type VLLMModelPermission struct {
	Group              *string `json:"group"`
	ID                 string  `json:"id"`
	Object             string  `json:"object"`
	Organization       string  `json:"organization"`
	Created            int64   `json:"created"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	IsBlocking         bool    `json:"is_blocking"`
}
