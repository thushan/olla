package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestNewLMDeployConverter(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()
	assert.NotNil(t, c)
	assert.Equal(t, constants.ProviderTypeLMDeploy, c.GetFormatName())
}

func TestLMDeployConverter_ConvertToFormat_Empty(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()
	result, err := c.ConvertToFormat([]*domain.UnifiedModel{}, ports.ModelFilters{})

	require.NoError(t, err)
	resp, ok := result.(profile.LMDeployResponse)
	require.True(t, ok)
	assert.Equal(t, "list", resp.Object)
	assert.Empty(t, resp.Data)
}

func TestLMDeployConverter_ConvertToFormat_SingleModel(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()

	model := &domain.UnifiedModel{
		ID: "internlm/internlm2_5-7b-chat",
		Aliases: []domain.AliasEntry{
			{
				Name:   "internlm/internlm2_5-7b-chat",
				Source: constants.ProviderTypeLMDeploy,
			},
		},
	}

	result, err := c.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	resp, ok := result.(profile.LMDeployResponse)
	require.True(t, ok)
	require.Len(t, resp.Data, 1)

	m := resp.Data[0]
	assert.Equal(t, "internlm/internlm2_5-7b-chat", m.ID)
	assert.Equal(t, "model", m.Object)
	assert.NotZero(t, m.Created)
	// org extracted from ID
	assert.Equal(t, "internlm", m.OwnedBy)
	// LMDeploy does not expose max_model_len — the field should remain zero-value
	// (permissions are always generated)
	require.Len(t, m.Permission, 1)
	assert.True(t, m.Permission[0].AllowSampling)
}

func TestLMDeployConverter_ConvertToFormat_NoOrgInID(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()

	model := &domain.UnifiedModel{
		ID: "simple-model",
	}

	result, err := c.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	resp, ok := result.(profile.LMDeployResponse)
	require.True(t, ok)
	require.Len(t, resp.Data, 1)
	// Default owner when there is no org/model-name slash
	assert.Equal(t, constants.ProviderTypeLMDeploy, resp.Data[0].OwnedBy)
}

func TestLMDeployConverter_ConvertToFormat_MultipleModels(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()

	models := []*domain.UnifiedModel{
		{
			ID: "internlm/internlm2_5-7b-chat",
			Aliases: []domain.AliasEntry{
				{Name: "internlm/internlm2_5-7b-chat", Source: constants.ProviderTypeLMDeploy},
			},
		},
		{
			ID: "meta-llama/Meta-Llama-3.1-8B-Instruct",
			Aliases: []domain.AliasEntry{
				{Name: "meta-llama/Meta-Llama-3.1-8B-Instruct", Source: constants.ProviderTypeLMDeploy},
			},
		},
	}

	result, err := c.ConvertToFormat(models, ports.ModelFilters{})

	require.NoError(t, err)
	resp, ok := result.(profile.LMDeployResponse)
	require.True(t, ok)
	assert.Equal(t, "list", resp.Object)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "internlm/internlm2_5-7b-chat", resp.Data[0].ID)
	assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", resp.Data[1].ID)
}

func TestLMDeployConverter_FallbackToAliasOrID(t *testing.T) {
	t.Parallel()

	c := NewLMDeployConverter()

	// No LMDeploy-sourced alias — should fall back to first alias
	modelWithOtherAlias := &domain.UnifiedModel{
		ID: "fallback-id",
		Aliases: []domain.AliasEntry{
			{Name: "alias-from-ollama", Source: constants.ProviderTypeOllama},
		},
	}

	result, err := c.ConvertToFormat([]*domain.UnifiedModel{modelWithOtherAlias}, ports.ModelFilters{})
	require.NoError(t, err)
	resp, ok := result.(profile.LMDeployResponse)
	require.True(t, ok)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "alias-from-ollama", resp.Data[0].ID)

	// No aliases at all — should use unified ID
	modelWithNoAlias := &domain.UnifiedModel{
		ID: "bare-id",
	}

	result2, err2 := c.ConvertToFormat([]*domain.UnifiedModel{modelWithNoAlias}, ports.ModelFilters{})
	require.NoError(t, err2)
	resp2, ok2 := result2.(profile.LMDeployResponse)
	require.True(t, ok2)
	require.Len(t, resp2.Data, 1)
	assert.Equal(t, "bare-id", resp2.Data[0].ID)
}
