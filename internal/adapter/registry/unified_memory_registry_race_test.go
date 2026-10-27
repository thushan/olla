package registry

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// The unified view is read by HTTP handlers without any lock while discovery
// merges, removes and re-registers endpoints. Each scenario overlaps one writer
// with the readers that mattered in production. Under -race they report the
// data race; in the plain test-short run they act as a crash guard for the
// unrecoverable "fatal error: concurrent map iteration and map write".

const registryRaceIterations = 100

func raceEndpoint(name, raw string) *domain.Endpoint {
	u, _ := url.Parse(raw)
	return &domain.Endpoint{Name: name, URL: u, URLString: raw, Type: "ollama"}
}

func raceModel(name, digest string) *domain.ModelInfo {
	return &domain.ModelInfo{Name: name, Size: 100, Details: &domain.ModelDetails{Digest: &digest}}
}

func touchUnified(m *domain.UnifiedModel) int {
	n := 0
	for _, a := range m.Aliases {
		n += len(a.Name)
	}
	for _, s := range m.SourceEndpoints {
		n += len(s.NativeName) + int(s.DiskSize)
	}
	for _, c := range m.Capabilities {
		n += len(c)
	}
	for k := range m.Metadata {
		n += len(k)
	}
	return n + int(m.DiskSize) + int(m.LastSeen.Unix())
}

func readers(ctx context.Context, r *UnifiedMemoryModelRegistry) map[string]func() {
	return map[string]func(){
		"GetUnifiedModelByID": func() {
			if m, err := r.GetUnifiedModel(ctx, "llama3:8b"); err == nil {
				touchUnified(m)
			}
		},
		"GetUnifiedModelByAlias": func() {
			if m, err := r.GetUnifiedModel(ctx, "LLAMA3-8B-B"); err == nil {
				touchUnified(m)
			}
		},
		"GetUnifiedModels": func() {
			ms, _ := r.GetUnifiedModels(ctx)
			for _, m := range ms {
				touchUnified(m)
			}
		},
		"GetModelsByCapability": func() {
			ms, _ := r.GetModelsByCapability(ctx, "chat")
			for _, m := range ms {
				touchUnified(m)
			}
		},
		"GetEndpointsForModel": func() {
			_, _ = r.GetEndpointsForModel(ctx, "LLAMA3-8B-B")
		},
	}
}

// writeWhileReading runs write in a goroutine while the reader spins until it ends.
func writeWhileReading(read, write func()) {
	var done atomic.Bool
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer done.Store(true)
		write()
	}()
	for !done.Load() {
		read()
	}
	<-finished
}

func seedTwoEndpoints(t *testing.T, r *UnifiedMemoryModelRegistry) {
	t.Helper()
	ctx := context.Background()
	// one at a time: the unified ID comes from whichever endpoint unifies first
	require.NoError(t, r.RegisterModelsWithEndpoint(ctx, raceEndpoint("a", "http://a:11434"), []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}))
	r.WaitForUnification()
	require.NoError(t, r.RegisterModelsWithEndpoint(ctx, raceEndpoint("b", "http://b:11434"), []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")}))
	r.WaitForUnification()
}

func TestUnifiedRegistry_ReadersDuringSecondEndpointRegister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	probe := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	for name := range readers(ctx, probe) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for range registryRaceIterations {
				r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
				require.NoError(t, r.RegisterModelsWithEndpoint(ctx, raceEndpoint("a", "http://a:11434"), []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}))
				r.WaitForUnification()

				writeWhileReading(readers(ctx, r)[name], func() {
					_ = r.RegisterModelsWithEndpoint(ctx, raceEndpoint("b", "http://b:11434"), []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")})
					r.WaitForUnification()
				})
			}
		})
	}
}

func TestUnifiedRegistry_ReadersDuringRemoveEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	probe := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	for name := range readers(ctx, probe) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for range registryRaceIterations {
				r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
				seedTwoEndpoints(t, r)

				writeWhileReading(readers(ctx, r)[name], func() {
					_ = r.RemoveEndpoint(ctx, "http://b:11434")
				})
			}
		})
	}
}

func TestUnifiedRegistry_ReadersDuringRediscoveryCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	probe := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	for name := range readers(ctx, probe) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
			seedTwoEndpoints(t, r)

			writeWhileReading(readers(ctx, r)[name], func() {
				for range 300 {
					_ = r.RegisterModelsWithEndpoint(ctx, raceEndpoint("a", "http://a:11434"), []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")})
					r.WaitForUnification()
				}
			})
		})
	}
}

