package registry

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// Ollama lists a default-tag pull as "name:latest" but clients ask for "name".
// These tests pin the rule: the fallback is exact-first, Ollama-sourced only, and
// never rewrites anything stored.

const (
	ollamaURL  = "http://ollama-1:11434"
	ollama2URL = "http://ollama-2:11434"
	vllmURL    = "http://vllm-1:8000"
)

type tagFixture struct {
	url    string
	name   string
	typ    string
	models []string
}

func newEndpoint(f tagFixture) *domain.Endpoint {
	return &domain.Endpoint{Name: f.name, URLString: f.url, Type: f.typ, Status: domain.StatusHealthy}
}

func modelInfos(names []string) []*domain.ModelInfo {
	out := make([]*domain.ModelInfo, 0, len(names))
	for _, n := range names {
		out = append(out, &domain.ModelInfo{Name: n})
	}
	return out
}

func buildUnified(t *testing.T, fleet ...tagFixture) *UnifiedMemoryModelRegistry {
	t.Helper()
	reg := createTestUnifiedRegistry()
	ctx := context.Background()
	for _, f := range fleet {
		require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, newEndpoint(f), modelInfos(f.models)))
	}
	// Unification is asynchronous: wait until every native name resolves.
	require.Eventually(t, func() bool {
		for _, f := range fleet {
			for _, n := range f.models {
				if u, err := reg.GetUnifiedModel(ctx, n); err != nil || u == nil {
					return false
				}
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)
	return reg
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestUnifiedRegistry_OllamaDefaultTag(t *testing.T) {
	t.Parallel()

	ollama := func(models ...string) tagFixture {
		return tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: models}
	}
	vllm := func(models ...string) tagFixture {
		return tagFixture{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: models}
	}

	tests := []struct {
		name  string
		query string
		want  []string
		fleet []tagFixture
	}{
		{"bare name resolves to ollama latest", "all-minilm", []string{ollamaURL}, []tagFixture{ollama("all-minilm:latest")}},
		{"tagged name is unchanged", "all-minilm:latest", []string{ollamaURL}, []tagFixture{ollama("all-minilm:latest")}},
		{"case differences still resolve through the catalogue", "ALL-MINILM", []string{ollamaURL}, []tagFixture{ollama("all-minilm:latest")}},
		{"a bare name does not reach a non-latest tag", "qwen2.5", nil, []tagFixture{ollama("qwen2.5:0.5b")}},
		{"unknown stays unknown", "nonexistent-xyz", nil, []tagFixture{ollama("all-minilm:latest")}},
		{"vllm-only foo:latest does not answer to foo", "foo", nil, []tagFixture{vllm("foo:latest")}},
		{"vllm bare foo does not answer to foo:latest", "foo:latest", nil, []tagFixture{vllm("foo")}},
		{"exact vllm foo wins over ollama foo:latest", "foo", []string{vllmURL}, []tagFixture{vllm("foo"), ollama("foo:latest")}},
		{"ollama foo:latest still exact beside vllm foo", "foo:latest", []string{ollamaURL}, []tagFixture{vllm("foo"), ollama("foo:latest")}},
		{"shared foo:latest id answers a bare name from ollama only", "foo", []string{ollamaURL}, []tagFixture{vllm("foo:latest"), ollama("foo:latest")}},
		{"shared foo:latest id stays on both when asked exactly", "foo:latest", []string{ollamaURL, vllmURL}, []tagFixture{vllm("foo:latest"), ollama("foo:latest")}},
		{"registry host with a port is not a tag", "registry.example.com:5000/team/foo", []string{ollamaURL}, []tagFixture{ollama("registry.example.com:5000/team/foo:latest")}},
		{
			"two ollama hosts both answer", "foo", []string{ollamaURL, ollama2URL},
			[]tagFixture{ollama("foo:latest"), {url: ollama2URL, name: "ollama-2", typ: domain.ProfileOllama, models: []string{"foo:latest"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := buildUnified(t, tt.fleet...)
			ctx := context.Background()

			got, err := reg.GetEndpointsForModel(ctx, tt.query)
			require.NoError(t, err)
			assert.Equal(t, sorted(tt.want), sorted(got))
			assert.Equal(t, len(tt.want) > 0, reg.IsModelAvailable(ctx, tt.query))
		})
	}
}

func TestUnifiedRegistry_OllamaDefaultTag_GetUnifiedModel(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t,
		tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"all-minilm:latest"}},
		tagFixture{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: []string{"only-vllm:latest"}},
	)
	ctx := context.Background()

	m, err := reg.GetUnifiedModel(ctx, "all-minilm")
	require.NoError(t, err)
	assert.Equal(t, "all-minilm:latest", m.ID, "the unified ID keeps the native name")

	_, err = reg.GetUnifiedModel(ctx, "only-vllm")
	assert.Error(t, err, "a non-ollama latest tag must not answer to the bare name")
}

// Ollama lists "FOO:latest" and vLLM lists "foo:latest". The unifier merges them
// case-insensitively, but only the Ollama host answers to the bare "foo".
func TestUnifiedRegistry_OllamaDefaultTag_MixedCaseKeepsToOllama(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t,
		tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"FOO:latest"}},
		tagFixture{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: []string{"foo:latest"}},
	)
	ctx := context.Background()

	got, err := reg.GetEndpointsForModel(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, []string{ollamaURL}, got, "the bare name must not reach the vLLM host")

}

