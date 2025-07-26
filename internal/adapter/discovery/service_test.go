package discovery

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

func TestModelDiscoveryServiceStartStop(t *testing.T) {
	client := &mockDiscoveryClient{}
	endpointRepo := &mockEndpointRepository{}
	modelRegistry := domain.ModelRegistry(&mockModelRegistry{})
	config := DiscoveryConfig{
		Interval:          100 * time.Millisecond,
		Timeout:           5 * time.Second,
		ConcurrentWorkers: 2,
		RetryAttempts:     3,
		RetryBackoff:      time.Millisecond,
	}

	service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	if !service.isRunning.Load() {
		t.Errorf("Service should be running after start")
	}

	err = service.Start(ctx)
	if err == nil {
		t.Errorf("Expected error when starting already running service")
	}

	err = service.Stop(ctx)
	if err != nil {
		t.Errorf("Failed to stop service: %v", err)
	}

	// Give time for goroutines to clean up
	time.Sleep(10 * time.Millisecond)

	if service.isRunning.Load() {
		t.Errorf("Service should be stopped after stop")
	}

	// we should protected against double stops
	err = service.Stop(ctx)
	if err != nil {
		t.Errorf("Double stop should not error: %v", err)
	}
}

func TestDiscoverAll(t *testing.T) {
	tests := []struct {
		name                string
		healthyEndpoints    []*domain.Endpoint
		repositoryError     error
		discoveryErrors     map[string]error
		expectedDiscoveries int
		expectedError       bool
	}{
		{
			name: "successful discovery from multiple endpoints",
			healthyEndpoints: []*domain.Endpoint{
				createMockEndpoint("http://localhost:11434", "ollama-1"),
				createMockEndpoint("http://localhost:1234", "lm-studio-1"),
			},
			discoveryErrors:     map[string]error{},
			expectedDiscoveries: 2,
			expectedError:       false,
		},
		{
			name:                "no healthy endpoints",
			healthyEndpoints:    []*domain.Endpoint{},
			discoveryErrors:     map[string]error{},
			expectedDiscoveries: 0,
			expectedError:       false,
		},
		{
			name:            "repository error",
			repositoryError: errors.New("repository error"),
			expectedError:   true,
		},
		{
			name: "partial discovery failures",
			healthyEndpoints: []*domain.Endpoint{
				createMockEndpoint("http://localhost:11434", "ollama-1"),
				createMockEndpoint("http://localhost:1234", "lm-studio-1"),
			},
			discoveryErrors: map[string]error{
				"http://localhost:11434": errors.New("discovery failed"),
			},
			expectedDiscoveries: 2, // Both attempted, one fails
			expectedError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockDiscoveryClient{
				discoveryErrors: tt.discoveryErrors,
			}
			endpointRepo := &mockEndpointRepository{
				healthyEndpoints: tt.healthyEndpoints,
				getHealthyError:  tt.repositoryError,
			}
			modelRegistry := &mockModelRegistry{}
			config := DiscoveryConfig{
				Timeout:           5 * time.Second,
				ConcurrentWorkers: 2,
			}

			service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

			err := service.DiscoverAll(context.Background())

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if client.discoverCallCount != tt.expectedDiscoveries {
				t.Errorf("Expected %d discovery calls, got %d", tt.expectedDiscoveries, client.discoverCallCount)
			}
		})
	}
}

func TestDiscoverEndpoint(t *testing.T) {
	tests := []struct {
		name                 string
		endpoint             *domain.Endpoint
		discoveryError       error
		registryError        error
		expectedModelCount   int
		expectedError        bool
		expectRegistryCalled bool
	}{
		{
			name:                 "successful discovery and registration",
			endpoint:             createMockEndpoint("http://localhost:11434", "ollama-1"),
			discoveryError:       nil,
			registryError:        nil,
			expectedModelCount:   2,
			expectedError:        false,
			expectRegistryCalled: true,
		},
		{
			name:                 "discovery failure",
			endpoint:             createMockEndpoint("http://localhost:11434", "ollama-1"),
			discoveryError:       errors.New("network error"),
			expectedError:        true,
			expectRegistryCalled: false,
		},
		{
			name:                 "registry failure",
			endpoint:             createMockEndpoint("http://localhost:11434", "ollama-1"),
			discoveryError:       nil,
			registryError:        errors.New("registry error"),
			expectedError:        true,
			expectRegistryCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoveryErrors := make(map[string]error)
			if tt.discoveryError != nil {
				discoveryErrors[tt.endpoint.URLString] = tt.discoveryError
			}

			client := &mockDiscoveryClient{
				discoveryErrors: discoveryErrors,
			}
			endpointRepo := &mockEndpointRepository{}
			modelRegistry := &mockModelRegistry{
				registeredModels: make([]*domain.ModelInfo, 0),
				registerError:    tt.registryError,
			}
			config := DiscoveryConfig{
				Timeout: 5 * time.Second,
			}

			service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

			err := service.DiscoverEndpoint(context.Background(), tt.endpoint)

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expectRegistryCalled && modelRegistry.registerCallCount == 0 {
				t.Errorf("Expected registry to be called but it wasn't")
			}
			if !tt.expectRegistryCalled && modelRegistry.registerCallCount > 0 {
				t.Errorf("Expected registry not to be called but it was")
			}

			// check that the right number of models registered considering it seems successful
			if !tt.expectedError && tt.expectRegistryCalled && len(modelRegistry.registeredModels) != tt.expectedModelCount {
				t.Errorf("Expected %d models registered, got %d (call count: %d, actual models: %v)",
					tt.expectedModelCount, len(modelRegistry.registeredModels), modelRegistry.registerCallCount, modelRegistry.registeredModels)
			}
		})
	}
}

