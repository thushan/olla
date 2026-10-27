package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
)

// Every handler that serialises the unified view reads *UnifiedModel pointers
// the registry published without a lock. This loops that route table while
// discovery merges a second endpoint, removes it and re-registers, so a handler
// that reads Aliases, SourceEndpoints or Metadata mid-mutation fails under -race
// (or crashes the run with a map fatal without it).
func TestUnifiedViewHandlers_ConcurrentWithRegistration(t *testing.T) {
	t.Parallel()

	newEndpoint := func(name string) *domain.Endpoint {
		u, err := url.Parse("http://" + name + ":11434")
		require.NoError(t, err)
		return &domain.Endpoint{Name: name, URL: u, URLString: u.String(), Type: domain.ProfileOllama, Status: domain.StatusHealthy}
	}
	model := func(name string) []*domain.ModelInfo {
		digest := "sha256:shared"
		return []*domain.ModelInfo{{Name: name, Size: 100, Details: &domain.ModelDetails{Digest: &digest}}}
	}

	epA, epB := newEndpoint("a"), newEndpoint("b")
	ctx := context.Background()

	for range 30 {
		reg := registry.NewUnifiedMemoryModelRegistry(&mockStyledLogger{}, nil, nil, nil)
		reg.RegisterEndpoint(epA)
		reg.RegisterEndpoint(epB)
		require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, epA, model("llama3:8b")))
		reg.WaitForUnification()

		app := &Application{
			modelRegistry:    reg,
			repository:       &mockRepository{endpoints: []*domain.Endpoint{epA, epB}},
			converterFactory: converter.NewConverterFactory(),
			logger:           &mockStyledLogger{},
		}

		routes := map[string]http.HandlerFunc{
			"/olla/models":                          app.unifiedModelsHandler,
			"/olla/models?include_unavailable=true": app.unifiedModelsHandler,
			"/olla/models/llama3-8b-b":              app.unifiedModelByAliasHandler,
			"/olla/models/llama3:8b":                app.unifiedModelByAliasHandler,
			"/internal/status/models":               app.modelsStatusHandler,
			"/olla/proxy/v1/models":                 app.openaiModelsHandler,
			"/olla/ollama/api/tags":                 app.ollamaModelsHandler,
			"/olla/ollama/v1/models":                app.ollamaOpenAIModelsHandler,
			"/olla/lmstudio/api/v0/models":          app.lmstudioOpenAIModelsHandler,
			"/olla/vllm/v1/models":                  app.genericProviderModelsHandler(domain.ProfileOllama, "openai"),
		}

		var done atomic.Bool
		var readers sync.WaitGroup
		for path, h := range routes {
			readers.Add(1)
			go func() {
				defer readers.Done()
				for !done.Load() {
					req := httptest.NewRequest(http.MethodGet, path, nil)
					req.SetPathValue("id", "llama3:8b")
					w := httptest.NewRecorder()
					h(w, req)
				}
			}()
		}

		// second endpoint merges, then leaves, then the first re-registers
		require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, epB, model("llama3-8b-b")))
		reg.WaitForUnification()
		for range 10 {
			require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, epA, model("llama3:8b")))
			reg.WaitForUnification()
		}
		require.NoError(t, reg.RemoveEndpoint(ctx, epB.URLString))

		done.Store(true)
		readers.Wait()
	}
}
