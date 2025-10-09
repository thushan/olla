package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/registry/routing"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// UnifiedMemoryModelRegistry extends MemoryModelRegistry with model unification
type UnifiedMemoryModelRegistry struct {
	unifier         ports.ModelUnifier
	routingStrategy ports.ModelRoutingStrategy
	*MemoryModelRegistry
	unifiedModels     *xsync.Map[string, *domain.UnifiedModel]         // Endpoint -> []UnifiedModel
	globalUnified     *xsync.Map[string, *domain.UnifiedModel]         // UnifiedID -> UnifiedModel (merged across endpoints)
	endpoints         *xsync.Map[string, *domain.Endpoint]             // URL -> Endpoint mapping
	modelEndpointSets *xsync.Map[string, *xsync.Map[string, struct{}]] // ModelID -> Set of endpoint URLs (cached for fast lookup)
	unificationMutex  sync.Mutex
}

// NewUnifiedMemoryModelRegistry creates a new registry with unification support
func NewUnifiedMemoryModelRegistry(logger logger.StyledLogger, unificationConfig *config.UnificationConfig,
	routingConfig *config.ModelRoutingStrategy, discovery DiscoveryService) *UnifiedMemoryModelRegistry {
	// Create unifier with config
	unifierFactory := unifier.NewFactory(logger)
	var modelUnifier ports.ModelUnifier

	if unificationConfig != nil && unificationConfig.StaleThreshold > 0 {
		// Create unifier with custom config
		unifierConfig := unifier.DefaultConfig()
		unifierConfig.ModelTTL = unificationConfig.StaleThreshold
		if unificationConfig.CleanupInterval > 0 {
			unifierConfig.CleanupInterval = unificationConfig.CleanupInterval
		}
		modelUnifier, _ = unifierFactory.CreateWithConfig(unifier.DefaultUnifierType, unifierConfig)
	} else {
		// Use default config
		modelUnifier, _ = unifierFactory.Create(unifier.DefaultUnifierType)
	}

	// create routing strategy
	var routingStrategy ports.ModelRoutingStrategy
	if routingConfig != nil {
		factory := routing.NewFactory(logger)
		// adapt discovery interface if provided
		var discoveryAdapter ports.DiscoveryService
		if discovery != nil {
			discoveryAdapter = &discoveryServiceAdapter{discovery: discovery}
		}

		// Attempt to create the configured strategy
		strategy, err := factory.Create(*routingConfig, discoveryAdapter)
		if err != nil || strategy == nil {
			// Log the error and fall back to strict strategy
			logger.Error("Failed to create routing strategy, falling back to strict",
				"configured_type", routingConfig.Type,
				"error", err)
			routingStrategy = routing.NewStrictStrategy(logger)
		} else {
			routingStrategy = strategy
		}
	} else {
		// default to strict strategy
		routingStrategy = routing.NewStrictStrategy(logger)
	}

	// Ensure routingStrategy is never nil
	if routingStrategy == nil {
		logger.Warn("Routing strategy was nil, using strict strategy as fallback")
		routingStrategy = routing.NewStrictStrategy(logger)
	}

	return &UnifiedMemoryModelRegistry{
		MemoryModelRegistry: NewMemoryModelRegistry(logger),
		unifier:             modelUnifier,
		routingStrategy:     routingStrategy,
		unifiedModels:       xsync.NewMap[string, *domain.UnifiedModel](),
		globalUnified:       xsync.NewMap[string, *domain.UnifiedModel](),
		endpoints:           xsync.NewMap[string, *domain.Endpoint](),
		modelEndpointSets:   xsync.NewMap[string, *xsync.Map[string, struct{}]](),
	}
}

// RegisterEndpoint stores endpoint information for later use
func (r *UnifiedMemoryModelRegistry) RegisterEndpoint(endpoint *domain.Endpoint) {
	if endpoint != nil && endpoint.GetURLString() != "" {
		r.endpoints.Store(endpoint.GetURLString(), endpoint)
	}
}

// RegisterModelsWithEndpoint registers models with full endpoint information
func (r *UnifiedMemoryModelRegistry) RegisterModelsWithEndpoint(ctx context.Context, endpoint *domain.Endpoint, models []*domain.ModelInfo) error {
	// Store endpoint
	r.RegisterEndpoint(endpoint)

	// Register models normally
	return r.RegisterModels(ctx, endpoint.GetURLString(), models)
}

// RegisterModels overrides the base method to add unification
func (r *UnifiedMemoryModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	// First, register models normally
	if err := r.MemoryModelRegistry.RegisterModels(ctx, endpointURL, models); err != nil {
		return err
	}

	// Invalidate any cached endpoint sets for these models since they're being updated
	for _, model := range models {
		if model != nil {
			r.modelEndpointSets.Delete(model.Name)
		}
	}

	// Then unify them
	go r.unifyModelsAsync(ctx, endpointURL, models)

	return nil
}

