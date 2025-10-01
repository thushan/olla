package profile

// SGLangResponse represents the response structure from SGLang /v1/models endpoint
type SGLangResponse struct {
	Object string        `json:"object"`
	Data   []SGLangModel `json:"data"`
}

// SGLangModel represents a model in SGLang response with extended metadata
type SGLangModel struct {

	// SGLang-specific fields
	Parent          *string `json:"parent,omitempty"`           // Parent model for fine-tuned models
	MaxModelLen     *int64  `json:"max_model_len,omitempty"`    // Maximum context length
	SupportsVision  *bool   `json:"supports_vision,omitempty"`  // Vision capability
	RadixCacheSize  *int64  `json:"radix_cache_size,omitempty"` // RadixAttention cache size
	SpecDecoding    *bool   `json:"spec_decoding,omitempty"`    // Speculative decoding support
	FrontendEnabled *bool   `json:"frontend_enabled,omitempty"` // Frontend Language support
	// Standard OpenAI-compatible fields
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Root    string `json:"root,omitempty"`

	Created int64 `json:"created"`
}
