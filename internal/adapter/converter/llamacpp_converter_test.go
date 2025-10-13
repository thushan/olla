package converter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestLlamaCppConverter_ConvertToFormat(t *testing.T) {
	converter := NewLlamaCppConverter()

	t.Run("converts unified models to llama.cpp format with llamacpp alias", func(t *testing.T) {
		maxContext := int64(8192)
		models := []*domain.UnifiedModel{
			{
				ID:               "llama/3.1:8b-instruct",
				Family:           "llama",
				Variant:          "3.1",
				ParameterSize:    "8b",
				ParameterCount:   8000000000,
				Format:           "gguf",
				MaxContextLength: &maxContext,
				Aliases: []domain.AliasEntry{
					{Name: "llama-3.1-8b-instruct-q4_k_m.gguf", Source: "llamacpp"},
					{Name: "llama3.1:latest", Source: "ollama"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL:  "http://localhost:8080",
						EndpointName: "llamacpp-server",
						NativeName:   "llama-3.1-8b-instruct-q4_k_m.gguf",
						State:        "loaded",
					},
				},
				Capabilities: []string{"chat"},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok, "Result should be of type LlamaCppResponse")
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 1)

		model := response.Data[0]
		assert.Equal(t, "llama-3.1-8b-instruct-q4_k_m.gguf", model.ID, "Should use llamacpp alias as model ID")
		assert.Equal(t, "model", model.Object)
		assert.NotZero(t, model.Created, "Created timestamp should be set")
		assert.WithinDuration(t, time.Now(), time.Unix(model.Created, 0), 5*time.Second, "Created timestamp should be recent")

		// Owner should be extracted from filename pattern
		assert.Equal(t, "llamacpp", model.OwnedBy, "Should default to llamacpp when no organisation in name")
	})

	t.Run("handles models without llamacpp native names - fallback to first alias", func(t *testing.T) {
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
					{Name: "microsoft/phi-4", Source: "lmstudio"},
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

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)

		model := response.Data[0]
		assert.Equal(t, "phi4:latest", model.ID, "Should use first alias when no llamacpp alias exists")
		assert.Equal(t, "llamacpp", model.OwnedBy, "Should default to llamacpp owner")
	})

	t.Run("handles models without any aliases - fallback to unified ID", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:             "mistral/7b:instruct",
				Family:         "mistral",
				Variant:        "7b",
				ParameterSize:  "7b",
				ParameterCount: 7000000000,
				Format:         "gguf",
				Aliases:        []domain.AliasEntry{}, // No aliases
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL: "http://localhost:8080",
						NativeName:  "mistral-7b-instruct",
						State:       "loaded",
					},
				},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)

		model := response.Data[0]
		assert.Equal(t, "mistral/7b:instruct", model.ID, "Should use unified ID when no aliases exist")
	})

	t.Run("filters work correctly with family filter", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:     "tinyllama/1.1b:chat-v1.0",
				Family: "tinyllama",
				Aliases: []domain.AliasEntry{
					{Name: "tinyllama-1.1b-chat.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "tinyllama-1.1b-chat.gguf"},
				},
			},
			{
				ID:     "llama/3.1:8b",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "llama-3.1-8b.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "llama-3.1-8b.gguf"},
				},
			},
		}

		filters := ports.ModelFilters{
			Family: "llama",
		}

		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1, "Should only include models matching family filter")
		assert.Equal(t, "llama-3.1-8b.gguf", response.Data[0].ID)
	})

	t.Run("handles empty model list", func(t *testing.T) {
		models := []*domain.UnifiedModel{}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Empty(t, response.Data, "Should return empty data array for empty model list")
	})

	t.Run("handles organisation in path format - slash-separated", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:     "llama/3.1:8b",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "meta-llama/llama-3.1-8b-instruct.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "meta-llama/llama-3.1-8b-instruct.gguf"},
				},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)

		model := response.Data[0]
		assert.Equal(t, "meta-llama/llama-3.1-8b-instruct.gguf", model.ID)
		assert.Equal(t, "meta-llama", model.OwnedBy, "Should extract organisation from slash-separated path")
	})

	t.Run("handles hyphenated organisation prefix", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:     "mistral/7b:instruct",
				Family: "mistral",
				Aliases: []domain.AliasEntry{
					{Name: "mistralai-mistral-7b-instruct.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{NativeName: "mistralai-mistral-7b-instruct.gguf"},
				},
			},
		}

		filters := ports.ModelFilters{}
		result, err := converter.ConvertToFormat(models, filters)
		require.NoError(t, err)

		response, ok := result.(LlamaCppResponse)
		require.True(t, ok)
		assert.Len(t, response.Data, 1)

		model := response.Data[0]
		assert.Equal(t, "mistralai-mistral-7b-instruct.gguf", model.ID)
		// "mistralai" contains "mistral" which is a known organisation
		assert.Equal(t, "mistralai", model.OwnedBy, "Should extract organisation from hyphenated prefix")
	})

	t.Run("all models have consistent structure", func(t *testing.T) {
		models := []*domain.UnifiedModel{
			{
				ID:     "model1",
				Family: "test",
				Aliases: []domain.AliasEntry{
					{Name: "test-model-1.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{{NativeName: "test-model-1.gguf"}},
			},
			{
				ID:     "model2",
				Family: "test",
				Aliases: []domain.AliasEntry{
					{Name: "test-model-2.gguf", Source: "llamacpp"},
				},
				SourceEndpoints: []domain.SourceEndpoint{{NativeName: "test-model-2.gguf"}},
			},
		}

		result, err := converter.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response := result.(LlamaCppResponse)
		require.Len(t, response.Data, 2)

		// Verify all models have required fields
		for _, model := range response.Data {
			assert.Equal(t, "model", model.Object, "All models should have object type 'model'")
			assert.NotEmpty(t, model.ID, "All models should have an ID")
			assert.NotZero(t, model.Created, "All models should have creation timestamp")
			assert.NotEmpty(t, model.OwnedBy, "All models should have an owner")
		}
	})
}

func TestLlamaCppConverter_GetFormatName(t *testing.T) {
	converter := NewLlamaCppConverter()
	assert.Equal(t, "llamacpp", converter.GetFormatName(), "Format name should be 'llamacpp'")
}

func TestLlamaCppConverter_determineOwner(t *testing.T) {
	converter := NewLlamaCppConverter().(*LlamaCppConverter)

	tests := []struct {
		name     string
		modelID  string
		expected string
	}{
		{
			name:     "slash-separated path with organisation",
			modelID:  "meta-llama/llama-3.1-8b-instruct.gguf",
			expected: "meta-llama",
		},
		{
			name:     "slash-separated with different organisation",
			modelID:  "mistralai/mixtral-8x7b.gguf",
			expected: "mistralai",
		},
		{
			name:     "hyphenated prefix with known organisation",
			modelID:  "mistralai-mistral-7b-instruct.gguf",
			expected: "mistralai",
		},
		{
			name:     "hyphenated prefix with meta organisation",
			modelID:  "meta-llama-3-8b.gguf",
			expected: "meta",
		},
		{
			name:     "simple filename without organisation",
			modelID:  "tinyllama-1.1b.gguf",
			expected: "llamacpp",
		},
		{
			name:     "model name that looks like organisation but is not",
			modelID:  "llama-3-8b.gguf",
			expected: "llamacpp",
		},
		{
			name:     "unknown pattern defaults to llamacpp",
			modelID:  "custom-model.gguf",
			expected: "llamacpp",
		},
		{
			name:     "huggingface-style path",
			modelID:  "huggingface/gpt2.gguf",
			expected: "huggingface",
		},
		{
			name:     "single word without extension",
			modelID:  "gpt4",
			expected: "llamacpp",
		},
		{
			name:     "organisation with hyphen in prefix",
			modelID:  "google-gemma-7b.gguf",
			expected: "google",
		},
		{
			name:     "nomic organisation",
			modelID:  "nomic-embed-text.gguf",
			expected: "nomic",
		},
		{
			name:     "case sensitivity - uppercase organisation",
			modelID:  "Meta-Llama/model.gguf",
			expected: "Meta-Llama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.determineOwner(tt.modelID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLlamaCppConverter_findLlamaCppNativeName(t *testing.T) {
	converter := NewLlamaCppConverter().(*LlamaCppConverter)

	t.Run("finds llamacpp name from aliases with llamacpp source", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "llama/3.1:8b",
			Aliases: []domain.AliasEntry{
				{Name: "llama3.1:latest", Source: "ollama"},
				{Name: "llama-3.1-8b-instruct-q4_k_m.gguf", Source: "llamacpp"},
				{Name: "meta-llama/Meta-Llama-3.1-8B", Source: "vllm"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "llama-3.1-8b-instruct-q4_k_m.gguf"},
			},
		}

		result := converter.findLlamaCppNativeName(model)
		assert.Equal(t, "llama-3.1-8b-instruct-q4_k_m.gguf", result, "Should find llamacpp-specific alias")
	})

	t.Run("returns empty string when no llamacpp name found", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "phi/4:14b",
			Aliases: []domain.AliasEntry{
				{Name: "phi4:latest", Source: "ollama"},
				{Name: "microsoft/phi-4", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "phi4:latest"},
			},
		}

		result := converter.findLlamaCppNativeName(model)
		assert.Equal(t, "", result, "Should return empty string when no llamacpp alias exists")
	})

	t.Run("ignores names from other providers", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "mistral/7b",
			Aliases: []domain.AliasEntry{
				{Name: "mistral:latest", Source: "ollama"},
				{Name: "mistralai/Mistral-7B-Instruct-v0.2", Source: "vllm"},
				{Name: "mistral-7b-instruct", Source: "openai-compatible"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "mistral:latest"},
			},
		}

		result := converter.findLlamaCppNativeName(model)
		assert.Equal(t, "", result, "Should not pick up names from non-llamacpp sources")
	})

	t.Run("returns first llamacpp alias when multiple exist", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "llama/3:8b",
			Aliases: []domain.AliasEntry{
				{Name: "llama-3-8b-q4_k_m.gguf", Source: "llamacpp"},
				{Name: "llama-3-8b-q8_0.gguf", Source: "llamacpp"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "llama-3-8b-q4_k_m.gguf"},
			},
		}

		result := converter.findLlamaCppNativeName(model)
		assert.Equal(t, "llama-3-8b-q4_k_m.gguf", result, "Should return first llamacpp alias")
	})

	t.Run("handles model with only llamacpp alias", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "gemma/2:9b",
			Aliases: []domain.AliasEntry{
				{Name: "google-gemma-2-9b.gguf", Source: "llamacpp"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "google-gemma-2-9b.gguf"},
			},
		}

		result := converter.findLlamaCppNativeName(model)
		assert.Equal(t, "google-gemma-2-9b.gguf", result)
	})
}

