package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestPrepareProxyContext_InjectsAliasMapWhenPresent(t *testing.T) {
	t.Parallel()

	app := &Application{}

	profile := domain.NewRequestProfile("/v1/messages")
	aliasMap := map[string]string{
		"http://ollama:11434":  "gpt-oss:120b",
		"http://lmstudio:1234": "gpt-oss-120b-MLX",
	}
	profile.SetInspectionMeta(constants.ContextModelAliasMapKey, aliasMap)
	profile.RoutingDecision = &domain.ModelRoutingDecision{
		Strategy: "alias",
		Action:   "routed",
	}

	pr := &proxyRequest{
		model:   "gpt-oss-120b",
		profile: profile,
		stats:   &ports.RequestStats{},
	}

	r := httptest.NewRequest("POST", "/v1/messages", nil)
	ctx := context.Background()

	ctx, r = app.prepareProxyContext(ctx, r, pr)

	// Alias map should be retrievable from the resulting context
	rawMap := r.Context().Value(constants.ContextModelAliasMapKey)
	require.NotNil(t, rawMap, "alias rewrite map should be present in context")

	resultMap, ok := rawMap.(map[string]string)
	require.True(t, ok, "context value should be map[string]string")

	assert.Equal(t, "gpt-oss:120b", resultMap["http://ollama:11434"])
	assert.Equal(t, "gpt-oss-120b-MLX", resultMap["http://lmstudio:1234"])

	// Routing decision should also be propagated to stats
	assert.Equal(t, "alias", pr.stats.RoutingDecision.Strategy)
}

func TestPrepareProxyContext_NoAliasMapWhenNoneStored(t *testing.T) {
	t.Parallel()

	app := &Application{}

	// Standard request — no alias resolved, so no alias map in profile
	profile := domain.NewRequestProfile("/v1/messages")

	pr := &proxyRequest{
		model:   "llama3.1:8b",
		profile: profile,
		stats:   &ports.RequestStats{},
	}

	r := httptest.NewRequest("POST", "/v1/messages", nil)
	ctx := context.Background()

	ctx, r = app.prepareProxyContext(ctx, r, pr)

	// No alias map should be in context for non-alias requests
	rawMap := r.Context().Value(constants.ContextModelAliasMapKey)
	assert.Nil(t, rawMap, "alias rewrite map should not be present for non-alias requests")

	// Model should still be set in context
	assert.Equal(t, "llama3.1:8b", r.Context().Value("model"))
}

func TestPrepareProxyContext_NilProfile(t *testing.T) {
	t.Parallel()

	app := &Application{}

	pr := &proxyRequest{
		model:   "llama3.1:8b",
		profile: nil,
		stats:   &ports.RequestStats{},
	}

	r := httptest.NewRequest("POST", "/v1/messages", nil)
	ctx := context.Background()

	ctx, r = app.prepareProxyContext(ctx, r, pr)

	// Should not panic with nil profile and should still set the model
	rawMap := r.Context().Value(constants.ContextModelAliasMapKey)
	assert.Nil(t, rawMap, "alias rewrite map should not be present when profile is nil")

	assert.Equal(t, "llama3.1:8b", r.Context().Value("model"))
}
