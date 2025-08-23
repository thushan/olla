package converter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestVLLMConverter_ConvertToFormat(t *testing.T) {
	converter := NewVLLMConverter()

	t.Run("converts unified models to vLLM format with extended metadata", func(t *testing.T) {
		maxContext := int64(2048)
		models := []*domain.UnifiedModel{
			{
				ID:               "tinyllama/1.1b:chat-v1.0",
				Family:           "tinyllama",
				Variant:          "1.1b",
				ParameterSize:    "1.1b",
				ParameterCount:   1100000000,
				Format:           "safetensors",
				MaxContextLength: &maxContext,
				Aliases: []domain.AliasEntry{
					{Name: "TinyLlama/TinyLlama-1.1B-Chat-v1.0", Source: "vllm"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL:  "http://192.168.0.1:8000",
						EndpointName: "vllm-server",
						NativeName:   "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
						State:        "loaded",
					},
				},
				Capabilities: []string{"chat"},
			},
			{
				ID:               "llama/3.1:8b-instruct",
				Family:           "llama",
				Variant:          "3.1",
				ParameterSize:    "8b",
				ParameterCount:   8000000000,
				Format:           "safetensors",
				MaxContextLength: &[]int64{131072}[0],
				Aliases: []domain.AliasEntry{
					{Name: "meta-llama/Meta-Llama-3.1-8B-Instruct", Source: "vllm"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL:  "http://192.168.0.1:8000",
						EndpointName: "vllm-server",
						NativeName:   "meta-llama/Meta-Llama-3.1-8B-Instruct",
						State:        "available",
					},
				},
				Capabilities: []string{"chat", "completion"},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(VLLMModelResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 2)

		// Check first model (TinyLlama)
		tinyllama := response.Data[0]
		assert.Equal(t, "TinyLlama/TinyLlama-1.1B-Chat-v1.0", tinyllama.ID)
		assert.Equal(t, "model", tinyllama.Object)
		assert.Equal(t, "TinyLlama", tinyllama.OwnedBy)
		assert.Equal(t, "TinyLlama/TinyLlama-1.1B-Chat-v1.0", tinyllama.Root)
		assert.NotNil(t, tinyllama.MaxModelLen)
		assert.Equal(t, int64(2048), *tinyllama.MaxModelLen)

		// Check permissions are generated
		require.Len(t, tinyllama.Permission, 1)
		perm := tinyllama.Permission[0]
		assert.Equal(t, "model_permission", perm.Object)
		assert.True(t, perm.AllowSampling)
		assert.True(t, perm.AllowLogprobs)
		assert.True(t, perm.AllowView)
		assert.Equal(t, "*", perm.Organization)
		assert.False(t, perm.IsBlocking)

		// Check second model (Llama 3.1)
		llama := response.Data[1]
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", llama.ID)
		assert.Equal(t, "meta-llama", llama.OwnedBy)
		assert.NotNil(t, llama.MaxModelLen)
		assert.Equal(t, int64(131072), *llama.MaxModelLen)
	})

	t.Run("handles models without vLLM native names", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:             "phi/4:14.7b-q4km",
				Family:         "phi",
				Variant:        "4",
				ParameterSize:  "14.7b",
				ParameterCount: 14700000000,
				Format:         "gguf",
				Aliases: []domain.AliasEntry{
					{Name: "phi4:latest", Source: "ollama"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL: "http://localhost:11434",
						NativeName:  "phi4:latest",
						State:       "loaded",
					},
				},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(VLLMModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)

		// Should use first alias when no vLLM native name exists
		model := response.Data[0]
		assert.Equal(t, "phi4:latest", model.ID)
		assert.Equal(t, "vllm", model.OwnedBy) // Default owner when no org in name
		assert.Nil(t, model.MaxModelLen)       // No context length specified
	})

	t.Run("filters work correctly with vLLM format", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:            "tinyllama/1.1b:chat-v1.0",
				Family:        "tinyllama",
				Variant:       "1.1b",
				ParameterSize: "1.1b",
				Aliases: []domain.AliasEntry{
					{Name: "TinyLlama/TinyLlama-1.1B-Chat-v1.0", Source: "vllm"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "TinyLlama/TinyLlama-1.1B-Chat-v1.0"},
				},
			},
			{
				ID:            "llama/3.1:8b",
				Family:        "llama",
				Variant:       "3.1",
				ParameterSize: "8b",
				Aliases: []domain.AliasEntry{
					{Name: "meta-llama/Meta-Llama-3.1-8B-Instruct", Source: "vllm"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "meta-llama/Meta-Llama-3.1-8B-Instruct"},
				},
			},
		}

		filters := ports.ModelFilters{
			Family: "llama",
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(VLLMModelResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", response.Data[0].ID)
	})

	t.Run("generates consistent permissions for all models", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID: "test/model",
				Aliases: []domain.AliasEntry{
					{Name: "org/test-model", Source: "vllm"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "org/test-model"},
				},
			},
		}

		result, err := converter.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response := result.(VLLMModelResponse)
		model := response.Data[0]

		// Permission ID should be deterministic based on model ID
		assert.Contains(t, model.Permission[0].ID, "modelperm-olla-org-test-model")

		// Verify permission timestamp is recent
		assert.WithinDuration(t, time.Now(), time.Unix(model.Permission[0].Created, 0), 5*time.Second)
	})
}

