package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// OllamaModelResponse represents the Ollama-compatible format response
type OllamaModelResponse struct {
	Models []OllamaModelData `json:"models"`
}

// OllamaModelData represents a single model in Ollama format
type OllamaModelData struct {
	Details    *OllamaDetails `json:"details,omitempty"`
	Name       string         `json:"name"`
	Model      string         `json:"model"`
	ModifiedAt string         `json:"modified_at"`
	Digest     string         `json:"digest"`
	Size       int64          `json:"size"`
}

// OllamaDetails represents model details in Ollama format
type OllamaDetails struct {
	Family            string `json:"family"`
	ParameterSize     string `json:"parameter_size"`
	QuantizationLevel string `json:"quantization_level"`
}

// OllamaConverter converts models to Ollama-compatible format
type OllamaConverter struct{}

// NewOllamaConverter creates a new Ollama format converter
func NewOllamaConverter() ports.ModelResponseConverter {
	return &OllamaConverter{}
}

func (c *OllamaConverter) GetFormatName() string {
	return "ollama"
}

func (c *OllamaConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	filtered := filterModels(models, filters)

	data := make([]OllamaModelData, 0, len(filtered))
	for _, model := range filtered {
		// For Ollama format, use the native Ollama name if available
		ollamaModel := c.convertModel(model)
		if ollamaModel != nil {
			data = append(data, *ollamaModel)
		}
	}

	return OllamaModelResponse{
		Models: data,
	}, nil
}

func (c *OllamaConverter) convertModel(model *domain.UnifiedModel) *OllamaModelData {
	// Find the Ollama-specific alias or endpoint
	var ollamaName string
	var ollamaEndpoint *domain.SourceEndpoint

	// First, look for an Ollama source in aliases
	for _, alias := range model.Aliases {
		if alias.Source == "ollama" {
			ollamaName = alias.Name
			break
		}
	}

	// Find corresponding Ollama endpoint
	for i := range model.SourceEndpoints {
		ep := &model.SourceEndpoints[i]
		// Check if this is an Ollama endpoint (could be by port or other logic)
		if ollamaName != "" && ep.NativeName == ollamaName {
			ollamaEndpoint = ep
			break
		}
	}

	// If no Ollama-specific data, skip this model for Ollama format
	if ollamaName == "" {
		return nil
	}

	// Extract digest from metadata if available
	digest := ""
	if d, ok := model.Metadata["digest"].(string); ok {
		digest = d
	}

	// Use actual disk size from endpoint if available, otherwise use total
	size := model.DiskSize
	if ollamaEndpoint != nil && ollamaEndpoint.DiskSize > 0 {
		size = ollamaEndpoint.DiskSize
	}

	return &OllamaModelData{
		Name:       ollamaName,
		Model:      ollamaName,
		ModifiedAt: model.LastSeen.Format(time.RFC3339),
		Size:       size,
		Digest:     digest,
		Details: &OllamaDetails{
			Family:            model.Family,
			ParameterSize:     model.ParameterSize,
			QuantizationLevel: denormalizeQuantization(model.Quantization),
		},
	}
}

// denormalizeQuantization converts normalized quantization back to Ollama format
func denormalizeQuantization(quant string) string {
	// Reverse the normalization mappings
	mappings := map[string]string{
		"q4km": "Q4_K_M",
		"q4ks": "Q4_K_S",
		"q3kl": "Q3_K_L",
		"q3km": "Q3_K_M",
		"q3ks": "Q3_K_S",
		"q5km": "Q5_K_M",
		"q5ks": "Q5_K_S",
		"q6k":  "Q6_K",
		"q8":   "Q8_0",
		"q4":   "Q4_0",
		"q5":   "Q5_0",
		"f16":  "F16",
		"f32":  "F32",
		"unk":  "unknown",
	}

	if denormalized, exists := mappings[quant]; exists {
		return denormalized
	}

	// Default: uppercase the quantization
	return quant
}
