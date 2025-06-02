package registry

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

type MemoryModelRegistry struct {
	endpointModels   map[string]*domain.EndpointModels
	modelToEndpoints map[string]map[string]struct{}
	stats            domain.RegistryStats
	mu               sync.RWMutex
}

func NewMemoryModelRegistry() *MemoryModelRegistry {
	return &MemoryModelRegistry{
		endpointModels:   make(map[string]*domain.EndpointModels),
		modelToEndpoints: make(map[string]map[string]struct{}),
		stats: domain.RegistryStats{
			TotalEndpoints:    0,
			TotalModels:       0,
			ModelsPerEndpoint: make(map[string]int),
			LastUpdated:       time.Now(),
		},
	}
}

func (r *MemoryModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	if err := r.validateInputs(endpointURL, model.Name); err != nil {
		return domain.NewModelRegistryError("register_model", endpointURL, model.Name, err)
	}

	select {
	case <-ctx.Done():
		return domain.NewModelRegistryError("register_model", endpointURL, model.Name, ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	endpointData := r.endpointModels[endpointURL]
	if endpointData == nil {
		endpointData = &domain.EndpointModels{
			EndpointURL: endpointURL,
			Models:      make([]*domain.ModelInfo, 0, 1),
			LastUpdated: time.Now(),
		}
		r.endpointModels[endpointURL] = endpointData
	}

	modelExists := false
	for i, existing := range endpointData.Models {
		if existing.Name == model.Name {
			endpointData.Models[i] = &domain.ModelInfo{
				Name:        model.Name,
				Size:        model.Size,
				Type:        model.Type,
				Description: model.Description,
				LastSeen:    model.LastSeen,
			}
			modelExists = true
			break
		}
	}

	if !modelExists {
		endpointData.Models = append(endpointData.Models, &domain.ModelInfo{
			Name:        model.Name,
			Size:        model.Size,
			Type:        model.Type,
			Description: model.Description,
			LastSeen:    model.LastSeen,
		})
	}

	endpointData.LastUpdated = time.Now()

	if r.modelToEndpoints[model.Name] == nil {
		r.modelToEndpoints[model.Name] = make(map[string]struct{})
	}
	r.modelToEndpoints[model.Name][endpointURL] = struct{}{}

	r.updateStats()
	return nil
}

func (r *MemoryModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	if endpointURL == "" {
		return domain.NewModelRegistryError("register_models", endpointURL, "", fmt.Errorf("endpoint URL cannot be empty"))
	}

	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return domain.NewModelRegistryError("register_models", endpointURL, "", fmt.Errorf("invalid endpoint URL: %w", err))
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return domain.NewModelRegistryError("register_models", endpointURL, "", fmt.Errorf("endpoint URL must have scheme and host"))
	}

	select {
	case <-ctx.Done():
		return domain.NewModelRegistryError("register_models", endpointURL, "", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.removeEndpointFromIndex(endpointURL)

	if len(models) == 0 {
		delete(r.endpointModels, endpointURL)
		r.updateStats()
		return nil
	}

	modelsCopy := make([]*domain.ModelInfo, 0, len(models))
	for _, model := range models {
		if model.Name == "" {
			return domain.NewModelRegistryError("register_models", endpointURL, model.Name, fmt.Errorf("model name cannot be empty"))
		}

		modelsCopy = append(modelsCopy, &domain.ModelInfo{
			Name:        model.Name,
			Size:        model.Size,
			Type:        model.Type,
			Description: model.Description,
			LastSeen:    model.LastSeen,
		})

		if r.modelToEndpoints[model.Name] == nil {
			r.modelToEndpoints[model.Name] = make(map[string]struct{})
		}
		r.modelToEndpoints[model.Name][endpointURL] = struct{}{}
	}

	r.endpointModels[endpointURL] = &domain.EndpointModels{
		EndpointURL: endpointURL,
		Models:      modelsCopy,
		LastUpdated: time.Now(),
	}

	r.updateStats()
	return nil
}

func (r *MemoryModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	if endpointURL == "" {
		return nil, domain.NewModelRegistryError("get_models_for_endpoint", endpointURL, "", fmt.Errorf("endpoint URL cannot be empty"))
	}

	select {
	case <-ctx.Done():
		return nil, domain.NewModelRegistryError("get_models_for_endpoint", endpointURL, "", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	endpointData := r.endpointModels[endpointURL]
	if endpointData == nil {
		return []*domain.ModelInfo{}, nil
	}

	models := make([]*domain.ModelInfo, len(endpointData.Models))
	for i, model := range endpointData.Models {
		models[i] = &domain.ModelInfo{
			Name:        model.Name,
			Size:        model.Size,
			Type:        model.Type,
			Description: model.Description,
			LastSeen:    model.LastSeen,
		}
	}

	return models, nil
}

func (r *MemoryModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	if modelName == "" {
		return nil, domain.NewModelRegistryError("get_endpoints_for_model", "", modelName, fmt.Errorf("model name cannot be empty"))
	}

	select {
	case <-ctx.Done():
		return nil, domain.NewModelRegistryError("get_endpoints_for_model", "", modelName, ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	endpointSet := r.modelToEndpoints[modelName]
	if len(endpointSet) == 0 {
		return []string{}, nil
	}

	endpoints := make([]string, 0, len(endpointSet))
	for endpoint := range endpointSet {
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

func (r *MemoryModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	if modelName == "" {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.modelToEndpoints[modelName]) > 0
}

func (r *MemoryModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	select {
	case <-ctx.Done():
		return nil, domain.NewModelRegistryError("get_all_models", "", "", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]*domain.ModelInfo, len(r.endpointModels))
	for endpointURL, endpointData := range r.endpointModels {
		models := make([]*domain.ModelInfo, len(endpointData.Models))
		for i, model := range endpointData.Models {
			models[i] = &domain.ModelInfo{
				Name:        model.Name,
				Size:        model.Size,
				Type:        model.Type,
				Description: model.Description,
				LastSeen:    model.LastSeen,
			}
		}
		result[endpointURL] = models
	}

	return result, nil
}

func (r *MemoryModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	if endpointURL == "" {
		return domain.NewModelRegistryError("remove_endpoint", endpointURL, "", fmt.Errorf("endpoint URL cannot be empty"))
	}

	select {
	case <-ctx.Done():
		return domain.NewModelRegistryError("remove_endpoint", endpointURL, "", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.removeEndpointFromIndex(endpointURL)
	delete(r.endpointModels, endpointURL)
	r.updateStats()

	return nil
}

func (r *MemoryModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	select {
	case <-ctx.Done():
		return domain.RegistryStats{}, domain.NewModelRegistryError("get_stats", "", "", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	modelsPerEndpoint := make(map[string]int, len(r.stats.ModelsPerEndpoint))
	for k, v := range r.stats.ModelsPerEndpoint {
		modelsPerEndpoint[k] = v
	}

	return domain.RegistryStats{
		TotalEndpoints:    r.stats.TotalEndpoints,
		TotalModels:       r.stats.TotalModels,
		ModelsPerEndpoint: modelsPerEndpoint,
		LastUpdated:       r.stats.LastUpdated,
	}, nil
}

func (r *MemoryModelRegistry) validateInputs(endpointURL, modelName string) error {
	if endpointURL == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	if modelName == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	if strings.TrimSpace(endpointURL) == "" {
		return fmt.Errorf("endpoint URL cannot be whitespace only")
	}

	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name cannot be whitespace only")
	}

	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("endpoint URL must have scheme and host")
	}

	return nil
}

func (r *MemoryModelRegistry) removeEndpointFromIndex(endpointURL string) {
	endpointData := r.endpointModels[endpointURL]
	if endpointData != nil {
		for _, model := range endpointData.Models {
			if endpointSet := r.modelToEndpoints[model.Name]; endpointSet != nil {
				delete(endpointSet, endpointURL)
				if len(endpointSet) == 0 {
					delete(r.modelToEndpoints, model.Name)
				}
			}
		}
	}
}

func (r *MemoryModelRegistry) updateStats() {
	modelSet := make(map[string]struct{})
	modelsPerEndpoint := make(map[string]int)

	for endpointURL, endpointData := range r.endpointModels {
		modelCount := len(endpointData.Models)
		modelsPerEndpoint[endpointURL] = modelCount

		for _, model := range endpointData.Models {
			modelSet[model.Name] = struct{}{}
		}
	}

	r.stats.TotalEndpoints = len(r.endpointModels)
	r.stats.TotalModels = len(modelSet)
	r.stats.ModelsPerEndpoint = modelsPerEndpoint
	r.stats.LastUpdated = time.Now()
}