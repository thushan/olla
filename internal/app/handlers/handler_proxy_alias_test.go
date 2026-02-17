package handlers

import (
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

	// Registry has no models matching the alias
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{},
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

	// Should fall back to all candidates since alias resolved to no endpoints
	// and standard routing also finds nothing useful
	assert.Len(t, result, 1)
	assert.Contains(t, result, candidates[0])
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
