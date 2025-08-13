package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

// TestModelFallbackWhenEndpointGoesOffline tests the specific scenario reported by the user:
// When an endpoint with a model goes offline, requests should fall back to other healthy endpoints
func TestModelFallbackWhenEndpointGoesOffline(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockLogger := logger.NewStyledLogger(testLogger, &theme.Theme{}, false)

	// Create test endpoints - note localOllama is offline (not in healthy list)
	macOllama := &domain.Endpoint{
		Name:      "mac-ollama",
		URLString: "http://192.168.0.144:11434",
		Type:      "ollama",
		Status:    domain.StatusHealthy,
		Priority:  100,
	}

	// Mock registry that knows local-ollama has the model (even though it's offline)
	mockRegistry := &mockModelRegistryForFallback{
		endpointsForModel: map[string][]string{
			"phi3.5:latest": {"http://localhost:11434"}, // Only the offline endpoint has it
		},
	}

	app := &Application{
		logger:        mockLogger,
		modelRegistry: mockRegistry,
	}

	// Create request profile for phi3.5:latest
	profile := &domain.RequestProfile{
		ModelName:   "phi3.5:latest",
		Path:        "/api/chat",
		SupportedBy: []string{"ollama"},
	}

	// Test filtering - only healthy endpoints passed in
	healthyEndpoints := []*domain.Endpoint{macOllama}
	filtered := app.filterEndpointsByProfile(healthyEndpoints, profile, mockLogger)

	// Should return mac-ollama even though it doesn't have the model in registry
	// because local-ollama (which has the model) is offline
	if len(filtered) != 1 {
		t.Fatalf("Expected 1 endpoint after filtering, got %d", len(filtered))
	}

	if filtered[0].Name != "mac-ollama" {
		t.Errorf("Expected mac-ollama to be selected for fallback, got %s", filtered[0].Name)
	}
}

// TestModelRoutingWhenAllHealthy tests normal routing when all endpoints are healthy
func TestModelRoutingWhenAllHealthy(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockLogger := logger.NewStyledLogger(testLogger, &theme.Theme{}, false)

	localOllama := &domain.Endpoint{
		Name:      "local-ollama",
		URLString: "http://localhost:11434",
		Type:      "ollama",
		Status:    domain.StatusHealthy,
		Priority:  100,
	}

	macOllama := &domain.Endpoint{
		Name:      "mac-ollama",
		URLString: "http://192.168.0.144:11434",
		Type:      "ollama",
		Status:    domain.StatusHealthy,
		Priority:  100,
	}

	// Mock registry - local-ollama has the model
	mockRegistry := &mockModelRegistryForFallback{
		endpointsForModel: map[string][]string{
			"phi3.5:latest": {"http://localhost:11434"},
		},
	}

	app := &Application{
		logger:        mockLogger,
		modelRegistry: mockRegistry,
	}

	profile := &domain.RequestProfile{
		ModelName:   "phi3.5:latest",
		Path:        "/api/chat",
		SupportedBy: []string{"ollama"},
	}

	// Test filtering
	healthyEndpoints := []*domain.Endpoint{localOllama, macOllama}
	filtered := app.filterEndpointsByProfile(healthyEndpoints, profile, mockLogger)

	// Should return only local-ollama since it has the model
	if len(filtered) != 1 {
		t.Fatalf("Expected 1 endpoint after filtering, got %d", len(filtered))
	}

	if filtered[0].Name != "local-ollama" {
		t.Errorf("Expected local-ollama to be selected (has model), got %s", filtered[0].Name)
	}
}

// mockModelRegistryForFallback for testing
type mockModelRegistryForFallback struct {
	baseMockRegistry
	endpointsForModel map[string][]string
}

func (m *mockModelRegistryForFallback) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	if endpoints, ok := m.endpointsForModel[modelName]; ok {
		return endpoints, nil
	}
	return []string{}, nil
}

func (m *mockModelRegistryForFallback) IsModelAvailable(ctx context.Context, modelName string) bool {
	_, ok := m.endpointsForModel[modelName]
	return ok
}

func (m *mockModelRegistryForFallback) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// get endpoints that have this model
	modelEndpoints, _ := m.GetEndpointsForModel(ctx, modelName)

	// filter healthy endpoints to only those with the model
	modelEndpointMap := make(map[string]bool)
	for _, url := range modelEndpoints {
		modelEndpointMap[url] = true
	}

	var routable []*domain.Endpoint
	for _, endpoint := range healthyEndpoints {
		if modelEndpointMap[endpoint.URLString] {
			routable = append(routable, endpoint)
		}
	}

	// if no healthy endpoints have the model, fall back to all healthy
	if len(routable) == 0 && len(modelEndpoints) > 0 {
		// model exists but only on unhealthy endpoints - fallback
		return healthyEndpoints, &domain.ModelRoutingDecision{
			Strategy: "test-fallback",
			Action:   "fallback",
			Reason:   "model only on unhealthy endpoints",
		}, nil
	}

	if len(routable) == 0 {
		// model doesn't exist
		return nil, &domain.ModelRoutingDecision{
			Strategy:   "test",
			Action:     "rejected",
			Reason:     "model not found",
			StatusCode: 404,
		}, nil
	}

	return routable, &domain.ModelRoutingDecision{
		Strategy: "test",
		Action:   "routed",
		Reason:   "model found",
	}, nil
}
