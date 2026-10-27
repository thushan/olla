package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
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

// Newer Ollama reports vision and tools in /api/tags. A vision model with no vision hint in
// its name must list as vlm, and a older host (no capabilities field) must not change.
func TestUnifiedModelsHandler_OllamaCapabilitiesListVision(t *testing.T) {
	t.Parallel()

	log, _, _ := logger.New(&logger.Config{Level: "error", Theme: "default"})
	styledLogger := logger.NewPlainStyledLogger(log)
	ctx := context.Background()

	ollamaURL, _ := url.Parse("http://ollama-new:11434")
	oldURL, _ := url.Parse("http://ollama-old:11434")
	newHost := &domain.Endpoint{Name: "ollama-new", URL: ollamaURL, URLString: ollamaURL.String(), Status: domain.StatusHealthy, Type: domain.ProfileOllama}
	oldHost := &domain.Endpoint{Name: "ollama-old", URL: oldURL, URLString: oldURL.String(), Status: domain.StatusHealthy, Type: domain.ProfileOllama}

	profileFactory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)
	prof, err := profileFactory.GetProfile(domain.ProfileOllama)
	require.NoError(t, err)

	reg := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	register := func(ep *domain.Endpoint, tags string) {
		models, perr := prof.ParseModelsResponse([]byte(tags))
		require.NoError(t, perr)
		reg.RegisterEndpoint(ep)
		require.NoError(t, reg.RegisterModelsWithEndpoint(ctx, ep, models))
	}
	register(newHost, `{"models":[
		{"name":"qwen3-vl:8b","details":{"family":"qwen3vl"},"capabilities":["completion","vision","tools"]},
		{"name":"gpt-oss:20b","capabilities":["completion","tools"]}]}`)
	register(oldHost, `{"models":[{"name":"mistral-small3.2:24b"}]}`)
	require.Eventually(t, func() bool {
		models, gerr := reg.GetUnifiedModels(ctx)
		return gerr == nil && len(models) == 3
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	app := &Application{
		modelRegistry:    reg,
		repository:       &mockRepository{endpoints: []*domain.Endpoint{newHost, oldHost}},
		converterFactory: converter.NewConverterFactory(),
		logger:           styledLogger,
	}

	list := func(query string) map[string][]string {
		w := httptest.NewRecorder()
		app.unifiedModelsHandler(w, httptest.NewRequest(http.MethodGet, "/olla/models?"+query, nil))
		require.Equal(t, http.StatusOK, w.Code, query)
		var resp converter.UnifiedModelResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		got := make(map[string][]string, len(resp.Data))
		for _, m := range resp.Data {
			require.NotNil(t, m.Olla)
			got[m.ID] = m.Olla.Capabilities
		}
		return got
	}

	for _, query := range []string{"type=vlm", "capability=vision", "capability=multimodal"} {
		got := list(query)
		require.Len(t, got, 1, query)
		assert.Contains(t, got["qwen3-vl:8b"], "vision", query)
		assert.Contains(t, got["qwen3-vl:8b"], "function_calling", query)
		assert.Contains(t, got["qwen3-vl:8b"], "text-generation", query)
	}

	all := list("")
	require.Len(t, all, 3)
	assert.Contains(t, all["gpt-oss:20b"], "function_calling")
	assert.NotContains(t, all["gpt-oss:20b"], "vision")
	assert.NotContains(t, all["mistral-small3.2:24b"], "vision", "a host with no capabilities field is unchanged")
	assert.NotContains(t, all["mistral-small3.2:24b"], "function_calling")
}
