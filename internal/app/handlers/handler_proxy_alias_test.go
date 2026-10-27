package handlers

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

func TestResolveAliasEndpoints_ResolvesToCorrectEndpoints(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	endpoint2URL, _ := url.Parse("http://lmstudio:1234")
	endpoint3URL, _ := url.Parse("http://llamacpp:8080")

	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "lmstudio",
			URL:       endpoint2URL,
			URLString: "http://lmstudio:1234",
			Type:      domain.ProfileLmStudio,
		},
		{
			Name:      "llamacpp",
			URL:       endpoint3URL,
			URLString: "http://llamacpp:8080",
			Type:      domain.ProfileLlamaCpp,
		},
	}

	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss:120b":     {"http://ollama:11434"},
			"gpt-oss-120b-MLX": {"http://lmstudio:1234"},
			"some-other-model": {"http://llamacpp:8080"},
		},
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b-MLX", "gguf_gpt_oss_120b.gguf"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Should return only ollama and lmstudio (not llamacpp, which doesn't have any aliased model)
	assert.Len(t, result, 2)
	assert.Contains(t, result, candidates[0]) // ollama
	assert.Contains(t, result, candidates[1]) // lmstudio

	// Verify the alias rewrite map was stored in the profile
	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	require.True(t, ok, "alias rewrite map should be stored in profile")

	aliasMap, ok := aliasMapRaw.(map[string]string)
	require.True(t, ok, "alias map should be map[string]string")

	assert.Equal(t, "gpt-oss:120b", aliasMap["http://ollama:11434"])
	assert.Equal(t, "gpt-oss-120b-MLX", aliasMap["http://lmstudio:1234"])
}