func TestEndpointDisabling(t *testing.T) {
	tests := []struct {
		name                   string
		discoveryError         error
		expectedDisabled       bool
		consecutiveFailures    int
		expectFailureIncrement bool
	}{
		{
			name:                   "non-recoverable error disables immediately",
			discoveryError:         &ParseError{Data: []byte{}, Format: "json", Err: errors.New("parse error")},
			expectedDisabled:       true,
			expectFailureIncrement: false,
		},
		{
			name:                   "recoverable error increments failure count",
			discoveryError:         &NetworkError{URL: "http://test", Err: errors.New("network error")},
			expectedDisabled:       false,
			expectFailureIncrement: true,
		},
		{
			name:                   "success resets failure count",
			discoveryError:         nil,
			expectedDisabled:       false,
			expectFailureIncrement: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := createMockEndpoint("http://localhost:11434", "test-endpoint")

			client := &mockDiscoveryClient{
				discoveryErrors: map[string]error{
					endpoint.URLString: tt.discoveryError,
				},
			}
			endpointRepo := &mockEndpointRepository{}
			modelRegistry := &mockModelRegistry{
				registeredModels: make([]*domain.ModelInfo, 0),
			}
			config := DiscoveryConfig{Timeout: 5 * time.Second}

			service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

			// Simulate previous failures if needed
			for i := 0; i < tt.consecutiveFailures; i++ {
				service.incrementFailureCount(endpoint.URLString)
			}

			_ = service.DiscoverEndpoint(context.Background(), endpoint)

			isDisabled := service.isEndpointDisabled(endpoint.URLString)
			if isDisabled != tt.expectedDisabled {
				t.Errorf("Expected endpoint disabled state to be %v, got %v", tt.expectedDisabled, isDisabled)
			}

			// For recoverable errors, check that failure count was increased
			if tt.expectFailureIncrement {
				failureCount := service.getFailureCount(endpoint.URLString)
				if failureCount == 0 {
					t.Errorf("Expected failure count to be increased for recoverable error")
				}
			}
		})
	}
}

func TestEndpointDisabledAfterMaxFailures(t *testing.T) {
	endpoint := createMockEndpoint("http://localhost:11434", "test-endpoint")

	client := &mockDiscoveryClient{
		discoveryErrors: map[string]error{
			endpoint.URLString: &NetworkError{URL: endpoint.URLString, Err: errors.New("network error")},
		},
	}
	endpointRepo := &mockEndpointRepository{}
	modelRegistry := &mockModelRegistry{
		registeredModels: make([]*domain.ModelInfo, 0),
	}
	config := DiscoveryConfig{Timeout: 5 * time.Second}

	service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

	// Fail discovery MaxConsecutiveFailures times
	for i := 0; i < MaxConsecutiveFailures; i++ {
		_ = service.DiscoverEndpoint(context.Background(), endpoint)

		// Should not be disabled until we hit the limit
		if i < MaxConsecutiveFailures-1 {
			if service.isEndpointDisabled(endpoint.URLString) {
				t.Errorf("Endpoint should not be disabled after %d failures", i+1)
			}
		}
	}

	// Should be disabled now
	if !service.isEndpointDisabled(endpoint.URLString) {
		t.Errorf("Endpoint should be disabled after %d consecutive failures", MaxConsecutiveFailures)
	}

	// Remove error to simulate successful discovery
	delete(client.discoveryErrors, endpoint.URLString)

	// Successful discovery should reset the failure count
	t.Logf("Debug: Before successful discovery - failure count: %d, disabled: %v",
		service.getFailureCount(endpoint.URLString), service.isEndpointDisabled(endpoint.URLString))

	err := service.DiscoverEndpoint(context.Background(), endpoint)
	if err != nil {
		t.Errorf("Expected successful discovery, got error: %v", err)
	}

	t.Logf("Debug: After successful discovery - failure count: %d, disabled: %v",
		service.getFailureCount(endpoint.URLString), service.isEndpointDisabled(endpoint.URLString))

	if service.isEndpointDisabled(endpoint.URLString) {
		t.Errorf("Endpoint should be re-enabled after successful discovery")
	}
}

