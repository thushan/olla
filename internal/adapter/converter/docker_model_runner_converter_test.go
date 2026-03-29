package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestDockerModelRunnerConverter_GetFormatName(t *testing.T) {
	t.Parallel()
	c := NewDockerModelRunnerConverter()
	assert.Equal(t, constants.ProviderTypeDockerMR, c.GetFormatName())
}

func TestDockerModelRunnerConverter_ConvertToFormat(t *testing.T) {
	t.Parallel()

	c := NewDockerModelRunnerConverter()

	t.Run("converts model with DMR alias", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:     "llama/3.1:8b-q4km",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "ai/llama-3.1-8b", Source: constants.ProviderTypeDockerMR},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		require.Len(t, response.Data, 1)

		m := response.Data[0]
		assert.Equal(t, "ai/llama-3.1-8b", m.ID)
		assert.Equal(t, "model", m.Object)
		assert.NotZero(t, m.Created)
		// namespace "ai" extracted from "ai/llama-3.1-8b"
		assert.Equal(t, "ai", m.OwnedBy)
	})

	t.Run("falls back to first alias when no DMR alias", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:     "llama/3.1:8b-q4km",
				Family: "llama",
				Aliases: []domain.AliasEntry{
					{Name: "llama3.1:8b-q4_K_M", Source: "ollama"},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "llama3.1:8b-q4_K_M", response.Data[0].ID)
	})

	t.Run("falls back to unified ID when no aliases at all", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID:      "fallback/model",
				Aliases: []domain.AliasEntry{},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "fallback/model", response.Data[0].ID)
	})

	t.Run("extracts owner from namespace/name format", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID: "test/model",
				Aliases: []domain.AliasEntry{
					{Name: "huggingface/test-model", Source: constants.ProviderTypeDockerMR},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "huggingface", response.Data[0].OwnedBy)
	})

	t.Run("defaults owner to docker for models without namespace", func(t *testing.T) {
		t.Parallel()
		models := []*domain.UnifiedModel{
			{
				ID: "simple-model",
				Aliases: []domain.AliasEntry{
					{Name: "simple-model", Source: constants.ProviderTypeDockerMR},
				},
			},
		}

		result, err := c.ConvertToFormat(models, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "docker", response.Data[0].OwnedBy)
	})

	t.Run("handles empty model list", func(t *testing.T) {
		t.Parallel()
		result, err := c.ConvertToFormat([]*domain.UnifiedModel{}, ports.ModelFilters{})
		require.NoError(t, err)

		response, ok := result.(DockerModelRunnerResponse)
		require.True(t, ok)
		assert.Equal(t, "list", response.Object)
		assert.Empty(t, response.Data)
	})
}

func TestDockerModelRunnerConverter_DetermineOwner(t *testing.T) {
	t.Parallel()

	c := NewDockerModelRunnerConverter().(*DockerModelRunnerConverter)

	cases := []struct {
		modelID  string
		expected string
	}{
		{"ai/smollm2", "ai"},
		{"docker/llama3.2", "docker"},
		{"huggingface/model", "huggingface"},
		{"simple-model", "docker"},
		{"no-slash", "docker"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.modelID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, c.determineOwner(tc.modelID))
		})
	}
}
