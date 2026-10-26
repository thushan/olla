package unifier

import (
	"context"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// The catalog store hands out live pointers that lock-free readers keep using
// while discovery merges, so these tests overlap a mutating UnifyModels with
// readers of the same model. They are only meaningful under -race (or, without
// it, as a crash guard for "concurrent map iteration and map write").

const raceIterations = 100

func raceEndpoint(name, raw string) *domain.Endpoint {
	u, _ := url.Parse(raw)
	return &domain.Endpoint{Name: name, URL: u, URLString: raw, Type: "ollama"}
}

func raceModel(name, digest string) *domain.ModelInfo {
	return &domain.ModelInfo{Name: name, Size: 100, Details: &domain.ModelDetails{Digest: &digest}}
}

// touchModel reads every slice and ranges the Metadata map the way handlers and
// converters do.
func touchModel(m *domain.UnifiedModel) int {
	n := 0
	for _, a := range m.Aliases {
		n += len(a.Name)
	}
	for _, s := range m.SourceEndpoints {
		n += len(s.NativeName) + int(s.DiskSize)
		if s.StateInfo != nil {
			n += len(s.StateInfo.LastError)
		}
	}
	for _, c := range m.Capabilities {
		n += len(c)
	}
	for k, v := range m.Metadata {
		n += len(k)
		if p, ok := v.(map[string]bool); ok {
			for pk := range p {
				n += len(pk)
			}
		}
	}
	return n + int(m.DiskSize) + int(m.LastSeen.Unix())
}

func TestDefaultUnifier_ConcurrentReadersDuringMerge(t *testing.T) {
	t.Parallel()

	readers := map[string]func(ctx context.Context, u *DefaultUnifier){
		"ResolveAlias": func(ctx context.Context, u *DefaultUnifier) {
			if m, err := u.ResolveAlias(ctx, "llama3:8b"); err == nil {
				touchModel(m)
			}
		},
		"ResolveModelByMergedAlias": func(ctx context.Context, u *DefaultUnifier) {
			if m, err := u.ResolveModel(ctx, "LLAMA3-8B-B"); err == nil {
				touchModel(m)
			}
		},
		"GetAllModels": func(ctx context.Context, u *DefaultUnifier) {
			ms, _ := u.GetAllModels(ctx)
			for _, m := range ms {
				touchModel(m)
			}
		},
		"GetAliases": func(ctx context.Context, u *DefaultUnifier) {
			_, _ = u.GetAliases(ctx, "llama3:8b")
		},
	}

	scenarios := map[string]func(ctx context.Context, u *DefaultUnifier){
		// second endpoint, same digest, different native name: mergeModel appends
		"SecondEndpointMerge": func(ctx context.Context, u *DefaultUnifier) {
			_, _ = u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")}, raceEndpoint("b", "http://b:11434"))
		},
		// steady-state discovery cycle: removeModelFromEndpoint then mergeModel
		"RediscoverCycle": func(ctx context.Context, u *DefaultUnifier) {
			_, _ = u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}, raceEndpoint("a", "http://a:11434"))
		},
	}

	for sName, scenario := range scenarios {
		for rName, read := range readers {
			t.Run(sName+"/"+rName, func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				for i := 0; i < raceIterations; i++ {
					u := NewDefaultUnifier().(*DefaultUnifier)
					_, err := u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}, raceEndpoint("a", "http://a:11434"))
					require.NoError(t, err)
					_, err = u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")}, raceEndpoint("b", "http://b:11434"))
					require.NoError(t, err)

					var done atomic.Bool
					var wg sync.WaitGroup
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer done.Store(true)
						scenario(ctx, u)
					}()
					for !done.Load() {
						read(ctx, u)
					}
					wg.Wait()
				}
			})
		}
	}
}

func TestDefaultUnifier_GetStatsDuringUnify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	u := NewDefaultUnifier().(*DefaultUnifier)
	ep := raceEndpoint("a", "http://a:11434")

	var done atomic.Bool
	go func() {
		defer done.Store(true)
		for i := 0; i < 300; i++ {
			_, _ = u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("m", "d")}, ep)
		}
	}()
	for !done.Load() {
		_ = u.GetStats()
	}
}

// Previously returned pointers are the published immutable view. Any later
// unification must leave them byte-for-byte unchanged, which this asserts
// without needing the race detector or lucky timing.
func TestDefaultUnifier_PublishedModelsAreImmutable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	u := NewDefaultUnifier().(*DefaultUnifier)

	_, err := u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}, raceEndpoint("a", "http://a:11434"))
	require.NoError(t, err)

	byID, ok := u.store.GetModel("llama3:8b")
	require.True(t, ok)
	byName, ok := u.store.ResolveByName("llama3:8b")
	require.True(t, ok)
	all := u.store.GetAllModels()
	require.Len(t, all, 1)
	byDigest, ok := u.store.ResolveByDigest("sha-1")
	require.True(t, ok)

	held := map[string]*domain.UnifiedModel{
		"GetModel": byID, "ResolveByName": byName, "GetAllModels": all[0], "ResolveByDigest": byDigest[0],
	}
	snapshots := make(map[string]*domain.UnifiedModel, len(held))
	for k, m := range held {
		snapshots[k] = m.Clone()
	}

	steps := []struct {
		name string
		run  func()
	}{
		{"second endpoint merges", func() {
			_, err := u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")}, raceEndpoint("b", "http://b:11434"))
			require.NoError(t, err)
		}},
		{"same endpoint rediscovers", func() {
			_, err := u.UnifyModels(ctx, []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}, raceEndpoint("a", "http://a:11434"))
			require.NoError(t, err)
		}},
		{"endpoint drops the model", func() {
			_, err := u.UnifyModels(ctx, []*domain.ModelInfo{}, raceEndpoint("b", "http://b:11434"))
			require.NoError(t, err)
		}},
	}
	for _, step := range steps {
		step.run()
		for k, m := range held {
			require.Truef(t, reflect.DeepEqual(snapshots[k], m), "%s pointer was mutated after %q", k, step.name)
		}
	}

	// and the store did move on, so the check above is not vacuous
	current, ok := u.store.GetModel("llama3:8b")
	require.True(t, ok)
	require.NotSame(t, byID, current)
}

func TestUnifiedModelClone_NestedMetadataIsIndependent(t *testing.T) {
	t.Parallel()

	orig := &domain.UnifiedModel{
		ID: "m",
		Metadata: map[string]interface{}{
			"platforms": map[string]bool{"ollama": true},
			"nested":    map[string]interface{}{"k": []interface{}{"v"}},
		},
		SourceEndpoints: []domain.SourceEndpoint{{
			EndpointURL: "http://a",
			StateInfo:   &domain.EndpointStateInfo{Metadata: map[string]interface{}{"x": "y"}},
		}},
	}
	c := orig.Clone()

	c.Metadata["platforms"].(map[string]bool)["lmstudio"] = true
	c.Metadata["nested"].(map[string]interface{})["k"].([]interface{})[0] = "changed"
	c.SourceEndpoints[0].StateInfo.Metadata["x"] = "z"
	c.SourceEndpoints[0].StateInfo.LastError = "boom"

	require.Len(t, orig.Metadata["platforms"], 1)
	require.Equal(t, "v", orig.Metadata["nested"].(map[string]interface{})["k"].([]interface{})[0])
	require.Equal(t, "y", orig.SourceEndpoints[0].StateInfo.Metadata["x"])
	require.Empty(t, orig.SourceEndpoints[0].StateInfo.LastError)
}
