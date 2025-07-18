package unifier

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

type DiscoveryClient interface {
	DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error)
}

// LifecycleUnifier manages model lifecycle with automatic cleanup,
// health monitoring, and circuit breaker support
type LifecycleUnifier struct {
	unifier         ports.ModelUnifier
	discoveryClient DiscoveryClient
	logger          logger.StyledLogger

	cleanupCtx       context.Context
	endpointManager  *EndpointManager
	stateTransitions chan domain.StateTransition

	cleanupCancel context.CancelFunc
	config        Config
	cleanupWg     sync.WaitGroup
	isRunning     atomic.Bool
}

func NewLifecycleUnifier(config Config, logger logger.StyledLogger) ports.ModelUnifier {
	if err := config.Validate(); err != nil {
		config = DefaultConfig()
	}

	return &LifecycleUnifier{
		unifier:          NewDefaultUnifier(),
		endpointManager:  NewEndpointManager(config, logger),
		config:           config,
		logger:           logger,
		stateTransitions: make(chan domain.StateTransition, 100),
	}
}

func (u *LifecycleUnifier) SetDiscoveryClient(client DiscoveryClient) {
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

	// Circuit breaker prevents cascading failures
	if u.config.CircuitBreaker.Enabled {
		cb := u.endpointManager.GetCircuitBreaker(endpointURL)
		if cb != nil && !cb.Allow() {
			return nil, fmt.Errorf("circuit breaker open for endpoint %s", endpoint.Name)
		}
	}

	unified, err := u.unifier.UnifyModels(ctx, models, endpoint)
	if err != nil {
		u.endpointManager.RecordFailure(endpointURL, err)
		return nil, err
	}

	u.endpointManager.RecordSuccess(endpointURL)
	u.updateEndpointStates(unified, endpointURL)

	return unified, nil
}

func (u *LifecycleUnifier) updateEndpointStates(models []*domain.UnifiedModel, endpointURL string) {
	stateInfo := u.endpointManager.GetState(endpointURL)
	if stateInfo == nil {
		return
	}

	// Models from UnifyModels are fresh instances, safe to modify
	for _, model := range models {
		for i := range model.SourceEndpoints {
			if model.SourceEndpoints[i].EndpointURL == endpointURL {
				model.SourceEndpoints[i].StateInfo = stateInfo
				model.SourceEndpoints[i].LastStateCheck = time.Now()
			}
		}
	}
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
	// Trigger model TTL cleanup in the underlying unifier
	if defaultUnifier, ok := u.unifier.(*DefaultUnifier); ok {
		defaultUnifier.cleanupStaleModels(u.config.ModelTTL)
	}
	activeEndpoints := make(map[string]bool)

	if extUnifier, ok := u.unifier.(ExtendedUnifier); ok {
		models, _ := extUnifier.GetAllModels(context.Background())
		for _, model := range models {
			for _, endpoint := range model.SourceEndpoints {
				activeEndpoints[endpoint.EndpointURL] = true
			}
		}
	}

	u.endpointManager.CleanupOrphaned(activeEndpoints)
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

func (u *LifecycleUnifier) recordStateTransition(from, to, reason string, err error) {
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
	}
}

func (u *LifecycleUnifier) UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpoint *domain.Endpoint) (*domain.UnifiedModel, error) {
	models := []*domain.ModelInfo{sourceModel}
	unified, err := u.UnifyModels(ctx, models, endpoint)
	if err != nil {
		return nil, err
	}
	if len(unified) > 0 {
		return unified[0], nil
	}
	return nil, nil
}

func (u *LifecycleUnifier) ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error) {
	if extUnifier, ok := u.unifier.(ExtendedUnifier); ok {
		return extUnifier.ResolveModel(ctx, nameOrID)
	}
	return u.unifier.ResolveAlias(ctx, nameOrID)
}

func (u *LifecycleUnifier) ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error) {
	return u.unifier.ResolveAlias(ctx, alias)
}

func (u *LifecycleUnifier) GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	if extUnifier, ok := u.unifier.(ExtendedUnifier); ok {
		return extUnifier.GetAllModels(ctx)
	}
	return []*domain.UnifiedModel{}, nil
}

func (u *LifecycleUnifier) GetAliases(ctx context.Context, unifiedID string) ([]string, error) {
	return u.unifier.GetAliases(ctx, unifiedID)
}

func (u *LifecycleUnifier) RegisterCustomRule(platformType string, rule ports.UnificationRule) error {
	return u.unifier.RegisterCustomRule(platformType, rule)
}

func (u *LifecycleUnifier) GetStats() domain.UnificationStats {
	return u.unifier.GetStats()
}

func (u *LifecycleUnifier) MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error) {
	return u.unifier.MergeUnifiedModels(ctx, models)
}

func (u *LifecycleUnifier) Clear(ctx context.Context) error {
	if err := u.unifier.Clear(ctx); err != nil {
		return err
	}

	u.endpointManager = NewEndpointManager(u.config, u.logger)

	return nil
}

// RemoveEndpoint removes an endpoint and its associated models
func (u *LifecycleUnifier) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	// Simulate empty model list to trigger cleanup in UnifyModels
	var endpointInfo *domain.Endpoint
	if extUnifier, ok := u.unifier.(ExtendedUnifier); ok {
		models, _ := extUnifier.GetAllModels(ctx)
		for _, model := range models {
			for _, source := range model.SourceEndpoints {
				if source.EndpointURL == endpointURL {
					endpointInfo = &domain.Endpoint{
						URLString: endpointURL,
						Name:      source.EndpointName,
					}
					break
				}
			}
			if endpointInfo != nil {
				break
			}
		}
	}

	if endpointInfo != nil {
		// Bypass circuit breaker for endpoint removal
		_, _ = u.unifier.UnifyModels(ctx, []*domain.ModelInfo{}, endpointInfo)
	}

	u.endpointManager.RemoveEndpoint(endpointURL)

	u.logger.Info("Endpoint removed", "url", endpointURL)
	return nil
}

func (u *LifecycleUnifier) GetEndpointState(endpointURL string) *domain.EndpointStateInfo {
	return u.endpointManager.GetState(endpointURL)
}

func (u *LifecycleUnifier) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return u.endpointManager.GetCircuitBreakerStats()
}

func (u *LifecycleUnifier) RecordEndpointFailure(endpointURL string, err error) {
	u.endpointManager.RecordFailure(endpointURL, err)
}

func (u *LifecycleUnifier) ForceEndpointCheck(ctx context.Context, endpointURL string) error {
	if u.discoveryClient == nil {
		return fmt.Errorf("discovery client not configured")
	}

	var endpoint *domain.Endpoint

	if extUnifier, ok := u.unifier.(ExtendedUnifier); ok {
		models, _ := extUnifier.GetAllModels(ctx)

		for _, model := range models {
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
	}

	if endpoint == nil {
		return fmt.Errorf("endpoint not found: %s", endpointURL)
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	discoveredModels, err := u.discoveryClient.DiscoverModels(checkCtx, endpoint)
	if err != nil {
		u.endpointManager.RecordFailure(endpointURL, err)
		return fmt.Errorf("endpoint check failed: %w", err)
	}

	_, unifyErr := u.UnifyModels(ctx, discoveredModels, endpoint)
	if unifyErr != nil {
		return fmt.Errorf("failed to update models: %w", unifyErr)
	}

	u.logger.Info("Endpoint check completed", "url", endpointURL, "models", len(discoveredModels))
	return nil
}