// TestResolveAliasEndpoints_NoMatchingEndpoints covers #191: when an alias resolves to
// no endpoints and the standard-routing fallback lookup also rejects (the target model
// exists nowhere in the fleet), the result must be empty. Returning candidates here would
// silently proxy the request to a backend that doesn't serve the requested model, defeating
// strict routing entirely. This is exactly the bug this test used to codify.
func TestResolveAliasEndpoints_NoMatchingEndpoints(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	// Registry has no models matching the alias, and strict mode means the standard-routing
	// fallback lookup rejects rather than handing back all candidates.
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{},
		strict:            true,
	}

	aliases := map[string][]string{
		"nonexistent-alias": {"model-not-in-registry"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "nonexistent-alias"
	profile.SupportedBy = []string{domain.ProfileOllama}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	assert.Empty(t, result, "a rejected fallback lookup must fail fast, not return all candidates")

	require.NotNil(t, profile.RoutingDecision, "rejection decision should still be recorded for headers/metrics")
	assert.Equal(t, "rejected", profile.RoutingDecision.Action)
}

// mockBasicActionModelRegistry mimics MemoryModelRegistry's base GetRoutableEndpointsForModel
// (internal/adapter/registry/memory_registry.go), which reports rejections with action
// "no_model"/"no_healthy" rather than "rejected", but still sets a 4xx/5xx StatusCode.
// Used to prove the alias fail-fast short-circuit is keyed on status code, not the
// "rejected" action string (fix 1 from the #191 follow-up review).
type mockBasicActionModelRegistry struct {
	baseMockRegistry
}

func (m *mockBasicActionModelRegistry) GetRoutableEndpointsForModel(_ context.Context, _ string, _ []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	return []*domain.Endpoint{}, &domain.ModelRoutingDecision{
		Strategy:   "basic",
		Action:     "no_model",
		Reason:     "Model not found in any endpoint",
		StatusCode: http.StatusNotFound,
	}, nil
}

// TestResolveAliasEndpoints_ShortCircuitsOnStatusCodeNotActionString covers fix 1 from
// the #191 follow-up review: the alias fail-fast short-circuit must key on
// decision.StatusCode (matching writeNoRoutableEndpoints' contract), not on
// decision.Action == "rejected". A registry that reports a 404 rejection under a
// different action name (as MemoryModelRegistry's base implementation does) must
// still short-circuit; keying on the action string alone would silently fall through
// and reintroduce the #191 bypass.
func TestResolveAliasEndpoints_ShortCircuitsOnStatusCodeNotActionString(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	// Alias resolves to nothing, so resolveAliasEndpoints falls through to the
	// standard-routing fallback lookup, which is where the short-circuit lives.
	aliases := map[string][]string{
		"nonexistent-alias": {"model-not-in-registry"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: &mockBasicActionModelRegistry{},
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "nonexistent-alias"
	profile.SupportedBy = []string{domain.ProfileOllama}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	assert.Empty(t, result, "a 404/503 decision must fail fast regardless of its action string")
	require.NotNil(t, profile.RoutingDecision)
	assert.Equal(t, "no_model", profile.RoutingDecision.Action, "sanity check: this registry deliberately does not use the \"rejected\" action string")
	assert.Equal(t, http.StatusNotFound, profile.RoutingDecision.StatusCode)
}

// TestResolveAliasEndpoints_NoIntersectionWithCandidates_StrictRejects covers fix 4 from
// the #191 follow-up review, CodeRabbit round 2: the alias resolves to real target models,
// but none of the endpoints serving those models are in the healthy/compatible candidate
// list. Rather than the handler synthesising its own rejection, this must fall through to
// the configured routing strategy for the alias name itself - exactly like the "resolved to
// no endpoints at all" branch - so the strategy's decision (not a hardcoded one) governs the
// outcome. Under a strict/rejecting registry that decision is still a fail-fast rejection,
// just reported as model_not_found/404 (an unknown model name) rather than the
// alias-specific model_unavailable/503 the handler used to synthesise.
func TestResolveAliasEndpoints_NoIntersectionWithCandidates_StrictRejects(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	// Only ollama is a candidate; lmstudio (which actually serves the aliased model) is not.
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	// The alias resolves to a model that only exists on lmstudio, which isn't a candidate.
	// strict:true means the fallback lookup on the alias name itself also rejects, since
	// "gpt-oss-120b" isn't a model the registry knows about either.
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss-120b-MLX": {"http://lmstudio:1234"},
		},
		strict: true,
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss-120b-MLX"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	assert.Empty(t, result, "strict routing on the alias name also rejects, so nothing is routable")

	require.NotNil(t, profile.RoutingDecision, "a decision must be recorded so headers/status are decision-aware, not generic")
	assert.Equal(t, "strict", profile.RoutingDecision.Strategy)
	assert.Equal(t, "rejected", profile.RoutingDecision.Action)
	assert.Equal(t, http.StatusNotFound, profile.RoutingDecision.StatusCode)

	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	assert.False(t, ok, "rejected fallback must not carry an alias rewrite map")
	assert.Nil(t, aliasMapRaw)
}

// TestResolveAliasEndpoints_NoIntersectionWithCandidates_OptimisticFallsBack covers the
// other side of the same fix: under a registry that returns a fallback decision with
// endpoints (optimistic routing, fallback_behavior: all) instead of a rejection, the
// no-intersection branch must let that fallback through rather than unconditionally
// returning empty. This is the behaviour CodeRabbit flagged as missing - previously this
// branch always rejected, so optimistic/all could never recover here. The returned
// endpoints don't serve the alias's actual target models (they're the routing strategy's
// substitute, not an alias match), so the alias rewrite map must NOT be set - the proxy
// has to forward the original request body unchanged.
func TestResolveAliasEndpoints_NoIntersectionWithCandidates_OptimisticFallsBack(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	// The alias resolves to a model that only exists on lmstudio, which isn't a candidate.
	// strict is left false, so mockSimpleModelRegistry's fallback lookup on the alias name
	// mirrors an optimistic/all decision: it hands back the candidates rather than rejecting.
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss-120b-MLX": {"http://lmstudio:1234"},
		},
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss-120b-MLX"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	require.Len(t, result, 1, "the fallback decision hands back the candidate set, so the request still proxies")
	assert.Equal(t, "http://ollama:11434", result[0].URLString)

	require.NotNil(t, profile.RoutingDecision)
	assert.NotEqual(t, "rejected", profile.RoutingDecision.Action)

	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	assert.False(t, ok, "fallback endpoints weren't confirmed to serve the aliased model, so no rewrite map must be set")
	assert.Nil(t, aliasMapRaw)
}

func TestResolveAliasEndpoints_SelfReferencingAlias(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	endpoint2URL, _ := url.Parse("http://lmstudio:1234")

	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "lmstudio",
			URL:       endpoint2URL,
			URLString: "http://lmstudio:1234",
			Type:      domain.ProfileLmStudio,
		},
	}

	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss:120b": {"http://ollama:11434"},
			"gpt-oss-120b": {"http://lmstudio:1234"}, // same name as alias
		},
	}

	// Alias name is also a real model on one backend
	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Both endpoints should be included
	assert.Len(t, result, 2)

	// Verify rewrite map: ollama gets gpt-oss:120b, lmstudio gets gpt-oss-120b (its native name)
	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	require.True(t, ok)

	aliasMap := aliasMapRaw.(map[string]string)
	assert.Equal(t, "gpt-oss:120b", aliasMap["http://ollama:11434"])
	assert.Equal(t, "gpt-oss-120b", aliasMap["http://lmstudio:1234"])
}

