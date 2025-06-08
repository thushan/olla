package balancer

import (
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

const DefaultBalancerPriority = "priority"
const DefaultBalancerRoundRobin = "round-robin"
const DefaultBalancerLeastConnections = "least-connections"

type Factory struct {
	creators       map[string]func(ports.StatsCollector) domain.EndpointSelector
	statsCollector ports.StatsCollector
	mu             sync.RWMutex
}

func NewFactory(statsCollector ports.StatsCollector) *Factory {
	factory := &Factory{
		creators:       make(map[string]func(ports.StatsCollector) domain.EndpointSelector),
		statsCollector: statsCollector,
	}

	factory.Register(DefaultBalancerPriority, func(collector ports.StatsCollector) domain.EndpointSelector {
		return NewPrioritySelector(collector)
	})
	factory.Register(DefaultBalancerRoundRobin, func(collector ports.StatsCollector) domain.EndpointSelector {
		return NewRoundRobinSelector(collector)
	})
	factory.Register(DefaultBalancerLeastConnections, func(collector ports.StatsCollector) domain.EndpointSelector {
		return NewLeastConnectionsSelector(collector)
	})

	return factory
}

func (f *Factory) Register(name string, creator func(ports.StatsCollector) domain.EndpointSelector) {
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

	return creator(f.statsCollector), nil
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
