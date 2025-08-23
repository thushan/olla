package converter

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
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
type OllamaConverter struct {
	*BaseConverter
}

// NewOllamaConverter creates a new Ollama format converter
func NewOllamaConverter() ports.ModelResponseConverter {
	return &OllamaConverter{
		BaseConverter: NewBaseConverter(constants.ProviderTypeOllama),
	}
}

func (c *OllamaConverter) GetFormatName() string {
	return constants.ProviderTypeOllama
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
	helper := c.BaseConverter.NewConversionHelper(model)

	if helper.ShouldSkip() {
		return nil
	}

	return &OllamaModelData{
		Name:       helper.Alias,
		Model:      helper.Alias,
		ModifiedAt: model.LastSeen.Format(time.RFC3339),
		Size:       helper.GetDiskSize(),
		Digest:     helper.GetMetadataString("digest"),
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
