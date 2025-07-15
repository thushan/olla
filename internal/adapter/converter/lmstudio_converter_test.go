package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestLMStudioConverter_ConvertToFormat(t *testing.T) {
	converter := NewLMStudioConverter()

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
				{Name: "llama3:latest", Source: "ollama"}, // No LM Studio alias
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
				{Name: "microsoft/phi-4", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "phi4:latest",
					State:       "not-loaded",
				},
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "microsoft/phi-4",
					State:       "loaded",
				},
			},
			Capabilities:     []string{"chat", "code"},
			MaxContextLength: int64Ptr(131072),
			Metadata: map[string]interface{}{
				"type":      "llm",
				"publisher": "microsoft",
			},
		},
		{
			ID:             "qwen/vlm:7b-q4",
			Family:         "qwen",
			Variant:        "vlm",
			ParameterSize:  "7b",
			ParameterCount: 7000000000,
			Quantization:   "q4",
			Format:         "gguf",
			Aliases: []domain.AliasEntry{
				{Name: "qwen/qwen-vlm", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "qwen/qwen-vlm",
					State:       "not-loaded",
				},
			},
			Capabilities: []string{"vision", "multimodal"},
			Metadata: map[string]interface{}{
				"vendor": "qwen",
			},
		},
	}

	t.Run("LM Studio format only includes LM Studio models", func(t *testing.T) {
		filters := ports.ModelFilters{}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LMStudioModelResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 2) // Only phi and qwen have LM Studio aliases

		// Check phi model
		phiModel := response.Data[0]
		assert.Equal(t, "microsoft/phi-4", phiModel.ID)
		assert.Equal(t, "model", phiModel.Object)
		assert.Equal(t, "llm", phiModel.Type)
		assert.Equal(t, "microsoft", phiModel.Publisher)
		assert.Equal(t, "phi", phiModel.Arch)
		assert.Equal(t, "Q4_K_M", phiModel.Quantization) // Denormalized
		assert.Equal(t, "loaded", phiModel.State)
		assert.Equal(t, int64(131072), *phiModel.MaxContextLength)

		// Check qwen VLM model
		qwenModel := response.Data[1]
		assert.Equal(t, "qwen/qwen-vlm", qwenModel.ID)
		assert.Equal(t, "vlm", qwenModel.Type)       // Inferred from capabilities
		assert.Equal(t, "qwen", qwenModel.Publisher) // From vendor metadata
		assert.Equal(t, "qwen", qwenModel.Arch)
		assert.Equal(t, "Q4_0", qwenModel.Quantization)
		assert.Equal(t, "not-loaded", qwenModel.State)
	})

	t.Run("type inference from capabilities", func(t *testing.T) {
		testModel := &domain.UnifiedModel{
			ID:     "test/model",
			Family: "test",
			Aliases: []domain.AliasEntry{
				{Name: "test/embedder", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "test/embedder",
					State:       "loaded",
				},
			},
			Capabilities: []string{"embeddings"},
			Quantization: "f16",
		}

		result, err := converter.ConvertToFormat([]*domain.UnifiedModel{testModel}, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(LMStudioModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		assert.Equal(t, "embeddings", response.Data[0].Type)
	})
}
