package discovery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestEndpointModelFilterPreservation(t *testing.T) {
	tests := []struct {
		name         string
		endpointCfg  config.EndpointConfig
		expectFilter bool
		filterRules  *domain.FilterConfig
	}{
		{
			name: "endpoint with model filter",
			endpointCfg: config.EndpointConfig{
				Name:           "test-endpoint",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/",
				ModelURL:       "/api/tags",
				ModelFilter: &domain.FilterConfig{
					Exclude: []string{"*embed*", "nomic-*"},
				},
				CheckInterval: 5 * time.Second,
				CheckTimeout:  2 * time.Second,
			},
			expectFilter: true,
			filterRules: &domain.FilterConfig{
				Exclude: []string{"*embed*", "nomic-*"},
			},
		},
		{
			name: "endpoint without model filter",
			endpointCfg: config.EndpointConfig{
				Name:           "test-endpoint-2",
				URL:            "http://localhost:11435",
				HealthCheckURL: "/",
				ModelURL:       "/api/tags",
				ModelFilter:    nil,
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectFilter: false,
			filterRules:  nil,
		},
		{
			name: "endpoint with include filter",
			endpointCfg: config.EndpointConfig{
				Name:           "test-endpoint-3",
				URL:            "http://localhost:11436",
				HealthCheckURL: "/",
				ModelURL:       "/api/tags",
				ModelFilter: &domain.FilterConfig{
					Include: []string{"llama*", "mistral*"},
				},
				CheckInterval: 5 * time.Second,
				CheckTimeout:  2 * time.Second,
			},
			expectFilter: true,
			filterRules: &domain.FilterConfig{
				Include: []string{"llama*", "mistral*"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := NewStaticEndpointRepository()

			// Load config
			err := repo.LoadFromConfig(ctx, []config.EndpointConfig{tt.endpointCfg})
			require.NoError(t, err)

			// Get the endpoint
			endpoints, err := repo.GetAll(ctx)
			require.NoError(t, err)
			require.Len(t, endpoints, 1)

			endpoint := endpoints[0]

			// Verify filter preservation
			if tt.expectFilter {
				assert.NotNil(t, endpoint.ModelFilter, "ModelFilter should be preserved")
				assert.Equal(t, tt.filterRules.Include, endpoint.ModelFilter.Include)
				assert.Equal(t, tt.filterRules.Exclude, endpoint.ModelFilter.Exclude)
			} else {
				assert.Nil(t, endpoint.ModelFilter, "ModelFilter should be nil")
			}
		})
	}
}

func TestDiscoveryServiceEndpointFiltering(t *testing.T) {
	tests := []struct {
		name             string
		endpoint         *domain.Endpoint
		discoveredModels []*domain.ModelInfo
		expectedModels   []string
	}{
		{
			name: "filter excludes embed models",
			endpoint: &domain.Endpoint{
				Name:      "test-endpoint",
				URLString: "http://localhost:11434",
				ModelFilter: &domain.FilterConfig{
					Exclude: []string{"*embed*", "bge-*"},
				},
			},
			discoveredModels: []*domain.ModelInfo{
				{Name: "llama3-8b"},
				{Name: "nomic-embed-text"},
				{Name: "bge-large"},
				{Name: "mistral-7b"},
				{Name: "text-embedding-ada"},
			},
			expectedModels: []string{"llama3-8b", "mistral-7b"},
		},
		{
			name: "filter includes only specific models",
			endpoint: &domain.Endpoint{
				Name:      "test-endpoint-2",
				URLString: "http://localhost:11435",
				ModelFilter: &domain.FilterConfig{
					Include: []string{"llama*", "mistral*"},
				},
			},
			discoveredModels: []*domain.ModelInfo{
				{Name: "llama3-8b"},
				{Name: "qwen2-7b"},
				{Name: "mistral-7b"},
				{Name: "deepseek-coder"},
				{Name: "llama2-13b"},
			},
			expectedModels: []string{"llama3-8b", "mistral-7b", "llama2-13b"},
		},
		{
			name: "no filter returns all models",
			endpoint: &domain.Endpoint{
				Name:        "test-endpoint-3",
				URLString:   "http://localhost:11436",
				ModelFilter: nil,
			},
			discoveredModels: []*domain.ModelInfo{
				{Name: "model1"},
				{Name: "model2"},
				{Name: "model3"},
			},
			expectedModels: []string{"model1", "model2", "model3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock components using a modified mockDiscoveryClient
			discoveredModels := tt.discoveredModels
			client := &mockDiscoveryClientWithModels{
				models: discoveredModels,
			}

			endpointRepo := &mockEndpointRepository{
				healthyEndpoints: []*domain.Endpoint{tt.endpoint},
			}

			registeredModels := make(map[string][]*domain.ModelInfo)
			modelRegistry := &mockModelRegistryWithCapture{
				registeredModels: registeredModels,
			}

			config := DiscoveryConfig{
				Interval:          30 * time.Second,
				Timeout:           10 * time.Second,
				ConcurrentWorkers: 1,
			}

			// Create discovery service with test logger
			slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelError,
			}))
			testLogger := logger.NewPlainStyledLogger(slogLogger)

			service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, testLogger)

			// Discover models for the endpoint
			err := service.DiscoverEndpoint(ctx, tt.endpoint)
			require.NoError(t, err)

			// Verify the correct models were registered
			registered, exists := registeredModels[tt.endpoint.URLString]
			require.True(t, exists, "Models should be registered for endpoint")

			// Extract model names
			var registeredNames []string
			for _, model := range registered {
				registeredNames = append(registeredNames, model.Name)
			}

			// Verify filtering worked correctly
			assert.ElementsMatch(t, tt.expectedModels, registeredNames,
				"Registered models should match expected after filtering")
		})
	}
}

