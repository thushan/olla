package unifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// DiscoveryClient interface for endpoint discovery operations
type DiscoveryClient interface {
	DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error)
}

// LifecycleUnifier adds production-ready lifecycle management to model unification.
// It tracks endpoint health, manages stale data cleanup, and provides circuit breaker
// protection to prevent cascade failures.
type LifecycleUnifier struct {
	logger          logger.StyledLogger
	cleanupCtx      context.Context
	discoveryClient DiscoveryClient
	*DefaultUnifier
	cleanupCancel     context.CancelFunc
	endpointStates    map[string]*domain.EndpointStateInfo
	endpointFailures  map[string]int
	circuitBreakers   map[string]*CircuitBreaker
	lastEndpointCheck map[string]time.Time
	stateTransitions  chan domain.StateTransition
	config            Config
	cleanupWg         sync.WaitGroup
	mu                sync.RWMutex
	isRunning         atomic.Bool
}

func NewLifecycleUnifier(config Config, logger logger.StyledLogger) ports.ModelUnifier {
	if err := config.Validate(); err != nil {
		config = DefaultConfig()
	}

	return &LifecycleUnifier{
		DefaultUnifier:    NewDefaultUnifier().(*DefaultUnifier), //nolint:forcetypeassert // NewDefaultUnifier always returns *DefaultUnifier
		config:            config,
		logger:            logger,
		endpointStates:    make(map[string]*domain.EndpointStateInfo),
		endpointFailures:  make(map[string]int),
		circuitBreakers:   make(map[string]*CircuitBreaker),
		lastEndpointCheck: make(map[string]time.Time),
		stateTransitions:  make(chan domain.StateTransition, 100),
	}
}

// SetDiscoveryClient sets the discovery client for endpoint health checks
func (u *LifecycleUnifier) SetDiscoveryClient(client DiscoveryClient) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.discoveryClient = client
}

func (u *LifecycleUnifier) Start(ctx context.Context) error {
	if !u.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("unifier is already running")
	}

	u.cleanupCtx, u.cleanupCancel = context.WithCancel(ctx)

	if u.config.EnableBackgroundCleanup {
		u.cleanupWg.Add(1)
		go u.cleanupRoutine()
	}

	if u.config.EnableStateTransitionLogging {
		u.cleanupWg.Add(1)
		go u.stateTransitionLogger()
	}

	u.logger.Info("Lifecycle unifier started",
		"model_ttl", u.config.ModelTTL,
		"cleanup_interval", u.config.CleanupInterval,
		"background_cleanup", u.config.EnableBackgroundCleanup)

	return nil
}

// Stop gracefully shuts down the unifier
func (u *LifecycleUnifier) Stop(ctx context.Context) error {
	if !u.isRunning.CompareAndSwap(true, false) {
		return nil
	}

	u.logger.Info("Stopping lifecycle unifier")

	if u.cleanupCancel != nil {
		u.cleanupCancel()
	}

	done := make(chan struct{})
	go func() {
		u.cleanupWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		u.logger.Info("Lifecycle unifier stopped gracefully")
	case <-ctx.Done():
		u.logger.Warn("Lifecycle unifier stop timeout")
		return ctx.Err()
	}

	close(u.stateTransitions)

	return nil
}

func (u *LifecycleUnifier) UnifyModels(ctx context.Context, models []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error) {
	endpointURL := endpoint.GetURLString()

	if u.config.CircuitBreaker.Enabled {
		cb := u.getOrCreateCircuitBreaker(endpointURL)
		if !cb.Allow() {
			return nil, fmt.Errorf("circuit breaker open for endpoint %s", endpoint.Name)
		}
	}

	// We need to handle the unification ourselves to ensure proper locking
	unified, err := u.unifyModelsWithLock(models, endpoint)
	if err != nil {
		u.recordEndpointFailure(endpointURL, err)
		return nil, err
	}

	u.recordEndpointSuccess(endpointURL)
	u.updateEndpointStates(unified, endpointURL)

	return unified, nil
}