func TestLifecycleRegistry_MarkEndpointOfflineDuringReads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	seedTwoEndpoints(t, r)
	lr := &LifecycleUnifiedRegistry{UnifiedMemoryModelRegistry: r}

	before, err := r.GetUnifiedModel(ctx, "llama3:8b")
	require.NoError(t, err)
	beforeState := before.GetEndpointByURL("http://b:11434").State

	writeWhileReading(readers(ctx, r)["GetUnifiedModels"], func() {
		for range 200 {
			require.NoError(t, lr.MarkEndpointOffline(ctx, "http://b:11434", "down"))
		}
	})

	// the pointer a reader obtained earlier is untouched; the registry moved on
	require.Equal(t, beforeState, before.GetEndpointByURL("http://b:11434").State)
	after, err := r.GetUnifiedModel(ctx, "llama3:8b")
	require.NoError(t, err)
	require.NotSame(t, before, after)
	require.Equal(t, domain.ModelStateOffline, after.GetEndpointByURL("http://b:11434").ModelState)
	require.NotEqual(t, domain.ModelStateOffline, after.GetEndpointByURL("http://a:11434").ModelState)
}

// Alias lookups fall through to the unifier's catalogue, so they must still
// return the registry's current record after MarkEndpointOffline replaces it.
func TestLifecycleRegistry_MarkEndpointOfflineVisibleToAliasLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	seedTwoEndpoints(t, r)
	lr := &LifecycleUnifiedRegistry{UnifiedMemoryModelRegistry: r}
	require.NoError(t, lr.MarkEndpointOffline(ctx, "http://b:11434", "down"))

	for _, name := range []string{"llama3:8b", "LLAMA3:8B"} {
		m, err := r.GetUnifiedModel(ctx, name)
		require.NoError(t, err, name)
		require.Equal(t, domain.ModelStateOffline, m.GetEndpointByURL("http://b:11434").ModelState, name)
	}
}

func TestUnifiedRegistry_RemoveEndpointLeavesHeldModelIntact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	seedTwoEndpoints(t, r)

	held, err := r.GetUnifiedModel(ctx, "llama3:8b")
	require.NoError(t, err)
	require.Len(t, held.SourceEndpoints, 2)

	require.NoError(t, r.RemoveEndpoint(ctx, "http://b:11434"))

	require.Len(t, held.SourceEndpoints, 2, "a model a reader already holds must not change underneath it")
	now, err := r.GetUnifiedModel(ctx, "llama3:8b")
	require.NoError(t, err)
	require.Len(t, now.SourceEndpoints, 1)
	require.Equal(t, "http://a:11434", now.SourceEndpoints[0].EndpointURL)
}

func TestUnifiedRegistry_GetUnifiedStatsDuringRegister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	writeWhileReading(func() { _, _ = r.GetUnifiedStats(ctx) }, func() {
		for range 200 {
			_ = r.RegisterModelsWithEndpoint(ctx, raceEndpoint("a", "http://a:11434"), []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")})
			r.WaitForUnification()
		}
	})
}

// The same digest arriving from two endpoints in alternating order must always list under
// the same id with the model's own id leading the aliases; the merge used to range over maps.
func TestUnifiedRegistry_MergedListingIsStableAcrossRefreshOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
	epA, epB := raceEndpoint("a", "http://a:11434"), raceEndpoint("b", "http://b:11434")
	modelsA := []*domain.ModelInfo{raceModel("llama3:8b", "sha-1")}
	modelsB := []*domain.ModelInfo{raceModel("llama3-8b-b", "sha-1")}

	register := func(ep *domain.Endpoint, m []*domain.ModelInfo) {
		require.NoError(t, r.RegisterModelsWithEndpoint(ctx, ep, m))
		r.WaitForUnification()
	}
	register(epA, modelsA)
	register(epB, modelsB)

	var wantID, wantLead string
	for i := range 200 {
		if i%2 == 0 {
			register(epA, modelsA)
			register(epB, modelsB)
		} else {
			register(epB, modelsB)
			register(epA, modelsA)
		}
		ms, err := r.GetUnifiedModels(ctx)
		require.NoError(t, err)
		require.Len(t, ms, 1, "same digest must collapse to one model")
		m := ms[0]
		require.Len(t, m.SourceEndpoints, 2)
		require.NotEmpty(t, m.Aliases)
		require.Equal(t, m.ID, m.Aliases[0].Name, "listings emit Aliases[0]; it must be the model's own id")
		if i == 0 {
			wantID, wantLead = m.ID, m.Aliases[0].Name
		}
		require.Equal(t, wantID, m.ID)
		require.Equal(t, wantLead, m.Aliases[0].Name)
	}
}
