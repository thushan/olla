package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// Mock repository for testing
type mockRepository struct {
	endpoints []*domain.Endpoint
}

func (m *mockRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	var healthy []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.GetHealthy(ctx)
}

func (m *mockRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

func (m *mockRepository) LoadFromConfig(ctx context.Context, configs []config.EndpointConfig) error {
	return nil
}

func (m *mockRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	for _, ep := range m.endpoints {
		if ep.URL.String() == endpointURL.String() {
			return true
		}
	}
	return false
}

func TestUnifiedModelsHandler_IncludeUnavailable(t *testing.T) {
	// Setup logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create test endpoints
	healthyURL, _ := url.Parse("http://healthy:11434")
	unhealthyURL, _ := url.Parse("http://unhealthy:11435")

	endpoints := []*domain.Endpoint{
		{
			Name:      "healthy-endpoint",
			URL:       healthyURL,
			URLString: healthyURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "unhealthy-endpoint",
			URL:       unhealthyURL,
			URLString: unhealthyURL.String(),
			Status:    domain.StatusUnhealthy,
			Type:      "ollama",
		},
	}

	// Create registry with test models
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	// Register models on both endpoints
	models := []*domain.ModelInfo{
		{Name: "llama3:latest"},
		{Name: "phi:latest"},
	}

	// Register endpoints first
	unifiedRegistry.RegisterEndpoint(endpoints[0])
	unifiedRegistry.RegisterEndpoint(endpoints[1])

	// Register models for both endpoints
	err := unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0], models)
	require.NoError(t, err)
	err = unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1], models)
	require.NoError(t, err)

	// Wait for async unification (RegisterModels kicks off unifyModelsAsync in
	// a goroutine per endpoint) to complete for BOTH endpoints. Polling on
	// model count alone (len >= 2) is not sufficient: that can be satisfied
	// after only the first endpoint's goroutine has merged in, before the
	// second one adds its own SourceEndpoints entry - which is exactly what
	// the assertions below (len(model.Olla.Availability) >= 2) depend on. Poll
	// on availability/source-endpoint count directly instead.
	require.Eventually(t, func() bool {
		allModels, err := unifiedRegistry.GetUnifiedModels(ctx)
		if err != nil || len(allModels) < 2 {
			return false
		}
		for _, m := range allModels {
			if len(m.SourceEndpoints) < 2 {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond, "async unification did not merge both endpoints' availability")

	// Verify models were registered
	allModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	require.NoError(t, err)
	t.Logf("Total unified models registered: %d", len(allModels))

	// Create converter factory
	converterFactory := converter.NewConverterFactory()

	// Create application with mocks
	app := &Application{
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converterFactory,
		logger:           styledLogger,
	}

	tests := []struct {
		name               string
		query              string
		expectedModelCount int
		description        string
	}{
		{
			name:               "default_filters_unhealthy",
			query:              "",
			expectedModelCount: 2, // Both models, but only showing healthy endpoints
			description:        "Default behavior shows models that have at least one healthy endpoint",
		},
		{
			name:               "include_unavailable_false",
			query:              "?include_unavailable=false",
			expectedModelCount: 2, // Both models, but only showing healthy endpoints
			description:        "Explicitly setting include_unavailable=false shows models with healthy endpoints",
		},
		{
			name:               "include_unavailable_true",
			query:              "?include_unavailable=true",
			expectedModelCount: 2, // Models from both endpoints (but deduplicated)
			description:        "Setting include_unavailable=true should show all models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("GET", "/olla/models"+tt.query, nil)
			w := httptest.NewRecorder()

			// Call handler
			app.unifiedModelsHandler(w, req)

			// Check response
			assert.Equal(t, http.StatusOK, w.Code)

			// Parse response
			var response converter.UnifiedModelResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Debug: print response body
			t.Logf("Response for %s: %s", tt.name, w.Body.String())

			// Check model count
			assert.Len(t, response.Data, tt.expectedModelCount, tt.description)

			// If include_unavailable=true, check that availability info shows endpoint states
			if tt.query == "?include_unavailable=true" {
				for _, model := range response.Data {
					assert.NotNil(t, model.Olla)
					assert.NotEmpty(t, model.Olla.Availability)
					// Should have info from both endpoints
					assert.GreaterOrEqual(t, len(model.Olla.Availability), 2)
				}
			}
		})
	}
}

// newKindFixture registers models through the shipped profile parsers so the
// kinds under test come from real parsed metadata, not hand-built fixtures.
func newKindFixture(t *testing.T) (*Application, *http.ServeMux) {
	t.Helper()
	styled := &mockStyledLogger{}
	profileFactory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	reg := registry.NewUnifiedMemoryModelRegistry(styled, nil, nil, nil)
	ctx := context.Background()
	var endpoints []*domain.Endpoint
	wantUnified := 0
	register := func(name, typ, body string) {
		u, perr := url.Parse("http://" + name + ":8080")
		require.NoError(t, perr)
		ep := &domain.Endpoint{Name: name, URL: u, URLString: u.String(), Type: typ, Status: domain.StatusHealthy}
		endpoints = append(endpoints, ep)
		reg.RegisterEndpoint(ep)
		prof, perr := profileFactory.GetProfile(typ)
		require.NoError(t, perr)
		models, perr := prof.ParseModelsResponse([]byte(body))
		require.NoError(t, perr)
		require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, ep, models))
		wantUnified += len(models)
		// count only: the unifier mutates models in place while it merges
		require.Eventually(t, func() bool {
			unified, gerr := reg.GetUnifiedModels(ctx)
			return gerr == nil && len(unified) >= wantUnified
		}, 5*time.Second, 10*time.Millisecond, "async unification of %s did not complete", name)
	}
	register("ollama-1", domain.ProfileOllama, `{"models":[{"name":"llama3.1:8b","details":{"family":"llama"}},{"name":"all-minilm:latest","details":{"family":"bert"},"capabilities":["embedding"]}]}`)
	register("lms-1", constants.ProviderTypeLMStudio, `{"data":[{"id":"acme/lookout-7b","object":"model","type":"vlm"}]}`)
	// vLLM reports no kind and writes no provider name into the type
	register("vllm-1", domain.ProfileVLLM, `{"data":[{"id":"Qwen/Qwen3-8B","object":"model"}]}`)
	register("sglang-1", domain.ProfileSGLang, `{"data":[{"id":"meta-llama/Llama-3-8B-Instruct","object":"model"}]}`)

	app := &Application{
		modelRegistry:    reg,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converter.NewConverterFactory(),
		logger:           styled,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+"/olla/models/", app.unifiedModelByAliasHandler)
	return app, mux
}