// unifyModelsWithLock performs model unification with proper locking
func (u *LifecycleUnifier) unifyModelsWithLock(models []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error) {
	if models == nil || len(models) == 0 {
		return nil, nil
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Clean up stale models periodically
	if time.Since(u.lastCleanup) > u.cleanupInterval {
		u.cleanupStaleModels()
		u.lastCleanup = time.Now()
	}

	// Use endpoint URL as key internally, but store name for display
	endpointURL := endpoint.GetURLString()

	// Clear previous models from this endpoint
	if oldModels, exists := u.endpointModels[endpointURL]; exists {
		for _, modelID := range oldModels {
			u.removeModelFromEndpoint(modelID, endpointURL, endpoint.Name)
		}
	}
	u.endpointModels[endpointURL] = []string{}

	processedModels := make([]*domain.UnifiedModel, 0, len(models))
	for _, modelInfo := range models {
		if modelInfo == nil {
			continue
		}

		// Convert ModelInfo to Model for processing
		model := u.convertModelInfoToModel(modelInfo)
		unified := u.processModel(model, endpoint)
		if unified != nil {
			processedModels = append(processedModels, unified)
			u.endpointModels[endpointURL] = append(u.endpointModels[endpointURL], unified.ID)
		}
	}

	u.stats.TotalModels = len(u.catalog)
	u.stats.UnifiedModels = len(processedModels)
	u.stats.LastUpdated = time.Now()

	return processedModels, nil
}

func (u *LifecycleUnifier) cleanupRoutine() {
	defer u.cleanupWg.Done()

	ticker := time.NewTicker(u.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-u.cleanupCtx.Done():
			return
		case <-ticker.C:
			u.performCleanup()
		}
	}
}

func (u *LifecycleUnifier) performCleanup() {
	u.mu.Lock()
	defer u.mu.Unlock()

	now := time.Now()
	staleThreshold := now.Add(-u.config.ModelTTL)

	toRemove := []string{}
	for id, model := range u.catalog {
		allStale := true
		for i := range model.SourceEndpoints {
			if model.SourceEndpoints[i].LastSeen.After(staleThreshold) {
				allStale = false
				break
			}
		}

		if allStale {
			toRemove = append(toRemove, id)
		} else {
			u.cleanupStaleEndpoints(model, staleThreshold)
		}
	}

	for _, id := range toRemove {
		u.removeModel(id)
		u.logger.Debug("Removed stale model", "id", id)
	}

	u.cleanupOrphanedStates()

	if len(toRemove) > 0 {
		u.logger.Info("Cleanup completed", "removed_models", len(toRemove))
	}
}

func (u *LifecycleUnifier) cleanupStaleEndpoints(model *domain.UnifiedModel, staleThreshold time.Time) {
	newEndpoints := make([]domain.SourceEndpoint, 0, len(model.SourceEndpoints))

	for _, endpoint := range model.SourceEndpoints {
		if endpoint.LastSeen.After(staleThreshold) {
			newEndpoints = append(newEndpoints, endpoint)
		} else {
			u.recordStateTransition(endpoint.EndpointURL, string(endpoint.GetEffectiveState()),
				string(domain.ModelStateOffline), "TTL expired", nil)
		}
	}

	model.SourceEndpoints = newEndpoints
	model.DiskSize = model.GetTotalDiskSize()
	model.LastSeen = time.Now()
}

// cleanupOrphanedStates prevents memory leaks by removing state data
// for endpoints that no longer have any associated models
func (u *LifecycleUnifier) cleanupOrphanedStates() {
	activeEndpoints := make(map[string]bool)
	for _, model := range u.catalog {
		for _, endpoint := range model.SourceEndpoints {
			activeEndpoints[endpoint.EndpointURL] = true
		}
	}

	for url := range u.endpointStates {
		if !activeEndpoints[url] {
			delete(u.endpointStates, url)
			delete(u.endpointFailures, url)
			delete(u.lastEndpointCheck, url)
			if cb, exists := u.circuitBreakers[url]; exists {
				cb.Reset()
				delete(u.circuitBreakers, url)
			}
		}
	}
}

func (u *LifecycleUnifier) removeModel(id string) {
	model, exists := u.catalog[id]
	if !exists {
		return
	}

	if model.Metadata != nil {
		if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
			u.removeFromIndex(u.digestIndex, digest, id)
		}
	}

	for _, alias := range model.Aliases {
		u.removeFromIndex(u.nameIndex, strings.ToLower(alias.Name), id)
	}

	for _, endpoint := range model.SourceEndpoints {
		u.recordStateTransition(endpoint.EndpointURL, string(endpoint.GetEffectiveState()),
			string(domain.ModelStateOffline), "Model removed", nil)
	}

	delete(u.catalog, id)
}

