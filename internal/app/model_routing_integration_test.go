package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// Mock components for integration testing
type mockDiscoveryService struct {
	endpoints []*domain.Endpoint
}

func (m *mockDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	var healthy []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return nil
}

type mockEndpointSelector struct {
	selectFunc func(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error)
}

func (m *mockEndpointSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if m.selectFunc != nil {
		return m.selectFunc(ctx, endpoints)
	}
	if len(endpoints) > 0 {
		return endpoints[0], nil
	}
	return nil, nil
}

func (m *mockEndpointSelector) Name() string {
	return "mock"
}

func (m *mockEndpointSelector) IncrementConnections(endpoint *domain.Endpoint) {}
func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {}

func TestModelRoutingIntegration(t *testing.T) {
	// Create test logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create unified registry
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil)

	// Create test endpoints
	endpoint1, _ := url.Parse("http://localhost:11434")
	endpoint2, _ := url.Parse("http://localhost:11435")
	endpoint3, _ := url.Parse("http://localhost:11436")

	endpoints := []*domain.Endpoint{
		{
			Name:        "ollama1",
			URL:         endpoint1,
			URLString:   "http://localhost:11434",
			Status:      domain.StatusHealthy,
			Priority:    1,
			LastChecked: time.Now(),
		},
		{
			Name:        "ollama2",
			URL:         endpoint2,
			URLString:   "http://localhost:11435",
			Status:      domain.StatusHealthy,
			Priority:    2,
			LastChecked: time.Now(),
		},
		{
			Name:        "ollama3",
			URL:         endpoint3,
			URLString:   "http://localhost:11436",
			Status:      domain.StatusUnhealthy,
			Priority:    3,
			LastChecked: time.Now(),
		},
	}

	// Register endpoints in the registry
	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register models on endpoints
	ctx := context.Background()

	// llama3 available on endpoints 1 and 2
	llamaModel := &domain.ModelInfo{
		Name:     "llama3:8b",
		LastSeen: time.Now(),
	}
	unifiedRegistry.RegisterModel(ctx, endpoints[0].URLString, llamaModel)
	unifiedRegistry.RegisterModel(ctx, endpoints[1].URLString, llamaModel)

	// mistral only on unhealthy endpoint 3
	mistralModel := &domain.ModelInfo{
		Name:     "mistral:7b",
		LastSeen: time.Now(),
	}
	unifiedRegistry.RegisterModel(ctx, endpoints[2].URLString, mistralModel)

	// Create mock services
	discovery := &mockDiscoveryService{endpoints: endpoints}

	// Test cases
	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		setupSelector  func() *mockEndpointSelector
		description    string
	}{
		{
			name: "Route to healthy endpoint with model",
			requestBody: map[string]interface{}{
				"model":  "llama3:8b",
				"prompt": "Hello",
			},
			expectedStatus: http.StatusOK,
			setupSelector: func() *mockEndpointSelector {
				return &mockEndpointSelector{
					selectFunc: func(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
						// Should receive only healthy endpoints with the model
						if len(endpoints) != 2 {
							t.Errorf("Expected 2 endpoints, got %d", len(endpoints))
						}
						return endpoints[0], nil
					},
				}
			},
			description: "Should route llama3 requests to healthy endpoints",
		},
		{
			name: "Model only on unhealthy endpoint",
			requestBody: map[string]interface{}{
				"model":  "mistral:7b",
				"prompt": "Hello",
			},
			expectedStatus: http.StatusServiceUnavailable,
			setupSelector: func() *mockEndpointSelector {
				return &mockEndpointSelector{
					selectFunc: func(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
						// Should receive no endpoints since mistral is only on unhealthy endpoint
						if len(endpoints) != 0 {
							t.Errorf("Expected 0 endpoints, got %d", len(endpoints))
						}
						return nil, nil
					},
				}
			},
			description: "Should not route to unhealthy endpoints even if model available",
		},
		{
			name: "Non-existent model",
			requestBody: map[string]interface{}{
				"model":  "gpt-4",
				"prompt": "Hello",
			},
			expectedStatus: http.StatusOK,
			setupSelector: func() *mockEndpointSelector {
				return &mockEndpointSelector{
					selectFunc: func(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
						// Should receive all healthy endpoints as fallback
						if len(endpoints) != 2 {
							t.Errorf("Expected 2 healthy endpoints as fallback, got %d", len(endpoints))
						}
						return endpoints[0], nil
					},
				}
			},
			description: "Should fallback to all healthy endpoints for unknown models",
		},
		{
			name: "Request without model field",
			requestBody: map[string]interface{}{
				"prompt": "Hello",
			},
			expectedStatus: http.StatusOK,
			setupSelector: func() *mockEndpointSelector {
				return &mockEndpointSelector{
					selectFunc: func(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
						// Should receive all healthy endpoints
						if len(endpoints) != 2 {
							t.Errorf("Expected 2 healthy endpoints, got %d", len(endpoints))
						}
						return endpoints[0], nil
					},
				}
			},
			description: "Should use all healthy endpoints when no model specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Create proxy handler with model routing logic
			selector := tt.setupSelector()

			// Simple handler that simulates the model routing logic
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Read and parse request body
				var requestData map[string]interface{}
				if r.Body != nil {
					decoder := json.NewDecoder(r.Body)
					decoder.Decode(&requestData)
				}

				// Extract model name
				modelName, _ := requestData["model"].(string)

				// Get healthy endpoints for model
				var filteredEndpoints []*domain.Endpoint
				if modelName != "" {
					healthyForModel, _ := unifiedRegistry.GetHealthyEndpointsForModel(ctx, modelName, &mockEndpointRepository{endpoints: endpoints})
					if len(healthyForModel) > 0 {
						// Use model-specific endpoints
						filteredEndpoints = healthyForModel
					} else {
						// Check if model exists at all
						allEndpointsForModel, _ := unifiedRegistry.GetEndpointsForModel(ctx, modelName)
						if len(allEndpointsForModel) > 0 {
							// Model exists but not on healthy endpoints
							filteredEndpoints = []*domain.Endpoint{}
						} else {
							// Model doesn't exist, fallback to all healthy endpoints
							filteredEndpoints, _ = discovery.GetHealthyEndpoints(ctx)
						}
					}
				} else {
					// No model specified, use all healthy endpoints
					filteredEndpoints, _ = discovery.GetHealthyEndpoints(ctx)
				}

				// Select endpoint
				selected, _ := selector.Select(ctx, filteredEndpoints)

				if selected == nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte("No available endpoints"))
					return
				}

				// Success - would normally proxy to selected endpoint
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"endpoint": "` + selected.Name + `"}`))
			})

			// Serve the request
			handler.ServeHTTP(rr, req)

			// Check status
			if rr.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, rr.Code)
			}
		})
	}
}

// Mock endpoint repository for the test
type mockEndpointRepository struct {
	endpoints []*domain.Endpoint
}

func (m *mockEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	var routable []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.Status.IsRoutable() {
			routable = append(routable, ep)
		}
	}
	return routable, nil
}

func (m *mockEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	var healthy []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
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
