package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestUnifiedConverter_ConvertToFormat(t *testing.T) {
	converter := NewUnifiedConverter()

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
					EndpointURL:  "http://localhost:11434",
					EndpointName: "Ollama Server",
					NativeName:   "llama3:latest",
					State:        "loaded",
					DiskSize:     40000000000,
				},
			},
			Capabilities:     []string{"chat", "completion"},
			MaxContextLength: int64Ptr(4096),
			PromptTemplateID: "chatml",
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
				{Name: "microsoft/phi-4", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL:  "http://localhost:11434",
					EndpointName: "Ollama Server",
					NativeName:   "phi4:latest",
					State:        "not-loaded",
					DiskSize:     8000000000,
				},
				{
					EndpointURL:  "http://localhost:1234",
					EndpointName: "LM Studio",
					NativeName:   "microsoft/phi-4",
					State:        "loaded",
					DiskSize:     8000000000,
				},
			},
			Capabilities: []string{"chat", "code"},
			Metadata: map[string]interface{}{
				"type": "llm",
			},
		},
	}

	t.Run("no filters", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(UnifiedModelResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 2)

		// Check first model
		assert.Equal(t, "llama/3:70b-q4km", response.Data[0].ID)
		assert.Equal(t, "model", response.Data[0].Object)
		assert.Equal(t, "olla", response.Data[0].OwnedBy)
		assert.NotNil(t, response.Data[0].Olla)
		assert.Equal(t, "llama", response.Data[0].Olla.Family)
		assert.Equal(t, "3", response.Data[0].Olla.Variant)
		assert.Equal(t, "70b", response.Data[0].Olla.ParameterSize)
		assert.Equal(t, "q4km", response.Data[0].Olla.Quantization)
		assert.Len(t, response.Data[0].Olla.Aliases, 2)
		assert.Len(t, response.Data[0].Olla.Availability, 1)
		assert.Equal(t, "Ollama Server", response.Data[0].Olla.Availability[0].Endpoint)
		assert.Equal(t, "loaded", response.Data[0].Olla.Availability[0].State)
		assert.Equal(t, "chatml", response.Data[0].Olla.PromptTemplateID)
		assert.Equal(t, int64(4096), *response.Data[0].Olla.MaxContextLength)
	})

	t.Run("filter by family", func(t *testing.T) {
		filters := ports.ModelFilters{
			Family: "phi",
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(UnifiedModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		assert.Equal(t, "phi/4:14.7b-q4km", response.Data[0].ID)
	})

	t.Run("filter by availability", func(t *testing.T) {
		avail := true
		filters := ports.ModelFilters{
			Available: &avail,
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(UnifiedModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 2) // Both have at least one loaded endpoint
	})

	t.Run("filter by endpoint", func(t *testing.T) {
		filters := ports.ModelFilters{
			Endpoint: "http://localhost:1234",
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(UnifiedModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		assert.Equal(t, "phi/4:14.7b-q4km", response.Data[0].ID)
	})

	t.Run("filter by type", func(t *testing.T) {
		filters := ports.ModelFilters{
			Type: "llm",
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(UnifiedModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 2) // Both are LLMs
	})
}

func int64Ptr(i int64) *int64 {
	return &i
}
