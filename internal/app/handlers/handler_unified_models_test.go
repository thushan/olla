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

	// Wait a bit for async unification
	// TODO: This is a hack, should use proper synchronisation
	<-time.After(100 * time.Millisecond)

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
			assert.Equal(t, tt.expectedModelCount, len(response.Data), tt.description)

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