// The inspector lowercases names, so "Qwen/Foo" reaches the registry as written or
// lowered. vLLM's existing case-insensitive catalogue match must beat the Ollama
// default-tag fallback.
func TestUnifiedRegistry_OllamaDefaultTag_ExactCatalogueMatchBeatsFallback(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t,
		tagFixture{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: []string{"Qwen/Foo"}},
		tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"qwen/foo:latest"}},
	)
	ctx := context.Background()

	for _, name := range []string{"Qwen/Foo", "qwen/foo"} {
		got, err := reg.GetEndpointsForModel(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, []string{vllmURL}, got, name)
		assert.Equal(t, name, reg.CanonicalModelName(name), "an exact match keeps its spelling")
	}
}

// An alias target in a different case still canonicalises to the Ollama native name.
func TestUnifiedRegistry_OllamaDefaultTag_MixedCaseCanonicalName(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t,
		tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"all-minilm:latest"}},
	)

	assert.Equal(t, "all-minilm:latest", reg.CanonicalModelName("ALL-MINILM"))
	assert.Equal(t, "all-minilm:latest", reg.CanonicalModelName("all-minilm"))
}

func TestUnifiedRegistry_OllamaDefaultTag_CanonicalName(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t,
		tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"all-minilm:latest", "foo:latest"}},
		tagFixture{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: []string{"foo", "bar:latest"}},
	)

	assert.Equal(t, "all-minilm:latest", reg.CanonicalModelName("all-minilm"), "bare name resolves to the native name")
	assert.Equal(t, "all-minilm:latest", reg.CanonicalModelName("all-minilm:latest"))
	assert.Equal(t, "foo", reg.CanonicalModelName("foo"), "an exact match wins, so the spelling is kept")
	assert.Equal(t, "bar", reg.CanonicalModelName("bar"), "a non-ollama latest tag does not canonicalise")
	assert.Equal(t, "unknown", reg.CanonicalModelName("unknown"))
	assert.Empty(t, reg.CanonicalModelName(""))
}

// With unification disabled the base registry serves lookups, so it needs the
// same rule or the fix would only hold for one configuration.
func TestBaseRegistries_OllamaDefaultTag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	build := func(t *testing.T) (*MemoryModelRegistry, *RoutingRegistry) {
		t.Helper()
		base := NewMemoryModelRegistry(createTestLogger())
		routing := NewRoutingRegistry(NewMemoryModelRegistry(createTestLogger()), nil, nil, createTestLogger())
		for _, f := range []tagFixture{
			{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"all-minilm:latest", "foo:latest"}},
			{url: vllmURL, name: "vllm-1", typ: domain.ProfileVLLM, models: []string{"foo", "bar:latest"}},
		} {
			require.NoError(t, base.RegisterModelsWithEndpoint(ctx, newEndpoint(f), modelInfos(f.models)))
			require.NoError(t, routing.RegisterModelsWithEndpoint(ctx, newEndpoint(f), modelInfos(f.models)))
		}
		return base, routing
	}

	base, routing := build(t)
	for name, reg := range map[string]domain.ModelRegistry{"memory": base, "routing": routing} {
		t.Run(name, func(t *testing.T) {
			got, err := reg.GetEndpointsForModel(ctx, "all-minilm")
			require.NoError(t, err)
			assert.Equal(t, []string{ollamaURL}, got)
			assert.True(t, reg.IsModelAvailable(ctx, "all-minilm"))

			got, err = reg.GetEndpointsForModel(ctx, "foo")
			require.NoError(t, err)
			assert.Equal(t, []string{vllmURL}, got, "exact vllm foo wins")

			got, err = reg.GetEndpointsForModel(ctx, "bar")
			require.NoError(t, err)
			assert.Empty(t, got, "vllm bar:latest is not ollama-sourced")
			assert.False(t, reg.IsModelAvailable(ctx, "bar"))
		})
	}

	assert.Equal(t, "all-minilm:latest", base.CanonicalModelName("all-minilm"))
	assert.Equal(t, "foo", base.CanonicalModelName("foo"))
	assert.Equal(t, "all-minilm:latest", routing.CanonicalModelName("all-minilm"))
}

// A bare name that stopped resolving when the host dropped the model must not be served from a stale cache.
func TestUnifiedRegistry_OllamaDefaultTag_DroppedWithEndpoint(t *testing.T) {
	t.Parallel()

	reg := buildUnified(t, tagFixture{url: ollamaURL, name: "ollama-1", typ: domain.ProfileOllama, models: []string{"all-minilm:latest"}})
	ctx := context.Background()
	require.NoError(t, reg.RemoveEndpoint(ctx, ollamaURL))

	got, err := reg.GetEndpointsForModel(ctx, "all-minilm")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, "all-minilm", reg.CanonicalModelName("all-minilm"))
}