// unifyModelsAsync performs model unification in the background
func (r *UnifiedMemoryModelRegistry) unifyModelsAsync(ctx context.Context, endpointURL string, models []*domain.ModelInfo) {
	r.unificationMutex.Lock()
	defer r.unificationMutex.Unlock()

	// Get or create endpoint object
	endpoint, exists := r.endpoints.Load(endpointURL)
	if !exists {
		// Create a minimal endpoint object for backward compatibility
		endpoint = &domain.Endpoint{
			URLString: endpointURL,
			Name:      endpointURL, // Use URL as name if not provided
		}
	}

	// Unify all models for this endpoint
	unifiedModels, err := r.unifier.UnifyModels(ctx, models, endpoint)
	if err != nil {
		r.logger.ErrorWithEndpoint(endpoint.Name, "Failed to unify models", err)
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

		// update cached endpoint set for this model
		var endpointURLs []string
		for _, sourceEndpoint := range merged.SourceEndpoints {
			endpointURLs = append(endpointURLs, sourceEndpoint.EndpointURL)
		}

		// we cache under the unified ID
		r.updateEndpointSet(id, endpointURLs)

		// and also cache under all native names and aliases for fast lookup
		for _, sourceEndpoint := range merged.SourceEndpoints {
			r.updateEndpointSet(sourceEndpoint.NativeName, endpointURLs)
		}
		for _, alias := range merged.Aliases {
			r.updateEndpointSet(alias.Name, endpointURLs)
		}
	}

	// r.logger.InfoWithEndpoint(" ", endpointUrl, "models", len(unifiedModels))
}

// updateEndpointSet updates the cached endpoint set for a given model
// trying to avoid repeated list-to-set conversions in GetHealthyEndpointsForModel
func (r *UnifiedMemoryModelRegistry) updateEndpointSet(modelID string, endpoints []string) {
	endpointSet := xsync.NewMap[string, struct{}]()
	for _, url := range endpoints {
		endpointSet.Store(url, struct{}{})
	}
	r.modelEndpointSets.Store(modelID, endpointSet)
}

// GetEndpointSet returns the cached endpoint set for a given model
func (r *UnifiedMemoryModelRegistry) GetEndpointSet(modelID string) (*xsync.MapOf[string, struct{}], bool) {
	return r.modelEndpointSets.Load(modelID)
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
	unified, _ := r.GetUnifiedModel(ctx, modelName)
	if unified == nil {
		// Model not found in unified registry, return empty array
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
		// we're capturing model metadata BEFORE mutation to avoid accessing empty slices
		// when the last endpoint is removed (model.RemoveEndpoint empties SourceEndpoints)
		sourceEndpoints := make([]domain.SourceEndpoint, len(model.SourceEndpoints))
		copy(sourceEndpoints, model.SourceEndpoints)
		aliases := make([]domain.AliasEntry, len(model.Aliases))
		copy(aliases, model.Aliases)

		if model.RemoveEndpoint(endpointURL) {
			// If no endpoints left, remove the unified model and its cached sets
			if !model.IsAvailable() {
				r.globalUnified.Delete(id)
				// delete cache entries for all model names using captured snapshots
				r.modelEndpointSets.Delete(id)
				for _, sourceEndpoint := range sourceEndpoints {
					r.modelEndpointSets.Delete(sourceEndpoint.NativeName)
				}
				for _, alias := range aliases {
					r.modelEndpointSets.Delete(alias.Name)
				}
			} else {
				// Update the model
				model.DiskSize = model.GetTotalDiskSize()
				model.LastSeen = time.Now()

				// update cached endpoint set to reflect the removed endpoint
				var endpointURLs []string
				for _, sourceEndpoint := range model.SourceEndpoints {
					endpointURLs = append(endpointURLs, sourceEndpoint.EndpointURL)
				}

				// update cache for all model names
				r.updateEndpointSet(id, endpointURLs)
				for _, sourceEndpoint := range model.SourceEndpoints {
					r.updateEndpointSet(sourceEndpoint.NativeName, endpointURLs)
				}
				for _, alias := range model.Aliases {
					r.updateEndpointSet(alias.Name, endpointURLs)
				}
			}
		}
		return true
	})

	return nil
}

