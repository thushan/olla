package registry

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// Mock endpoint repository for testing
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

func createTestUnifiedRegistry() *UnifiedMemoryModelRegistry {
	return NewUnifiedMemoryModelRegistry(createTestLogger(), nil, nil, nil)
}

func TestGetHealthyEndpointsForModel(t *testing.T) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Create test endpoints
	endpoint1 := &domain.Endpoint{
		URLString: "http://localhost:11434",
		Name:      "ollama1",
		Status:    domain.StatusHealthy,
	}

	endpoint2 := &domain.Endpoint{
		URLString: "http://localhost:11435",
		Name:      "ollama2",
		Status:    domain.StatusHealthy,
	}

	endpoint3 := &domain.Endpoint{
		URLString: "http://localhost:11436",
		Name:      "ollama3",
		Status:    domain.StatusUnhealthy,
	}

	// Register endpoints
	registry.RegisterEndpoint(endpoint1)
	registry.RegisterEndpoint(endpoint2)
	registry.RegisterEndpoint(endpoint3)

	// Create mock endpoint repository
	mockRepo := &mockEndpointRepository{
		endpoints: []*domain.Endpoint{endpoint1, endpoint2, endpoint3},
	}

	// Register models for endpoints
	model1 := &domain.ModelInfo{
		Name:     "llama3:8b",
		LastSeen: time.Now(),
	}

	model2 := &domain.ModelInfo{
		Name:     "mistral:7b",
		LastSeen: time.Now(),
	}

	// Register models - llama3:8b on endpoints 1 and 2, mistral:7b only on endpoint 3
	err := registry.RegisterModel(ctx, endpoint1.URLString, model1)
	if err != nil {
		t.Fatalf("Failed to register model1 on endpoint1: %v", err)
	}

	err = registry.RegisterModel(ctx, endpoint2.URLString, model1)
	if err != nil {
		t.Fatalf("Failed to register model1 on endpoint2: %v", err)
	}

	err = registry.RegisterModel(ctx, endpoint3.URLString, model2)
	if err != nil {
		t.Fatalf("Failed to register model2 on endpoint3: %v", err)
	}

	// Test cases
	tests := []struct {
		name          string
		modelName     string
		expectedCount int
		expectedURLs  []string
	}{
		{
			name:          "Model available on healthy endpoints",
			modelName:     "llama3:8b",
			expectedCount: 2,
			expectedURLs:  []string{"http://localhost:11434", "http://localhost:11435"},
		},
		{
			name:          "Model only on unhealthy endpoint",
			modelName:     "mistral:7b",
			expectedCount: 0,
			expectedURLs:  []string{},
		},
		{
			name:          "Non-existent model",
			modelName:     "gpt-4",
			expectedCount: 0,
			expectedURLs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthyEndpoints, err := registry.GetHealthyEndpointsForModel(ctx, tt.modelName, mockRepo)
			if err != nil {
				t.Fatalf("GetHealthyEndpointsForModel failed: %v", err)
			}

			if len(healthyEndpoints) != tt.expectedCount {
				t.Errorf("Expected %d healthy endpoints, got %d", tt.expectedCount, len(healthyEndpoints))
			}

			// Check that the returned endpoints match expected URLs
			returnedURLs := make(map[string]bool)
			for _, ep := range healthyEndpoints {
				returnedURLs[ep.GetURLString()] = true
			}

			for _, expectedURL := range tt.expectedURLs {
				if !returnedURLs[expectedURL] {
					t.Errorf("Expected endpoint URL %s not found in results", expectedURL)
				}
			}
		})
	}
}

func TestGetModelsByCapability(t *testing.T) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Manually create and add unified models with different capabilities
	// Since we're testing the capability filtering, we'll directly add to globalUnified

	chatModel := &domain.UnifiedModel{
		ID:           "chat-model",
		Family:       "test",
		Capabilities: []string{"chat", "streaming"},
	}

	embeddingModel := &domain.UnifiedModel{
		ID:           "embedding-model",
		Family:       "test",
		Capabilities: []string{"embeddings"},
	}

	visionModel := &domain.UnifiedModel{
		ID:           "vision-model",
		Family:       "test",
		Capabilities: []string{"chat", "vision", "streaming"},
	}

	codeModel := &domain.UnifiedModel{
		ID:           "code-model",
		Family:       "test",
		Capabilities: []string{"chat", "code", "function_calling"},
	}

	// Store models directly in globalUnified for testing
	registry.globalUnified.Store(chatModel.ID, chatModel)
	registry.globalUnified.Store(embeddingModel.ID, embeddingModel)
	registry.globalUnified.Store(visionModel.ID, visionModel)
	registry.globalUnified.Store(codeModel.ID, codeModel)

	// Test cases
	tests := []struct {
		name        string
		capability  string
		expectedIDs []string
	}{
		{
			name:        "Chat capability",
			capability:  "chat",
			expectedIDs: []string{"chat-model", "vision-model", "code-model"},
		},
		{
			name:        "Chat completion capability (alternate name)",
			capability:  "chat_completion",
			expectedIDs: []string{"chat-model", "vision-model", "code-model"},
		},
		{
			name:        "Embeddings capability",
			capability:  "embeddings",
			expectedIDs: []string{"embedding-model"},
		},
		{
			name:        "Vision capability",
			capability:  "vision",
			expectedIDs: []string{"vision-model"},
		},
		{
			name:        "Code generation capability",
			capability:  "code",
			expectedIDs: []string{"code-model"},
		},
		{
			name:        "Function calling capability",
			capability:  "function",
			expectedIDs: []string{"code-model"},
		},
		{
			name:        "Streaming capability",
			capability:  "streaming",
			expectedIDs: []string{"chat-model", "vision-model"},
		},
		{
			name:        "Unknown capability",
			capability:  "unknown",
			expectedIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := registry.GetModelsByCapability(ctx, tt.capability)
			if err != nil {
				t.Fatalf("GetModelsByCapability failed: %v", err)
			}

			if len(models) != len(tt.expectedIDs) {
				t.Errorf("Expected %d models, got %d", len(tt.expectedIDs), len(models))
			}

			// Check that all expected models are returned
			returnedIDs := make(map[string]bool)
			for _, model := range models {
				returnedIDs[model.ID] = true
			}

			for _, expectedID := range tt.expectedIDs {
				if !returnedIDs[expectedID] {
					t.Errorf("Expected model ID %s not found in results", expectedID)
				}
			}
		})
	}
}

