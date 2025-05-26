package balancer

import (
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
)

type Factory struct {
	creators map[string]func() domain.EndpointSelector
	mu       sync.RWMutex
}

func NewFactory() *Factory {
	factory := &Factory{
		creators: make(map[string]func() domain.EndpointSelector),
	}

	// Register default strategies
	factory.Register("round-robin", func() domain.EndpointSelector {
		return NewRoundRobinSelector()
	})

	return factory
}

func (f *Factory) Register(name string, creator func() domain.EndpointSelector) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[name] = creator
}

func (f *Factory) Create(name string) (domain.EndpointSelector, error) {
	f.mu.RLock()
	creator, exists := f.creators[name]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown load balancer strategy: %s", name)
	}

	return creator(), nil
}

func (f *Factory) GetAvailableStrategies() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	strategies := make([]string, 0, len(f.creators))
	for name := range f.creators {
		strategies = append(strategies, name)
	}
	return strategies
}