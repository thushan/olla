package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestVLLMMLXConverter_GetFormatName(t *testing.T) {
	t.Parallel()
	c := NewVLLMMLXConverter()
	assert.Equal(t, constants.ProviderTypeVLLMMLX, c.GetFormatName())
}

func TestVLLMMLXConverter_ConvertToFormat(t *testing.T) {
	t.Parallel()

	c := NewVLLMMLXConverter()

	t.Run("converts model with vllm-mlx alias", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:     "mlx-community/Llama-3.2-3B-Instruct-4bit",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "mlx-community/Llama-3.2-3B-Instruct-4bit", Source: constants.ProviderTypeVLLMMLX},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		require.Len(t, response.Data, 1)

		m := response.Data[0]
		assert.Equal(t, "mlx-community/Llama-3.2-3B-Instruct-4bit", m.ID)
		assert.Equal(t, "model", m.Object)
		assert.NotZero(t, m.Created)
		// namespace "mlx-community" extracted from "mlx-community/Llama-3.2-3B-Instruct-4bit"
		assert.Equal(t, "mlx-community", m.OwnedBy)
	})

	t.Run("falls back to first alias when no vllm-mlx alias", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:     "mlx-community/Llama-3.2-3B-Instruct-4bit",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "llama3.2:3b-instruct", Source: "ollama"},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "llama3.2:3b-instruct", response.Data[0].ID)
	})

	t.Run("falls back to unified ID when no aliases", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:      "mlx-community/Llama-3.2-3B-Instruct-4bit",
				Aliases: []domain.AliasEntry{},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "mlx-community/Llama-3.2-3B-Instruct-4bit", response.Data[0].ID)
	})

	t.Run("extracts owner from namespace/name format", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID: "mlx-community/Llama-3.2-3B-Instruct-4bit",
				Aliases: []domain.AliasEntry{
					{Name: "mlx-community/Llama-3.2-3B-Instruct-4bit", Source: constants.ProviderTypeVLLMMLX},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "mlx-community", response.Data[0].OwnedBy)
	})

	t.Run("defaults owner for models without namespace", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID: "simple-model",
				Aliases: []domain.AliasEntry{
					{Name: "simple-model", Source: constants.ProviderTypeVLLMMLX},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, constants.ProviderTypeVLLMMLX, response.Data[0].OwnedBy)
	})

	t.Run("handles empty model list", func(t *testing.T) {
		t.Parallel()
		result, err := c.ConvertToFormat([]*domain.UnifiedModel{}, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Empty(t, response.Data)
	})

	t.Run("response has correct structure", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID: "mlx-community/Llama-3.2-3B-Instruct-4bit",
				Aliases: []domain.AliasEntry{
					{Name: "mlx-community/Llama-3.2-3B-Instruct-4bit", Source: constants.ProviderTypeVLLMMLX},
				},
			},
			{
				ID: "mlx-community/Mistral-7B-Instruct-v0.3-4bit",
				Aliases: []domain.AliasEntry{
					{Name: "mlx-community/Mistral-7B-Instruct-v0.3-4bit", Source: constants.ProviderTypeVLLMMLX},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(VLLMMLXResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		require.Len(t, response.Data, len(models))
		for _, m := range response.Data {
			assert.Equal(t, "model", m.Object)
		}
	})
}

func TestVLLMMLXConverter_DetermineOwner(t *testing.T) {
	t.Parallel()

	c := NewVLLMMLXConverter().(*VLLMMLXConverter)

	cases := []struct {
		modelID  string
		expected string
	}{
		{"mlx-community/model", "mlx-community"},
		{"org/model", "org"},
		{"no-slash", constants.ProviderTypeVLLMMLX},
		{"a/b/c", "a"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.modelID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, c.determineOwner(tc.modelID))
		})
	}
}
