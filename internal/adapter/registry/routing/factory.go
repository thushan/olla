package routing

import (
	"sync"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	StrategyStrict     = "strict"
	StrategyOptimistic = "optimistic"
	StrategyDiscovery  = "discovery"
)

// Factory creates model routing strategy instances
type Factory struct {
	creators map[string]func(config.ModelRoutingStrategyOptions, ports.DiscoveryService, logger.StyledLogger) ports.ModelRoutingStrategy
	logger   logger.StyledLogger
	mu       sync.RWMutex
}

// NewFactory creates a new routing strategy factory with default registrations
func NewFactory(log logger.StyledLogger) *Factory {
	factory := &Factory{
		creators: make(map[string]func(config.ModelRoutingStrategyOptions, ports.DiscoveryService, logger.StyledLogger) ports.ModelRoutingStrategy),
		logger:   log,
	}

	// register default strategies
	factory.Register(StrategyStrict, func(opts config.ModelRoutingStrategyOptions, discovery ports.DiscoveryService, log logger.StyledLogger) ports.ModelRoutingStrategy {
		return NewStrictStrategy(log)
	})

	factory.Register(StrategyOptimistic, func(opts config.ModelRoutingStrategyOptions, discovery ports.DiscoveryService, log logger.StyledLogger) ports.ModelRoutingStrategy {
		return NewOptimisticStrategy(opts.FallbackBehavior, log)
	})

	factory.Register(StrategyDiscovery, func(opts config.ModelRoutingStrategyOptions, discovery ports.DiscoveryService, log logger.StyledLogger) ports.ModelRoutingStrategy {
		return NewDiscoveryStrategy(discovery, opts, log)
	})

	return factory
}

// Register adds a new strategy type to the factory
func (f *Factory) Register(name string, creator func(config.ModelRoutingStrategyOptions, ports.DiscoveryService, logger.StyledLogger) ports.ModelRoutingStrategy) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[name] = creator
}

// Create instantiates a strategy of the specified type
func (f *Factory) Create(strategyConfig config.ModelRoutingStrategy, discovery ports.DiscoveryService) (ports.ModelRoutingStrategy, error) {
	f.mu.RLock()
	creator, exists := f.creators[strategyConfig.Type]
	f.mu.RUnlock()

	if !exists {
		// default to strict if unknown
		f.logger.Warn("Unknown routing strategy type, defaulting to strict", "type", strategyConfig.Type)
		return NewStrictStrategy(f.logger), nil
	}

	return creator(strategyConfig.Options, discovery, f.logger), nil
}

// GetAvailableStrategies returns all registered strategy types
func (f *Factory) GetAvailableStrategies() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	strategies := make([]string, 0, len(f.creators))
	for name := range f.creators {
		strategies = append(strategies, name)
	}
	return strategies
}
