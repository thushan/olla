package profile

// OllamaResponse is the response structure from Ollama's /api/tags endpoint
type OllamaResponse struct {
	Models []OllamaModel `json:"models"`
}

// OllamaModel represents a model in Ollama's response
type OllamaModel struct {
	Size        *int64         `json:"size,omitempty"`
	Digest      *string        `json:"digest,omitempty"`
	ModifiedAt  *string        `json:"modified_at,omitempty"`
	Description *string        `json:"description,omitempty"`
	Details     *OllamaDetails `json:"details,omitempty"`
	Name        string         `json:"name"`
}

// OllamaDetails contains additional model metadata from Ollama
type OllamaDetails struct {
	ParameterSize     *string  `json:"parameter_size,omitempty"`
	QuantizationLevel *string  `json:"quantization_level,omitempty"`
	Family            *string  `json:"family,omitempty"`
	Format            *string  `json:"format,omitempty"`
	ParentModel       *string  `json:"parent_model,omitempty"`
	Families          []string `json:"families,omitempty"`
}