func TestGetEndpointFilterConfig(t *testing.T) {
	config := DiscoveryConfig{
		Interval: 30 * time.Second,
		Timeout:  10 * time.Second,
	}

	tests := []struct {
		name               string
		endpoint           *domain.Endpoint
		setFilterOverrides map[string]*domain.FilterConfig
		expectedFilter     *domain.FilterConfig
	}{
		{
			name: "use endpoint's own filter when no override",
			endpoint: &domain.Endpoint{
				Name:      "test-endpoint",
				URLString: "http://localhost:11434",
				ModelFilter: &domain.FilterConfig{
					Exclude: []string{"*embed*"},
				},
			},
			setFilterOverrides: nil,
			expectedFilter: &domain.FilterConfig{
				Exclude: []string{"*embed*"},
			},
		},
		{
			name: "override takes precedence over endpoint filter",
			endpoint: &domain.Endpoint{
				Name:      "test-endpoint",
				URLString: "http://localhost:11434",
				ModelFilter: &domain.FilterConfig{
					Exclude: []string{"*embed*"},
				},
			},
			setFilterOverrides: map[string]*domain.FilterConfig{
				"test-endpoint": {
					Include: []string{"llama*"},
				},
			},
			expectedFilter: &domain.FilterConfig{
				Include: []string{"llama*"},
			},
		},
		{
			name: "no filter when endpoint has none and no override",
			endpoint: &domain.Endpoint{
				Name:        "test-endpoint",
				URLString:   "http://localhost:11434",
				ModelFilter: nil,
			},
			setFilterOverrides: nil,
			expectedFilter:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test logger
			slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelError,
			}))
			testLogger := logger.NewPlainStyledLogger(slogLogger)

			// Create discovery service
			service := NewModelDiscoveryService(
				&mockDiscoveryClient{},
				&mockEndpointRepository{},
				&mockModelRegistry{},
				config,
				testLogger,
			)

			// Set filter overrides if any
			for name, filter := range tt.setFilterOverrides {
				service.SetEndpointFilterConfig(name, filter)
			}

			// Get filter config
			filterConfig := service.getEndpointFilterConfig(tt.endpoint)

			// Verify result
			if tt.expectedFilter == nil {
				assert.Nil(t, filterConfig)
			} else {
				require.NotNil(t, filterConfig)
				assert.Equal(t, tt.expectedFilter.Include, filterConfig.Include)
				assert.Equal(t, tt.expectedFilter.Exclude, filterConfig.Exclude)
			}
		})
	}
}

// Extended mock for testing with specific models
type mockDiscoveryClientWithModels struct {
	models []*domain.ModelInfo
	mu     sync.Mutex
}

func (m *mockDiscoveryClientWithModels) DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.models == nil {
		return nil, errors.New("no models configured")
	}

	return m.models, nil
}

func (m *mockDiscoveryClientWithModels) HealthCheck(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

func (m *mockDiscoveryClientWithModels) GetMetrics() DiscoveryMetrics {
	return DiscoveryMetrics{}
}

// Extended mock for capturing registered models
type mockModelRegistryWithCapture struct {
	registeredModels map[string][]*domain.ModelInfo
	mu               sync.Mutex
}

func (m *mockModelRegistryWithCapture) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}

func (m *mockModelRegistryWithCapture) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.registeredModels[endpointURL] = models
	return nil
}

func (m *mockModelRegistryWithCapture) GetModel(ctx context.Context, modelName string) (*domain.UnifiedModel, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) GetModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) RemoveEndpointModels(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *mockModelRegistryWithCapture) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	return healthyEndpoints, &domain.ModelRoutingDecision{
		Strategy: "mock",
		Action:   "routed",
		Reason:   "mock routing",
	}, nil
}

func (m *mockModelRegistryWithCapture) HasModel(ctx context.Context, modelName string) bool {
	return false
}

func (m *mockModelRegistryWithCapture) GetModelStatistics(ctx context.Context) map[string]interface{} {
	return nil
}

func (m *mockModelRegistryWithCapture) GetEndpointModels(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) AddObserver(observer func(event string, data interface{})) {}

func (m *mockModelRegistryWithCapture) NotifyObservers(event string, data interface{}) {}

func (m *mockModelRegistryWithCapture) StartCleanup(ctx context.Context, interval time.Duration) {}

func (m *mockModelRegistryWithCapture) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) IsModelAvailable(ctx context.Context, modelName string) bool {
	return false
}

func (m *mockModelRegistryWithCapture) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *mockModelRegistryWithCapture) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}

func (m *mockModelRegistryWithCapture) ModelsToStrings(models []*domain.ModelInfo) []string {
	return nil
}

func (m *mockModelRegistryWithCapture) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}

func (m *mockModelRegistryWithCapture) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

func (m *mockModelRegistryWithCapture) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	return []*domain.UnifiedModel{}, nil
}