// GetHealthyEndpointsForModel returns healthy endpoints that have a specific model
func (r *UnifiedMemoryModelRegistry) GetHealthyEndpointsForModel(ctx context.Context, modelName string, endpointRepo domain.EndpointRepository) ([]*domain.Endpoint, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	endpointSet, found := r.GetEndpointSet(modelName)
	if !found {
		// Fallback: build set on demand and cache it
		endpointURLs, err := r.GetEndpointsForModel(ctx, modelName)
		if err != nil {
			// Error handling is a bit more nuanced, we want to propagate context errors
			// but treat model-not-found as empty result

			// If there's a better way to do this, we should improve the unifier's ResolveAlias method
			// and also apply the same fix in Scout, Sherpa and Scoot.

			// context errors should propagate - these indicate interrupted operations
			if ctx.Err() != nil {
				return nil, fmt.Errorf("failed to get endpoints for model: %w", err)
			}

			// model-not-found is treated as empty result (intentional design pattern)
			return []*domain.Endpoint{}, nil
		}

		if len(endpointURLs) == 0 {
			return []*domain.Endpoint{}, nil
		}

		// we create and cache the set for future lookups
		endpointSet = xsync.NewMap[string, struct{}]()
		for _, url := range endpointURLs {
			endpointSet.Store(url, struct{}{})
		}
		r.modelEndpointSets.Store(modelName, endpointSet)
	}

	// Get all healthy endpoints from the repository
	healthyEndpoints, err := endpointRepo.GetHealthy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get healthy endpoints: %w", err)
	}

	// Filter to only include endpoints that have the model using cached set
	var result []*domain.Endpoint
	for _, endpoint := range healthyEndpoints {
		if _, ok := endpointSet.Load(endpoint.GetURLString()); ok {
			result = append(result, endpoint)
		}
	}

	return result, nil
}

const (
	capabilityCode = "code"
)

// capabilityMatches checks if a model capability matches the requested capability
func capabilityMatches(modelCap, requestedCap string) bool {
	// Direct match
	if modelCap == requestedCap {
		return true
	}

	// Check aliases based on requested capability
	switch requestedCap {
	case "chat", "chat_completion":
		return modelCap == "chat" || modelCap == "chat_completion"
	case "text", "text_generation":
		return modelCap == "text" || modelCap == "text_generation" || modelCap == "completion"
	case "embeddings", "embedding":
		return modelCap == "embeddings" || modelCap == "embedding"
	case "vision", "vision_understanding":
		return modelCap == "vision" || modelCap == "vision_understanding" || modelCap == "image"
	case capabilityCode, "code_generation":
		return modelCap == capabilityCode || modelCap == "code_generation"
	case "function", "function_calling":
		return modelCap == "function" || modelCap == "function_calling" || modelCap == "tools"
	case "streaming", "stream":
		return modelCap == "streaming" || modelCap == "stream"
	}

	return false
}

// GetModelsByCapability returns models that support a specific capability
func (r *UnifiedMemoryModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var models []*domain.UnifiedModel

	r.globalUnified.Range(func(id string, model *domain.UnifiedModel) bool {
		// Check if model has the requested capability
		for _, cap := range model.Capabilities {
			if capabilityMatches(cap, capability) {
				models = append(models, model)
				return true
			}
		}
		return true
	})

	return models, nil
}

// UnifiedRegistryStats combines registry and unification statistics
type UnifiedRegistryStats struct {
	domain.RegistryStats
	domain.UnificationStats
	TotalUnifiedModels int `json:"total_unified_models"`
}

// GetRoutableEndpointsForModel implements model routing strategy
func (r *UnifiedMemoryModelRegistry) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// get endpoints that have this model
	modelEndpoints, err := r.GetEndpointsForModel(ctx, modelName)
	if err != nil {
		r.logger.Error("Failed to get endpoints for model", "model", modelName, "error", err)
		modelEndpoints = []string{} // treat error as model not found
	}

	// delegate to routing strategy
	return r.routingStrategy.GetRoutableEndpoints(ctx, modelName, healthyEndpoints, modelEndpoints)
}

// discoveryServiceAdapter adapts our DiscoveryService to ports.DiscoveryService
type discoveryServiceAdapter struct {
	discovery DiscoveryService
}

func (a *discoveryServiceAdapter) RefreshEndpoints(ctx context.Context) error {
	return a.discovery.RefreshEndpoints(ctx)
}

func (a *discoveryServiceAdapter) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	// Try to get all endpoints first
	endpoints, err := a.discovery.GetEndpoints(ctx)
	if err != nil {
		// Fallback to healthy endpoints for implementations that may fail
		// when retrieving all endpoints or only provide healthy endpoints
		return a.discovery.GetHealthyEndpoints(ctx)
	}
	return endpoints, nil
}

func (a *discoveryServiceAdapter) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return a.discovery.GetHealthyEndpoints(ctx)
}

func (a *discoveryServiceAdapter) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	if updater, ok := a.discovery.(interface {
		UpdateEndpointStatus(context.Context, *domain.Endpoint) error
	}); ok {
		return updater.UpdateEndpointStatus(ctx, endpoint)
	}
	return nil
}
