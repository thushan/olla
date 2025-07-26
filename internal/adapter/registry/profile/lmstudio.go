package profile

// LMStudioResponse is the response structure from LM Studio's /api/v0/models endpoint
// LM Studio's beta API gives us way more model metadata than the OpenAI endpoints.
// It tells us quantization levels, architecture, and whether models are loaded
// into memory - pretty handy for smart routing decisions.
type LMStudioResponse struct {
	Object string          `json:"object"`
	Data   []LMStudioModel `json:"data"`
}

// LMStudioModel represents a model in LM Studio's response
type LMStudioModel struct {
	Type              *string `json:"type,omitempty"`
	Publisher         *string `json:"publisher,omitempty"`
	Arch              *string `json:"arch,omitempty"`
	CompatibilityType *string `json:"compatibility_type,omitempty"`
	Quantization      *string `json:"quantization,omitempty"`
	State             *string `json:"state,omitempty"`
	MaxContextLength  *int64  `json:"max_context_length,omitempty"`
	ID                string  `json:"id"`
	Object            string  `json:"object"`
}
