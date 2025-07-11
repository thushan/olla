package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestOpenAIConverter_ConvertToFormat(t *testing.T) {
	converter := NewOpenAIConverter()
	
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
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "llama3:latest",
					State:       "loaded",
				},
			},
			Capabilities: []string{"chat", "completion"},
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
				{Name: "phi4:latest", Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "phi4:latest",
					State:       "not-loaded",
				},
			},
			Capabilities: []string{"chat", "code"},
		},
	}

	t.Run("OpenAI format strips Olla extensions", func(t *testing.T) {
		filters := ports.ModelFilters{}
		
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)
		
		response, ok := result.(OpenAIModelResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 2)
		
		// Check that only OpenAI-compatible fields are present
		for _, model := range response.Data {
			assert.NotEmpty(t, model.ID)
			assert.Equal(t, "model", model.Object)
			assert.NotZero(t, model.Created)
			assert.Equal(t, "olla", model.OwnedBy)
			
			// Ensure no Olla-specific fields in the struct
			// (They're not even in the struct definition)
		}
	})

	t.Run("filters still work with OpenAI format", func(t *testing.T) {
		filters := ports.ModelFilters{
			Family: "phi",
		}
		
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)
		
		response, ok := result.(OpenAIModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		assert.Equal(t, "phi/4:14.7b-q4km", response.Data[0].ID)
	})
}