func TestIsKnownOrganization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Known organisations
		{
			name:     "recognizes meta",
			input:    "meta",
			expected: true,
		},
		{
			name:     "recognizes mistral",
			input:    "mistral",
			expected: true,
		},
		{
			name:     "recognizes mistralai",
			input:    "mistralai",
			expected: true,
		},
		{
			name:     "recognizes huggingface",
			input:    "huggingface",
			expected: true,
		},
		{
			name:     "recognizes openai",
			input:    "openai",
			expected: true,
		},
		{
			name:     "recognizes anthropic",
			input:    "anthropic",
			expected: true,
		},
		{
			name:     "recognizes google",
			input:    "google",
			expected: true,
		},
		{
			name:     "recognizes microsoft",
			input:    "microsoft",
			expected: true,
		},
		{
			name:     "recognizes apple",
			input:    "apple",
			expected: true,
		},
		{
			name:     "recognizes nomic",
			input:    "nomic",
			expected: true,
		},
		// Case insensitive
		{
			name:     "case insensitive - Meta",
			input:    "Meta",
			expected: true,
		},
		{
			name:     "case insensitive - MISTRAL",
			input:    "MISTRAL",
			expected: true,
		},
		{
			name:     "case insensitive - HuggingFace",
			input:    "HuggingFace",
			expected: true,
		},
		// Substrings in compound names
		{
			name:     "meta-llama contains meta",
			input:    "meta-llama",
			expected: true,
		},
		{
			name:     "organisation as substring",
			input:    "mistralai",
			expected: true,
		},
		// Too short
		{
			name:     "rejects too short - 2 chars",
			input:    "ab",
			expected: false,
		},
		{
			name:     "rejects too short - single char",
			input:    "x",
			expected: false,
		},
		{
			name:     "accepts minimum length - 3 chars with known org",
			input:    "meta",
			expected: true,
		},
		// Too long
		{
			name:     "rejects too long - 21 chars",
			input:    "verylongorganization1",
			expected: false,
		},
		{
			name:     "rejects too long - 25 chars",
			input:    "verylongorganizationname1",
			expected: false,
		},
		{
			name:     "accepts maximum length - 20 chars",
			input:    "verylongorganization", // exactly 20 chars, but unknown
			expected: false,                  // still false because not in known list
		},
		// Unknown organisations
		{
			name:     "rejects unknown organisation",
			input:    "unknown",
			expected: false,
		},
		{
			name:     "rejects random string",
			input:    "xyz123",
			expected: false,
		},
		{
			name:     "rejects custom",
			input:    "custom",
			expected: false,
		},
		// Model names that are not organisations
		{
			name:     "rejects llama - model name not organisation",
			input:    "llama",
			expected: false,
		},
		{
			name:     "rejects phi - model name not organisation",
			input:    "phi",
			expected: false,
		},
		{
			name:     "rejects gemma - model name not organisation",
			input:    "gemma",
			expected: false,
		},
		{
			name:     "rejects mixtral - model name not organisation",
			input:    "mixtral",
			expected: false,
		},
		{
			name:     "rejects gpt - model name not organisation",
			input:    "gpt",
			expected: false, // gpt is 3 chars (min length) but doesn't match any known organisation
		},
		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "whitespace",
			input:    "   ",
			expected: false,
		},
		{
			name:     "numbers only",
			input:    "12345",
			expected: false,
		},
		{
			name:     "special characters",
			input:    "org-name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKnownOrganization(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
