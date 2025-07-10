package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// UnifiedMemoryModelRegistry extends MemoryModelRegistry with model unification
type UnifiedMemoryModelRegistry struct {
	*MemoryModelRegistry
	unifier          ports.ModelUnifier
	unifiedModels    *xsync.Map[string, *domain.UnifiedModel] // Endpoint -> []UnifiedModel
	globalUnified    *xsync.Map[string, *domain.UnifiedModel] // UnifiedID -> UnifiedModel (merged across endpoints)
	unificationMutex sync.Mutex
}

// NewUnifiedMemoryModelRegistry creates a new registry with unification support
func NewUnifiedMemoryModelRegistry(logger logger.StyledLogger) *UnifiedMemoryModelRegistry {
	// Create unifier
	unifierFactory := unifier.NewFactory(logger)
	modelUnifier, _ := unifierFactory.Create(unifier.DefaultUnifierType)

	return &UnifiedMemoryModelRegistry{
		MemoryModelRegistry: NewMemoryModelRegistry(logger),
		unifier:             modelUnifier,
		unifiedModels:       xsync.NewMap[string, *domain.UnifiedModel](),
		globalUnified:       xsync.NewMap[string, *domain.UnifiedModel](),
	}
}

// RegisterModels overrides the base method to add unification
func (r *UnifiedMemoryModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	// First, register models normally
	if err := r.MemoryModelRegistry.RegisterModels(ctx, endpointURL, models); err != nil {
		return err
	}

	// Then unify them
	go r.unifyModelsAsync(ctx, endpointURL, models)

	return nil
}

// unifyModelsAsync performs model unification in the background
func (r *UnifiedMemoryModelRegistry) unifyModelsAsync(ctx context.Context, endpointURL string, models []*domain.ModelInfo) {
	r.unificationMutex.Lock()
	defer r.unificationMutex.Unlock()

	// Unify all models for this endpoint
	unifiedModels, err := r.unifier.UnifyModels(ctx, models, endpointURL)
	if err != nil {
		r.logger.ErrorWithEndpoint(endpointURL, "Failed to unify models", err)
		return
	}

	// Group unified models by ID for merging
	modelGroups := make(map[string][]*domain.UnifiedModel)
	for _, unified := range unifiedModels {
		modelGroups[unified.ID] = append(modelGroups[unified.ID], unified)
	}

	// Merge models across endpoints
	for id, group := range modelGroups {
		// Check if we already have this model globally
		existing, exists := r.globalUnified.Load(id)
		if exists {
			// Merge with existing
			group = append(group, existing)
		}

		merged, err := r.unifier.MergeUnifiedModels(ctx, group)
		if err != nil {
			r.logger.Error("Failed to merge unified models", err)
			continue
		}

		r.globalUnified.Store(id, merged)
	}

	r.logger.InfoWithEndpoint(endpointURL, fmt.Sprintf("Unified %d models", len(unifiedModels)))
}

// GetUnifiedModels returns all unified models
func (r *UnifiedMemoryModelRegistry) GetUnifiedModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var models []*domain.UnifiedModel
	r.globalUnified.Range(func(id string, model *domain.UnifiedModel) bool {
		models = append(models, model)
		return true
	})

	return models, nil
}

// GetUnifiedModel returns a specific unified model by ID or alias
func (r *UnifiedMemoryModelRegistry) GetUnifiedModel(ctx context.Context, idOrAlias string) (*domain.UnifiedModel, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try direct ID lookup first
	if model, exists := r.globalUnified.Load(idOrAlias); exists {
		return model, nil
	}

	// Try alias resolution
	unified, err := r.unifier.ResolveAlias(ctx, idOrAlias)
	if err != nil {
		return nil, fmt.Errorf("model not found: %s", idOrAlias)
	}

	return unified, nil
}

// IsModelAvailable overrides to check by unified ID or alias
func (r *UnifiedMemoryModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	// First check the original registry
	if r.MemoryModelRegistry.IsModelAvailable(ctx, modelName) {
		return true
	}

	// Then check unified models
	_, err := r.GetUnifiedModel(ctx, modelName)
	return err == nil
}

// GetEndpointsForModel overrides to support unified model lookup
func (r *UnifiedMemoryModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	// Try original lookup first
	endpoints, err := r.MemoryModelRegistry.GetEndpointsForModel(ctx, modelName)
	if err == nil && len(endpoints) > 0 {
		return endpoints, nil
	}

	// Try unified model lookup
	unified, err := r.GetUnifiedModel(ctx, modelName)
	if err != nil {
		return []string{}, nil
	}

	// Extract endpoints from unified model
	var endpointURLs []string
	for _, endpoint := range unified.SourceEndpoints {
		endpointURLs = append(endpointURLs, endpoint.EndpointURL)
	}

	return endpointURLs, nil
}

// GetUnifiedStats returns statistics including unification metrics
func (r *UnifiedMemoryModelRegistry) GetUnifiedStats(ctx context.Context) (UnifiedRegistryStats, error) {
	baseStats, err := r.MemoryModelRegistry.GetStats(ctx)
	if err != nil {
		return UnifiedRegistryStats{}, err
	}

	unifierStats := r.unifier.GetStats()

	var totalUnifiedModels int
	r.globalUnified.Range(func(_ string, _ *domain.UnifiedModel) bool {
		totalUnifiedModels++
		return true
	})

	return UnifiedRegistryStats{
		RegistryStats:      baseStats,
		UnificationStats:   unifierStats,
		TotalUnifiedModels: totalUnifiedModels,
	}, nil
}

// RemoveEndpoint overrides to clean up unified models
func (r *UnifiedMemoryModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	// First remove from base registry
	if err := r.MemoryModelRegistry.RemoveEndpoint(ctx, endpointURL); err != nil {
		return err
	}

	// Clean up unified models
	r.unificationMutex.Lock()
	defer r.unificationMutex.Unlock()

	// Remove endpoint from all unified models
	r.globalUnified.Range(func(id string, model *domain.UnifiedModel) bool {
		if model.RemoveEndpoint(endpointURL) {
			// If no endpoints left, remove the unified model
			if !model.IsAvailable() {
				r.globalUnified.Delete(id)
			} else {
				// Update the model
				model.DiskSize = model.GetTotalDiskSize()
				model.LastSeen = time.Now()
			}
		}
		return true
	})

	return nil
}

// UnifiedRegistryStats combines registry and unification statistics
type UnifiedRegistryStats struct {
	domain.RegistryStats
	domain.UnificationStats
	TotalUnifiedModels int `json:"total_unified_models"`
}