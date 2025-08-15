package handlers

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
)

// baseMockRegistry provides default implementations for all ModelRegistry methods
type baseMockRegistry struct{}

func (m *baseMockRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}

func (m *baseMockRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	return nil
}

func (m *baseMockRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *baseMockRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	return []string{}, nil
}

func (m *baseMockRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	return false
}

func (m *baseMockRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *baseMockRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

func (m *baseMockRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *baseMockRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}

func (m *baseMockRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}

func (m *baseMockRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return nil
}

func (m *baseMockRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	return nil, nil
}

// GetRoutableEndpointsForModel provides basic routing without strategy
func (m *baseMockRegistry) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// basic implementation - return all healthy endpoints
	decision := &domain.ModelRoutingDecision{
		Strategy: "mock",
		Action:   "routed",
		Reason:   "mock routing",
	}
	return healthyEndpoints, decision, nil
}