func TestResolveAliasEndpoints_OnlyHealthyCandidatesReturned(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	endpoint2URL, _ := url.Parse("http://lmstudio:1234")

	// Only ollama is in the candidate list (healthy)
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	// Both endpoints have the model, but lmstudio is not in candidates (unhealthy)
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss:120b":     {"http://ollama:11434"},
			"gpt-oss-120b-MLX": {"http://lmstudio:1234"},
		},
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b-MLX"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Only the healthy candidate (ollama) should be returned
	assert.Len(t, result, 1)
	assert.Equal(t, "http://ollama:11434", result[0].URLString)

	// lmstudio should NOT be in result despite having a matching model
	_ = endpoint2URL // was used for setup clarity
}

func TestResolveAliasEndpoints_SetsRoutingDecision(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
	}

	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss:120b": {"http://ollama:11434"},
		},
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama}

	_ = app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Should set routing decision
	require.NotNil(t, profile.RoutingDecision, "routing decision should be set")
	assert.Equal(t, "alias", profile.RoutingDecision.Strategy)
	assert.Equal(t, "routed", profile.RoutingDecision.Action)
}

func TestResolveAliasEndpoints_IntegrationWithFilterEndpointsByProfile(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	endpoint2URL, _ := url.Parse("http://lmstudio:1234")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "lmstudio",
			URL:       endpoint2URL,
			URLString: "http://lmstudio:1234",
			Type:      domain.ProfileLmStudio,
		},
	}

	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-oss:120b":     {"http://ollama:11434"},
			"gpt-oss-120b-MLX": {"http://lmstudio:1234"},
		},
	}

	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b-MLX"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	// Test through filterEndpointsByProfile, which is the real entry point
	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-oss-120b"
	profile.SupportedBy = []string{domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.filterEndpointsByProfile(endpoints, profile, styledLog)

	// Both endpoints should be returned via alias resolution
	assert.Len(t, result, 2)

	// Verify the routing went through alias path
	require.NotNil(t, profile.RoutingDecision)
	assert.Equal(t, "alias", profile.RoutingDecision.Strategy)
}