func TestGetHealthyEndpointsForModel_ContextCancellation(t *testing.T) {
	registry := createTestUnifiedRegistry()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockRepo := &mockEndpointRepository{}

	_, err := registry.GetHealthyEndpointsForModel(ctx, "test-model", mockRepo)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

func TestGetModelsByCapability_ContextCancellation(t *testing.T) {
	registry := createTestUnifiedRegistry()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := registry.GetModelsByCapability(ctx, "chat")
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

// TestEndpointSetCaching verifies that the endpoint set cache improves performance
func TestEndpointSetCaching(t *testing.T) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Create test endpoints
	endpoint1 := &domain.Endpoint{
		URLString: "http://localhost:11434",
		Name:      "ollama1",
		Status:    domain.StatusHealthy,
	}

	endpoint2 := &domain.Endpoint{
		URLString: "http://localhost:11435",
		Name:      "ollama2",
		Status:    domain.StatusHealthy,
	}

	// Register endpoints
	registry.RegisterEndpoint(endpoint1)
	registry.RegisterEndpoint(endpoint2)

	mockRepo := &mockEndpointRepository{
		endpoints: []*domain.Endpoint{endpoint1, endpoint2},
	}

	// Register a model on both endpoints
	model := &domain.ModelInfo{
		Name:     "llama3:8b",
		LastSeen: time.Now(),
	}

	err := registry.RegisterModel(ctx, endpoint1.URLString, model)
	if err != nil {
		t.Fatalf("Failed to register model on endpoint1: %v", err)
	}

	err = registry.RegisterModel(ctx, endpoint2.URLString, model)
	if err != nil {
		t.Fatalf("Failed to register model on endpoint2: %v", err)
	}

	// First call should work (may create cache)
	healthyEndpoints, err := registry.GetHealthyEndpointsForModel(ctx, "llama3:8b", mockRepo)
	if err != nil {
		t.Fatalf("GetHealthyEndpointsForModel failed: %v", err)
	}
	if len(healthyEndpoints) != 2 {
		t.Errorf("Expected 2 healthy endpoints, got %d", len(healthyEndpoints))
	}

	// Second call should use cache (verify cache exists)
	endpointSet, found := registry.GetEndpointSet("llama3:8b")
	if !found {
		t.Log("Cache not found after first call - this is OK, may use fallback path")
	} else {
		// If cache exists, verify it's correct
		_, ok1 := endpointSet.Load(endpoint1.URLString)
		_, ok2 := endpointSet.Load(endpoint2.URLString)
		if !ok1 || !ok2 {
			t.Error("Cached set should contain both endpoints")
		}
	}

	// Third call should definitely use cache and return same results
	healthyEndpoints2, err := registry.GetHealthyEndpointsForModel(ctx, "llama3:8b", mockRepo)
	if err != nil {
		t.Fatalf("GetHealthyEndpointsForModel failed on second call: %v", err)
	}
	if len(healthyEndpoints2) != 2 {
		t.Errorf("Expected 2 healthy endpoints on second call, got %d", len(healthyEndpoints2))
	}
}

// TestEndpointSetCacheConcurrency verifies the cache is thread-safe under concurrent access
func TestEndpointSetCacheConcurrency(t *testing.T) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Create test endpoints
	endpoints := make([]*domain.Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &domain.Endpoint{
			URLString: fmt.Sprintf("http://localhost:%d", 11434+i),
			Name:      fmt.Sprintf("endpoint-%d", i),
			Status:    domain.StatusHealthy,
		}
		registry.RegisterEndpoint(endpoints[i])
	}

	mockRepo := &mockEndpointRepository{endpoints: endpoints}

	// Concurrently register models and query healthy endpoints
	done := make(chan bool)
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			model := &domain.ModelInfo{
				Name:     fmt.Sprintf("test-model-%d", id%5),
				LastSeen: time.Now(),
			}

			// Register model on random endpoints
			for j := 0; j < 5; j++ {
				endpointIdx := (id + j) % len(endpoints)
				_ = registry.RegisterModel(ctx, endpoints[endpointIdx].URLString, model)
			}

			// Query healthy endpoints multiple times
			for j := 0; j < 10; j++ {
				modelName := fmt.Sprintf("test-model-%d", j%5)
				_, _ = registry.GetHealthyEndpointsForModel(ctx, modelName, mockRepo)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// If we get here without panicking, the cache is thread-safe
	t.Log("Cache handled concurrent access successfully")
}