func (u *LifecycleUnifier) recordEndpointFailure(endpointURL string, err error) {
	u.mu.Lock()

	u.endpointFailures[endpointURL]++
	failures := u.endpointFailures[endpointURL]

	// Circuit breaker needs to be accessed without holding the main lock
	// to avoid deadlock with getOrCreateCircuitBreaker
	var cb *CircuitBreaker
	if u.config.CircuitBreaker.Enabled {
		if existingCB, exists := u.circuitBreakers[endpointURL]; exists {
			cb = existingCB
		} else {
			cb = NewCircuitBreaker(u.config.CircuitBreaker)
			u.circuitBreakers[endpointURL] = cb
		}
	}
	u.mu.Unlock()

	if cb != nil {
		cb.RecordFailure()
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if failures >= u.config.MaxConsecutiveFailures {
		u.markEndpointOffline(endpointURL, fmt.Sprintf("Too many failures: %v", err))
	}

	if state, exists := u.endpointStates[endpointURL]; exists {
		state.ConsecutiveFailures = failures
		state.LastError = err.Error()
	} else {
		u.endpointStates[endpointURL] = &domain.EndpointStateInfo{
			State:               domain.EndpointStateDegraded,
			LastStateChange:     time.Now(),
			ConsecutiveFailures: failures,
			LastError:           err.Error(),
		}
	}
}

func (u *LifecycleUnifier) recordEndpointSuccess(endpointURL string) {
	u.mu.Lock()

	u.endpointFailures[endpointURL] = 0

	// Same pattern as recordEndpointFailure to avoid deadlock
	var cb *CircuitBreaker
	if u.config.CircuitBreaker.Enabled {
		if existingCB, exists := u.circuitBreakers[endpointURL]; exists {
			cb = existingCB
		} else {
			cb = NewCircuitBreaker(u.config.CircuitBreaker)
			u.circuitBreakers[endpointURL] = cb
		}
	}
	u.mu.Unlock()

	if cb != nil {
		cb.RecordSuccess()
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if state, exists := u.endpointStates[endpointURL]; exists {
		if state.State != domain.EndpointStateOnline {
			state.State = domain.EndpointStateOnline
			state.LastStateChange = time.Now()
			state.ConsecutiveFailures = 0
			state.LastError = ""

			u.recordStateTransition(endpointURL, string(domain.EndpointStateOffline),
				string(domain.EndpointStateOnline), "Endpoint recovered", nil)
		}
	} else {
		u.endpointStates[endpointURL] = &domain.EndpointStateInfo{
			State:               domain.EndpointStateOnline,
			LastStateChange:     time.Now(),
			ConsecutiveFailures: 0,
		}
	}

	u.lastEndpointCheck[endpointURL] = time.Now()
}

func (u *LifecycleUnifier) markEndpointOffline(endpointURL string, reason string) {
	if state, exists := u.endpointStates[endpointURL]; exists {
		state.State = domain.EndpointStateOffline
		state.LastStateChange = time.Now()
		state.LastError = fmt.Sprintf("%s", reason)
	} else {
		u.endpointStates[endpointURL] = &domain.EndpointStateInfo{
			State:           domain.EndpointStateOffline,
			LastStateChange: time.Now(),
			LastError:       reason,
		}
	}

	for _, model := range u.catalog {
		model.MarkEndpointOffline(endpointURL, reason)
	}

	u.recordStateTransition(endpointURL, string(domain.EndpointStateOnline),
		string(domain.EndpointStateOffline), reason, fmt.Errorf("%s", reason))
}

func (u *LifecycleUnifier) updateEndpointStates(models []*domain.UnifiedModel, endpointURL string) {
	u.mu.Lock()
	stateInfo := u.endpointStates[endpointURL]

	for _, model := range models {
		for i := range model.SourceEndpoints {
			if model.SourceEndpoints[i].EndpointURL == endpointURL {
				model.SourceEndpoints[i].StateInfo = stateInfo
				model.SourceEndpoints[i].LastStateCheck = time.Now()
			}
		}
	}
	u.mu.Unlock()
}

func (u *LifecycleUnifier) getOrCreateCircuitBreaker(endpointURL string) *CircuitBreaker {
	u.mu.Lock()
	defer u.mu.Unlock()

	if cb, exists := u.circuitBreakers[endpointURL]; exists {
		return cb
	}

	cb := NewCircuitBreaker(u.config.CircuitBreaker)
	u.circuitBreakers[endpointURL] = cb
	return cb
}

func (u *LifecycleUnifier) recordStateTransition(_ /*endpoint*/, from, to, reason string, err error) {
	transition := domain.StateTransition{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Reason:    reason,
		Error:     err,
	}

	select {
	case u.stateTransitions <- transition:
	default:
		// Drop events when channel is full to avoid blocking
	}
}

func (u *LifecycleUnifier) stateTransitionLogger() {
	defer u.cleanupWg.Done()

	for {
		select {
		case <-u.cleanupCtx.Done():
			return
		case transition, ok := <-u.stateTransitions:
			if !ok {
				return
			}
			if transition.Error != nil {
				u.logger.Warn("State transition",
					"from", transition.From,
					"to", transition.To,
					"reason", transition.Reason,
					"error", transition.Error)
			} else {
				u.logger.Debug("State transition",
					"from", transition.From,
					"to", transition.To,
					"reason", transition.Reason)
			}
		}
	}
}

func (u *LifecycleUnifier) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if state, exists := u.endpointStates[endpointURL]; exists {
		state.State = domain.EndpointStateRemoved
		state.LastStateChange = time.Now()
	}

	modelsToRemove := []string{}
	for id, model := range u.catalog {
		if model.RemoveEndpoint(endpointURL) {
			if !model.IsAvailable() {
				modelsToRemove = append(modelsToRemove, id)
			} else {
				model.DiskSize = model.GetTotalDiskSize()
				model.LastSeen = time.Now()
			}
		}
	}

	for _, id := range modelsToRemove {
		u.removeModel(id)
	}

	delete(u.endpointStates, endpointURL)
	delete(u.endpointFailures, endpointURL)
	delete(u.lastEndpointCheck, endpointURL)
	delete(u.endpointModels, endpointURL)

	if cb, exists := u.circuitBreakers[endpointURL]; exists {
		cb.Reset()
		delete(u.circuitBreakers, endpointURL)
	}

	u.logger.Info("Endpoint removed", "url", endpointURL, "models_removed", len(modelsToRemove))

	return nil
}

func (u *LifecycleUnifier) GetEndpointState(endpointURL string) *domain.EndpointStateInfo {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if state, exists := u.endpointStates[endpointURL]; exists {
		return state
	}
	return nil
}

func (u *LifecycleUnifier) ForceEndpointCheck(ctx context.Context, endpointURL string) error {
	u.mu.RLock()
	discoveryClient := u.discoveryClient
	u.mu.RUnlock()

	if discoveryClient == nil {
		return fmt.Errorf("discovery client not configured")
	}

	// Find the endpoint info
	var endpoint *domain.Endpoint
	for _, model := range u.catalog {
		for _, source := range model.SourceEndpoints {
			if source.EndpointURL == endpointURL {
				endpoint = &domain.Endpoint{
					URLString: endpointURL,
					Name:      source.EndpointName,
				}
				break
			}
		}
		if endpoint != nil {
			break
		}
	}

	if endpoint == nil {
		return fmt.Errorf("endpoint not found: %s", endpointURL)
	}

	// Perform discovery with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := discoveryClient.DiscoverModels(checkCtx, endpoint)
	if err != nil {
		u.recordEndpointFailure(endpointURL, err)
		return fmt.Errorf("endpoint check failed: %w", err)
	}

	// Success - update models
	u.recordEndpointSuccess(endpointURL)

	// Re-unify the models to update states
	_, unifyErr := u.UnifyModels(ctx, models, endpoint)
	if unifyErr != nil {
		return fmt.Errorf("failed to update models: %w", unifyErr)
	}

	u.logger.Info("Endpoint check completed", "url", endpointURL, "models", len(models))
	return nil
}

// RecordEndpointFailure allows external components to report endpoint failures
func (u *LifecycleUnifier) RecordEndpointFailure(endpointURL string, err error) {
	u.recordEndpointFailure(endpointURL, err)
}

// GetCircuitBreakerStats returns statistics for all circuit breakers
func (u *LifecycleUnifier) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for url, cb := range u.circuitBreakers {
		stats[url] = cb.GetStats()
	}
	return stats
}

