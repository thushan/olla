package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// Ollama's "tools" is stored as function_calling; asking for either spelling finds it.
func TestGetModelsByCapability_FunctionCallingAliases(t *testing.T) {
	t.Parallel()

	reg := createTestUnifiedRegistry()
	ctx := context.Background()
	declared := []string{"function_calling"}
	require.NoError(t, reg.RegisterModelsWithEndpoint(ctx,
		&domain.Endpoint{Name: "ollama-1", URLString: ollamaURL, Type: domain.ProfileOllama},
		[]*domain.ModelInfo{
			{Name: "gpt-oss:20b", Details: &domain.ModelDetails{Capabilities: declared}},
			{Name: "llama3:8b"},
		}))
	reg.WaitForUnification()

	for _, capability := range []string{"function_calling", "function", "tools"} {
		models, err := reg.GetModelsByCapability(ctx, capability)
		require.NoError(t, err)
		require.Len(t, models, 1, capability)
		assert.Equal(t, "gpt-oss:20b", models[0].ID, capability)
	}
}