func TestFilterActiveEndpoints(t *testing.T) {
	endpoints := []*domain.Endpoint{
		createMockEndpoint("http://localhost:11434", "enabled-1"),
		createMockEndpoint("http://localhost:1234", "disabled-1"),
		createMockEndpoint("http://localhost:5678", "enabled-2"),
	}

	service := NewModelDiscoveryService(nil, nil, nil, DiscoveryConfig{}, createTestLogger())

	// Disable one endpoint
	service.disableEndpoint("http://localhost:1234")

	activeEndpoints := service.filterActiveEndpoints(endpoints)

	if len(activeEndpoints) != 2 {
		t.Errorf("Expected 2 active endpoints, got %d", len(activeEndpoints))
	}

	// Check that the disabled endpoint is not in the active list
	for _, endpoint := range activeEndpoints {
		if endpoint.URLString == "http://localhost:1234" {
			t.Errorf("Disabled endpoint should not be in active list")
		}
	}
}

func TestConcurrentDiscovery(t *testing.T) {
	// Create multiple endpoints
	endpoints := []*domain.Endpoint{
		createMockEndpoint("http://localhost:11434", "endpoint-1"),
		createMockEndpoint("http://localhost:1234", "endpoint-2"),
		createMockEndpoint("http://localhost:5678", "endpoint-3"),
		createMockEndpoint("http://localhost:9012", "endpoint-4"),
	}

	client := &mockDiscoveryClient{}
	endpointRepo := &mockEndpointRepository{
		healthyEndpoints: endpoints,
	}
	modelRegistry := &mockModelRegistry{}
	config := DiscoveryConfig{
		Timeout:           5 * time.Second,
		ConcurrentWorkers: 2, // Limit concurrency
	}

	service := NewModelDiscoveryService(client, endpointRepo, modelRegistry, config, createTestLogger())

	start := time.Now()
	err := service.DiscoverAll(context.Background())
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should discover from all endpoints
	if client.discoverCallCount != len(endpoints) {
		t.Errorf("Expected %d discovery calls, got %d", len(endpoints), client.discoverCallCount)
	}

	// With 2 workers and 4 endpoints, should take roughly 2 batches
	// Each mock discovery takes ~10ms, so should be well under 100ms
	if duration > 100*time.Millisecond {
		t.Errorf("Discovery took too long: %v (expected < 100ms)", duration)
	}
}

// Mock implementations
type mockDiscoveryClient struct {
	discoveryErrors   map[string]error
	discoverCallCount int
	healthCheckError  error
	mu                sync.Mutex
}

func (m *mockDiscoveryClient) DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.discoverCallCount++

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	if err, exists := m.discoveryErrors[endpoint.URLString]; exists {
		return nil, err
	}

	return []*domain.ModelInfo{
		{Name: "model-1", LastSeen: time.Now()},
		{Name: "model-2", LastSeen: time.Now()},
	}, nil
}

func (m *mockDiscoveryClient) HealthCheck(ctx context.Context, endpoint *domain.Endpoint) error {
	return m.healthCheckError
}

func (m *mockDiscoveryClient) GetMetrics() DiscoveryMetrics {
	return DiscoveryMetrics{}
}

type mockEndpointRepository struct {
	healthyEndpoints []*domain.Endpoint
	getHealthyError  error
}

func (m *mockEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.healthyEndpoints, nil
}

func (m *mockEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.healthyEndpoints, nil
}

func (m *mockEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.healthyEndpoints, m.getHealthyError
}

func (m *mockEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

func (m *mockEndpointRepository) LoadFromConfig(ctx context.Context, configs []config.EndpointConfig) error {
	return nil
}

func (m *mockEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	return false
}

type mockModelRegistry struct {
	registeredModels  []*domain.ModelInfo
	registerError     error
	registerCallCount int
	mu                sync.Mutex
}

func (m *mockModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}

func (m *mockModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.registerCallCount++

	if m.registerError != nil {
		return m.registerError
	}

	if m.registeredModels == nil {
		m.registeredModels = make([]*domain.ModelInfo, 0)
	}

	m.registeredModels = append(m.registeredModels, models...)
	return nil
}

func (m *mockModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	return nil, nil
}

func (m *mockModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	return false
}

func (m *mockModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *mockModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}

func (r *mockModelRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return nil
}

func (r *mockModelRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}
func (r *mockModelRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

func (r *mockModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	// Mock doesn't support capabilities
	return []*domain.UnifiedModel{}, nil
}
func createMockEndpoint(urlString, name string) *domain.Endpoint {
	return &domain.Endpoint{
		Name:      name,
		URLString: urlString,
		Status:    domain.StatusHealthy,
	}
}