// The kind is reported as olla.type in the listing and as type on the detail
// response, and ?type= selects on that same value.
func TestUnifiedModels_ModelKind(t *testing.T) {
	t.Parallel()
	app, mux := newKindFixture(t)

	want := map[string]string{
		"all-minilm:latest":              "embeddings", // Ollama capabilities
		"acme/lookout-7b":                "vlm",        // LM Studio type
		"llama3.1:8b":                    "llm",        // no kind reported
		"Qwen/Qwen3-8B":                  "llm",        // vLLM, provider name no longer in type
		"meta-llama/Llama-3-8B-Instruct": "llm",        // SGLang, ditto
	}

	t.Run("detail", func(t *testing.T) {
		t.Parallel()
		for id, kind := range want {
			// the detail route only matches a single path segment until slash
			// ids are supported, so slash-bearing ids are covered by the listing
			if strings.Contains(id, "/") {
				continue
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/olla/models/"+id, nil))
			require.Equal(t, http.StatusOK, w.Code, id)

			var summary UnifiedModelSummary
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
			assert.Equal(t, kind, summary.Type, id)
		}
	})

	list := func(t *testing.T, query string) map[string]string {
		t.Helper()
		w := httptest.NewRecorder()
		app.unifiedModelsHandler(w, httptest.NewRequest(http.MethodGet, "/olla/models?include_unavailable=true"+query, nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp converter.UnifiedModelResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		got := make(map[string]string, len(resp.Data))
		for _, m := range resp.Data {
			got[m.ID] = m.Olla.Type
		}
		return got
	}

	t.Run("listing", func(t *testing.T) {
		t.Parallel()
		got := list(t, "")
		for id, kind := range want {
			assert.Equal(t, kind, got[id], id)
		}
	})

	t.Run("filter agrees with reported kind", func(t *testing.T) {
		t.Parallel()
		for _, kind := range []string{"llm", "vlm", "embeddings", "reranker", "classifier"} {
			var wantIDs []string
			for id, k := range want {
				if k == kind {
					wantIDs = append(wantIDs, id)
				}
			}
			got := list(t, "&type="+kind)
			gotIDs := make([]string, 0, len(got))
			for id, k := range got {
				assert.Equal(t, kind, k, "?type=%s listed %s", kind, id)
				gotIDs = append(gotIDs, id)
			}
			assert.ElementsMatch(t, wantIDs, gotIDs, "?type=%s", kind)
		}
	})

	t.Run("type is omitted when no kind can be told", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(UnifiedModelSummary{ID: "x"})
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"type"`)
	})
}