// GetCircuitBreakerStatsForEndpoint returns circuit breaker stats for a specific endpoint
func (u *LifecycleUnifier) GetCircuitBreakerStatsForEndpoint(endpointURL string) (CircuitBreakerStats, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if cb, exists := u.circuitBreakers[endpointURL]; exists {
		return cb.GetStats(), true
	}
	return CircuitBreakerStats{}, false
}

// ResolveModel overrides DefaultUnifier to ensure proper locking
func (u *LifecycleUnifier) ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	// Direct access to catalog with proper locking
	if model, exists := u.catalog[nameOrID]; exists {
		return model, nil
	}

	// Try case-insensitive name lookup
	lowercaseName := strings.ToLower(nameOrID)
	if modelIDs, exists := u.nameIndex[lowercaseName]; exists && len(modelIDs) > 0 {
		return u.catalog[modelIDs[0]], nil
	}

	// Try alias lookup
	for _, model := range u.catalog {
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias.Name, nameOrID) {
				return model, nil
			}
		}
	}

	return nil, fmt.Errorf("model not found: %s", nameOrID)
}

// ResolveAlias overrides DefaultUnifier to ensure proper locking
func (u *LifecycleUnifier) ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error) {
	return u.ResolveModel(ctx, alias)
}

// GetAllModels overrides DefaultUnifier to ensure proper locking
func (u *LifecycleUnifier) GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	models := make([]*domain.UnifiedModel, 0, len(u.catalog))
	for _, model := range u.catalog {
		models = append(models, model)
	}
	return models, nil
}

// UnifyModel overrides DefaultUnifier to ensure proper locking
func (u *LifecycleUnifier) UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpoint *domain.Endpoint) (*domain.UnifiedModel, error) {
	if sourceModel == nil {
		return nil, nil
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	model := u.convertModelInfoToModel(sourceModel)
	return u.processModel(model, endpoint), nil
}
