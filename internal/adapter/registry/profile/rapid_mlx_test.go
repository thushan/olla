package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

func TestShippedRapidMLXProfile(t *testing.T) {
	t.Parallel()

	factory, err := NewFactory(shippedProfilesDir)
	require.NoError(t, err)
	require.Empty(t, factory.LoadWarnings())
	require.True(t, factory.ValidateProfileType(domain.ProfileRapidMLX))
	p, err := factory.GetProfile(domain.ProfileRapidMLX)
	require.NoError(t, err)
	assert.Equal(t, domain.ProfileRapidMLX, p.GetName())
	assert.True(t, p.IsOpenAPICompatible())
	assert.Equal(t, "http://localhost:8000/v1/models", p.GetModelDiscoveryURL("http://localhost:8000"))
	assert.Equal(t, "/v1/models", p.GetHealthCheckPath())
	assert.Equal(t, []string{"rapid-mlx"}, p.GetConfig().Routing.Prefixes)
	assert.Equal(t, []string{
		"/v1/models", "/v1/chat/completions", "/v1/embeddings", "/v1/messages", "/v1/responses",
	}, p.GetPaths())
	cfg := p.GetConfig()
	assert.Equal(t, "/v1/models", p.GetPath(cfg.PathIndices.Health))
	assert.Equal(t, "/v1/models", p.GetPath(cfg.PathIndices.Models))
	assert.Equal(t, "/v1/chat/completions", p.GetPath(cfg.PathIndices.ChatCompletions))
	assert.Equal(t, "/v1/embeddings", p.GetPath(cfg.PathIndices.Embeddings))
	support := factory.GetAnthropicSupport(domain.ProfileRapidMLX)
	require.NotNil(t, support)
	assert.True(t, support.Enabled)
	assert.Equal(t, "/v1/messages", support.MessagesPath)
	assert.False(t, support.TokenCount)

	models, err := p.ParseModelsResponse([]byte(`{"data":[{"id":"mlx-community/Qwen3.5-4B-MLX-4bit","object":"model","context_window":8192}]}`))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "mlx-community/Qwen3.5-4B-MLX-4bit", models[0].Name)
	require.NotNil(t, models[0].Details)
	require.NotNil(t, models[0].Details.MaxContextLength)
	assert.Equal(t, int64(8192), *models[0].Details.MaxContextLength)
}
