package unifier

import (
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultUnifierType   = "default"
	LifecycleUnifierType = "lifecycle"
)

// Factory creates model unifier instances
type Factory struct {
	creators map[string]func(log logger.StyledLogger) ports.ModelUnifier
	logger   logger.StyledLogger
	mu       sync.RWMutex
}

// NewFactory creates a new unifier factory with default registrations
func NewFactory(log logger.StyledLogger) *Factory {
	factory := &Factory{
		creators: make(map[string]func(log logger.StyledLogger) ports.ModelUnifier),
		logger:   log,
	}

	// Register default unifier
	factory.Register(DefaultUnifierType, func(l logger.StyledLogger) ports.ModelUnifier {
		return NewDefaultUnifier()
	})

	// Register lifecycle unifier
	factory.Register(LifecycleUnifierType, func(l logger.StyledLogger) ports.ModelUnifier {
		return NewLifecycleUnifier(DefaultConfig(), l)
	})

	return factory
}

// Register adds a new unifier type to the factory
func (f *Factory) Register(name string, creator func(log logger.StyledLogger) ports.ModelUnifier) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[name] = creator
}

// Create instantiates a unifier of the specified type
func (f *Factory) Create(name string) (ports.ModelUnifier, error) {
	f.mu.RLock()
	creator, exists := f.creators[name]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown unifier type: %s", name)
	}

	return creator(f.logger), nil
}

// GetAvailableTypes returns all registered unifier types
func (f *Factory) GetAvailableTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.creators))
	for name := range f.creators {
		types = append(types, name)
	}
	return types
}

// CreateWithConfig creates a unifier with custom configuration
func (f *Factory) CreateWithConfig(name string, config Config) (ports.ModelUnifier, error) {
	switch name {
	case LifecycleUnifierType:
		return NewLifecycleUnifier(config, f.logger), nil
	case DefaultUnifierType:
		return NewDefaultUnifier(), nil
	default:
		return f.Create(name)
	}
}

// CreateLifecycleUnifierWithDiscovery creates a lifecycle unifier with discovery client
func (f *Factory) CreateLifecycleUnifierWithDiscovery(config Config, discoveryClient DiscoveryClient) (ports.ModelUnifier, error) {
	unifier := NewLifecycleUnifier(config, f.logger)
	if lifecycleUnifier, ok := unifier.(*LifecycleUnifier); ok {
		lifecycleUnifier.SetDiscoveryClient(discoveryClient)
	}
	return unifier, nil
}