func TestVLLMConverter_GetFormatName(t *testing.T) {
	converter := NewVLLMConverter()
	assert.Equal(t, "vllm", converter.GetFormatName())
}

func TestVLLMConverter_determineOwner(t *testing.T) {
	converter := NewVLLMConverter().(*VLLMConverter)

	tests := []struct {
		name     string
		modelID  string
		expected string
	}{
		{
			name:     "extracts organisation from slash-separated name",
			modelID:  "meta-llama/Meta-Llama-3.1-8B-Instruct",
			expected: "meta-llama",
		},
		{
			name:     "handles TinyLlama format",
			modelID:  "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
			expected: "TinyLlama",
		},
		{
			name:     "defaults to vllm for non-slash names",
			modelID:  "llama3:latest",
			expected: "vllm",
		},
		{
			name:     "handles single word models",
			modelID:  "gpt4",
			expected: "vllm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.determineOwner(tt.modelID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVLLMConverter_findVLLMNativeName(t *testing.T) {
	converter := NewVLLMConverter().(*VLLMConverter)

	t.Run("only finds vLLM name from aliases with vllm source", func(t *testing.T) {
		// Test that slash-based names from non-vLLM sources are ignored
		model := &domain.UnifiedModel{
			ID: "tinyllama/1.1b",
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "TinyLlama/TinyLlama-1.1B-Chat-v1.0"},
			},
			Aliases: []domain.AliasEntry{
				{Name: "TinyLlama/TinyLlama-1.1B-Chat-v1.0", Source: "vllm"},
			},
		}

		result := converter.findVLLMNativeName(model)
		assert.Equal(t, "TinyLlama/TinyLlama-1.1B-Chat-v1.0", result)
	})

	t.Run("finds vLLM name from aliases when source is vllm", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "llama/3.1:8b",
			Aliases: []domain.AliasEntry{
				{Name: "llama3:latest", Source: "ollama"},
				{Name: "meta-llama/Meta-Llama-3.1-8B-Instruct", Source: "vllm"},
			},
		}

		result := converter.findVLLMNativeName(model)
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", result)
	})

	t.Run("returns empty string when no vLLM name found", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "phi/4:14b",
			Aliases: []domain.AliasEntry{
				{Name: "phi4:latest", Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "phi4:latest"},
			},
		}

		result := converter.findVLLMNativeName(model)
		assert.Equal(t, "", result)
	})

	t.Run("ignores slash-based names from other providers", func(t *testing.T) {
		// Important: Test that Ollama models with slashes are not mistaken for vLLM
		model := &domain.UnifiedModel{
			ID: "mistral/7b",
			Aliases: []domain.AliasEntry{
				{Name: "mistral/mistral-7b-instruct", Source: "ollama"},
				{Name: "huggingface/mistral-7b", Source: "openai-compatible"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "mistral/mistral-7b-instruct"},
			},
		}

		result := converter.findVLLMNativeName(model)
		assert.Equal(t, "", result, "Should not pick up slash-based names from non-vLLM sources")
	})
}