// TestResolveAliasEndpoints_FallbackToSecondTarget tests that when the first
// target model in an alias is not available on any healthy endpoint, the
// second target is tried (config order = priority)
func TestResolveAliasEndpoints_FallbackToSecondTarget(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://ollama:11434")
	endpoint2URL, _ := url.Parse("http://lmstudio:1234")
	endpoint3URL, _ := url.Parse("http://openai:8080")

	// All three endpoints are healthy candidates
	candidates := []*domain.Endpoint{
		{
			Name:      "ollama",
			URL:       endpoint1URL,
			URLString: "http://ollama:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "lmstudio",
			URL:       endpoint2URL,
			URLString: "http://lmstudio:1234",
			Type:      domain.ProfileLmStudio,
		},
		{
			Name:      "openai",
			URL:       endpoint3URL,
			URLString: "http://openai:8080",
			Type:      domain.ProfileOpenAI,
		},
	}

	// Alias has two targets: gpt-3.5-turbo (first priority) and gpt-4 (fallback)
	// Only gpt-4 is available on any endpoint (openai)
	// The alias should resolve to openai endpoint with gpt-4
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-3.5-turbo": {"http://unhealthy:9999"}, // not in candidates (unhealthy)
			"gpt-4":         {"http://openai:8080"},
		},
	}

	aliases := map[string][]string{
		"my-gpt": {"gpt-3.5-turbo", "gpt-4"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "my-gpt"
	profile.SupportedBy = []string{domain.ProfileOpenAI, domain.ProfileOllama, domain.ProfileLmStudio}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Should return only the openai endpoint (which has gpt-4)
	assert.Len(t, result, 1)
	assert.Equal(t, "http://openai:8080", result[0].URLString)

	// Verify the rewrite map uses gpt-4 for the openai endpoint
	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	require.True(t, ok)
	aliasMap := aliasMapRaw.(map[string]string)
	assert.Equal(t, "gpt-4", aliasMap["http://openai:8080"])
}

// TestResolveAliasEndpoints_FallbackToSecondTargetOnSameEndpoint tests fallback
// when both targets are on the same endpoint but first is unavailable
func TestResolveAliasEndpoints_FallbackToSecondTargetOnSameEndpoint(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://openai:8080")

	candidates := []*domain.Endpoint{
		{
			Name:      "openai",
			URL:       endpoint1URL,
			URLString: "http://openai:8080",
			Type:      domain.ProfileOpenAI,
		},
	}

	// Both targets on the same endpoint, but first is "unavailable"
	// The mock registry will only return endpoints for gpt-4
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			// gpt-3.5-turbo not listed = not available
			"gpt-4": {"http://openai:8080"},
		},
	}

	aliases := map[string][]string{
		"my-gpt": {"gpt-3.5-turbo", "gpt-4"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "my-gpt"
	profile.SupportedBy = []string{domain.ProfileOpenAI}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	// Should return the openai endpoint with gpt-4
	assert.Len(t, result, 1)
	assert.Equal(t, "http://openai:8080", result[0].URLString)

	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	require.True(t, ok)
	aliasMap := aliasMapRaw.(map[string]string)
	assert.Equal(t, "gpt-4", aliasMap["http://openai:8080"])
}

// TestResolveAliasEndpoints_TargetOrderMatters tests that config order determines
// priority - first target wins when both are available on the same endpoint
func TestResolveAliasEndpoints_TargetOrderMatters(t *testing.T) {
	styledLog := &mockStyledLogger{}

	endpoint1URL, _ := url.Parse("http://openai:8080")

	candidates := []*domain.Endpoint{
		{
			Name:      "openai",
			URL:       endpoint1URL,
			URLString: "http://openai:8080",
			Type:      domain.ProfileOpenAI,
		},
	}

	// Both targets available on the same endpoint
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-3.5-turbo": {"http://openai:8080"},
			"gpt-4":         {"http://openai:8080"},
		},
	}

	// gpt-3.5-turbo is first in config, so it should win for the endpoint
	aliases := map[string][]string{
		"my-gpt": {"gpt-3.5-turbo", "gpt-4"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        styledLog,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "my-gpt"
	profile.SupportedBy = []string{domain.ProfileOpenAI}

	result := app.resolveAliasEndpoints(t.Context(), profile, candidates, styledLog)

	assert.Len(t, result, 1)
	assert.Equal(t, "http://openai:8080", result[0].URLString)

	// First target in config should win
	aliasMapRaw, ok := profile.InspectionMeta.Load(constants.ContextModelAliasMapKey)
	require.True(t, ok)
	aliasMap := aliasMapRaw.(map[string]string)
	assert.Equal(t, "gpt-3.5-turbo", aliasMap["http://openai:8080"])
}
