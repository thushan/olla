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
		assert.Equal(t, "phi4:latest", response.Data[0].ID)
	})
}

func TestOpenAIConverter_ConfigAliases(t *testing.T) {
	converter := NewOpenAIConverter()

	// Model with config-sourced aliases (Source="config")
	configAliasModel := &domain.UnifiedModel{
		ID: "my-gpt", // alias name is the ID
		Aliases: []domain.AliasEntry{
			{Name: "gpt-3.5-turbo", Source: "config"},
			{Name: "gpt-4", Source: "config"},
		},
		SourceEndpoints: []domain.SourceEndpoint{
			{
				EndpointURL: "http://openai:8080",
				NativeName:  "my-gpt",
				State:       "available",
			},
		},
		Capabilities: []string{"chat", "completion"},
	}

	// Model with regular (unification) aliases
	regularModel := &domain.UnifiedModel{
		ID: "llama/3:70b-q4km",
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
	}

	t.Run("config alias uses alias ID not first target", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat([]*domain.UnifiedModel{configAliasModel}, filters)
		require.NoError(t, err)

		response, ok := result.(OpenAIModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		// Config alias should use its own ID (my-gpt), not first target (gpt-3.5-turbo)
		assert.Equal(t, "my-gpt", response.Data[0].ID)
	})

	t.Run("regular alias uses first target as ID", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat([]*domain.UnifiedModel{regularModel}, filters)
		require.NoError(t, err)

		response, ok := result.(OpenAIModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		// Regular alias uses first alias as ID
		assert.Equal(t, "llama3:latest", response.Data[0].ID)
	})

	t.Run("mixed config and regular aliases", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat([]*domain.UnifiedModel{configAliasModel, regularModel}, filters)
		require.NoError(t, err)

		response, ok := result.(OpenAIModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 2)

		modelIDs := make([]string, len(response.Data))
		for i, m := range response.Data {
			modelIDs[i] = m.ID
		}

		assert.Contains(t, modelIDs, "my-gpt")
		assert.Contains(t, modelIDs, "llama3:latest")
		assert.NotContains(t, modelIDs, "gpt-3.5-turbo")
		assert.NotContains(t, modelIDs, "gpt-4")
	})
}
