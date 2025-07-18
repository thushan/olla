package converter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestOllamaConverter_ConvertToFormat(t *testing.T) {
	converter := NewOllamaConverter()

	now := time.Now()
	models := []*domain.UnifiedModel{
		{
			ID:             "llama/3:70b-q4km",
			Family:         "llama",
			Variant:        "3",
			ParameterSize:  "70b",
			ParameterCount: 70000000000,
			Quantization:   "q4km",
			Format:         "gguf",
			Aliases: []domain.AliasEntry{
				{Name: "llama3:latest", Source: "ollama"},
				{Name: "llama3:70b", Source: "generated"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "llama3:latest",
					State:       "loaded",
					DiskSize:    40000000000,
				},
			},
			Capabilities: []string{"chat", "completion"},
			DiskSize:     40000000000,
			LastSeen:     now,
			Metadata: map[string]interface{}{
				"digest": "sha256:abc123def456",
			},
		},
		{
			ID:             "phi/4:14.7b-q4km",
			Family:         "phi",
			Variant:        "4",
			ParameterSize:  "14.7b",
			ParameterCount: 14700000000,
			Quantization:   "q4km",
			Format:         "gguf",
			Aliases: []domain.AliasEntry{
				{Name: "microsoft/phi-4", Source: "lmstudio"}, // No Ollama alias
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "microsoft/phi-4",
					State:       "loaded",
					DiskSize:    8000000000,
				},
			},
			Capabilities: []string{"chat", "code"},
			DiskSize:     8000000000,
			LastSeen:     now,
		},
	}

	t.Run("Ollama format only includes Ollama models", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(OllamaModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Models, 1) // Only the llama model has Ollama alias

		// Check the model
		model := response.Models[0]
		assert.Equal(t, "llama3:latest", model.Name)
		assert.Equal(t, "llama3:latest", model.Model)
		assert.Equal(t, now.Format(time.RFC3339), model.ModifiedAt)
		assert.Equal(t, int64(40000000000), model.Size)
		assert.Equal(t, "sha256:abc123def456", model.Digest)

		// Check details
		assert.NotNil(t, model.Details)
		assert.Equal(t, "llama", model.Details.Family)
		assert.Equal(t, "70b", model.Details.ParameterSize)
		assert.Equal(t, "Q4_K_M", model.Details.QuantizationLevel) // Denormalized
	})

	t.Run("quantization denormalization", func(t *testing.T) {
		testCases := []struct {
			normalized   string
			denormalized string
		}{
			{"q4km", "Q4_K_M"},
			{"q4ks", "Q4_K_S"},
			{"q8", "Q8_0"},
			{"f16", "F16"},
			{"unk", "unknown"},
		}

		for _, tc := range testCases {
			assert.Equal(t, tc.denormalized, denormalizeQuantization(tc.normalized))
		}
	})
}
