package registry

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// LifecycleUnifiedRegistry implements ManagedService for proper lifecycle integration
// with the service manager. It wraps UnifiedMemoryModelRegistry with retry logic and
// endpoint health tracking.
type LifecycleUnifiedRegistry struct {
	*UnifiedMemoryModelRegistry
	unifierConfig unifier.Config
	logger        logger.StyledLogger
	isRunning     atomic.Bool
}

func NewLifecycleUnifiedRegistry(logger logger.StyledLogger, config unifier.Config) *LifecycleUnifiedRegistry {
	unifierFactory := unifier.NewFactory(logger)
	modelUnifier, _ := unifierFactory.CreateWithConfig(unifier.LifecycleUnifierType, config)

	base := &UnifiedMemoryModelRegistry{
		MemoryModelRegistry: NewMemoryModelRegistry(logger),
		unifier:             modelUnifier,
		unifiedModels:       xsync.NewMap[string, *domain.UnifiedModel](),
		globalUnified:       xsync.NewMap[string, *domain.UnifiedModel](),
		endpoints:           xsync.NewMap[string, *domain.Endpoint](),
	}

	return &LifecycleUnifiedRegistry{
		UnifiedMemoryModelRegistry: base,
		unifierConfig:             config,
		logger:                    logger,
	}
}

func (r *LifecycleUnifiedRegistry) Name() string {
	return "lifecycle-unified-registry"
}

func (r *LifecycleUnifiedRegistry) Start(ctx context.Context) error {
	if !r.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("registry is already running")
	}

	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		if err := lifecycleUnifier.Start(ctx); err != nil {
			r.isRunning.Store(false)
			return fmt.Errorf("failed to start unifier: %w", err)
		}
	}

	r.logger.Info("Lifecycle registry started")
	return nil
}

func (r *LifecycleUnifiedRegistry) Stop(ctx context.Context) error {
	if !r.isRunning.CompareAndSwap(true, false) {
		return nil
	}

	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		if err := lifecycleUnifier.Stop(ctx); err != nil {
			r.logger.Warn("Error stopping unifier", "error", err)
		}
	}

	r.logger.Info("Lifecycle registry stopped")
	return nil
}

func (r *LifecycleUnifiedRegistry) Dependencies() []string {
	return []string{}
}

// RegisterModelsWithEndpoint adds retry logic to handle transient failures
func (r *LifecycleUnifiedRegistry) RegisterModelsWithEndpoint(ctx context.Context, endpoint *domain.Endpoint, models []*domain.ModelInfo) error {
	retryPolicy := r.unifierConfig.DiscoveryRetryPolicy
	
	err := unifier.Retry(ctx, retryPolicy, func(ctx context.Context) error {
		return r.UnifiedMemoryModelRegistry.RegisterModelsWithEndpoint(ctx, endpoint, models)
	})

	if err != nil {
		// Track failures for circuit breaker decisions
		if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
			lifecycleUnifier.RecordEndpointFailure(endpoint.GetURLString(), err)
		}
		return err
	}

	return nil
}

func (r *LifecycleUnifiedRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	// Unifier needs to clean up state before registry removes models
	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		if err := lifecycleUnifier.RemoveEndpoint(ctx, endpointURL); err != nil {
			r.logger.Warn("Error removing endpoint from unifier", "url", endpointURL, "error", err)
		}
	}

	return r.UnifiedMemoryModelRegistry.RemoveEndpoint(ctx, endpointURL)
}

func (r *LifecycleUnifiedRegistry) GetEndpointHealth(ctx context.Context) (map[string]*domain.EndpointStateInfo, error) {
	health := make(map[string]*domain.EndpointStateInfo)

	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		r.endpoints.Range(func(url string, endpoint *domain.Endpoint) bool {
			if state := lifecycleUnifier.GetEndpointState(url); state != nil {
				health[url] = state
			} else {
				health[url] = &domain.EndpointStateInfo{
					State:           domain.EndpointStateUnknown,
					LastStateChange: time.Now(),
				}
			}
			return true
		})
	}

	return health, nil
}

func (r *LifecycleUnifiedRegistry) MarkEndpointOffline(ctx context.Context, endpointURL string, reason string) error {
	r.globalUnified.Range(func(id string, model *domain.UnifiedModel) bool {
		model.MarkEndpointOffline(endpointURL, reason)
		return true
	})

	return nil
}

func (r *LifecycleUnifiedRegistry) ForceEndpointCheck(ctx context.Context, endpointURL string) error {
	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		return lifecycleUnifier.ForceEndpointCheck(ctx, endpointURL)
	}
	return fmt.Errorf("unifier does not support endpoint checks")
}

func (r *LifecycleUnifiedRegistry) GetLifecycleStats(ctx context.Context) (LifecycleRegistryStats, error) {
	baseStats, err := r.GetUnifiedStats(ctx)
	if err != nil {
		return LifecycleRegistryStats{}, err
	}

	endpointHealth, err := r.GetEndpointHealth(ctx)
	if err != nil {
		return LifecycleRegistryStats{}, err
	}

	stateCount := make(map[domain.EndpointState]int)
	for _, state := range endpointHealth {
		stateCount[state.State]++
	}

	var circuitBreakerStats map[string]unifier.CircuitBreakerStats
	if lifecycleUnifier, ok := r.unifier.(*unifier.LifecycleUnifier); ok {
		circuitBreakerStats = lifecycleUnifier.GetCircuitBreakerStats()
	}

	return LifecycleRegistryStats{
		UnifiedRegistryStats: baseStats,
		EndpointStates:      stateCount,
		CircuitBreakers:     circuitBreakerStats,
		LastCleanup:         time.Now(), // TODO: Track actual cleanup time
	}, nil
}

type LifecycleRegistryStats struct {
	UnifiedRegistryStats
	EndpointStates  map[domain.EndpointState]int               `json:"endpoint_states"`
	CircuitBreakers map[string]unifier.CircuitBreakerStats     `json:"circuit_breakers"`
	LastCleanup     time.Time                                   `json:"last_cleanup"`
}