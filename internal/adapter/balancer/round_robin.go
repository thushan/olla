package balancer

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

type RoundRobinSelector struct {
	statsCollector ports.StatsCollector
	counter        uint64
}

func NewRoundRobinSelector(statsCollector ports.StatsCollector) *RoundRobinSelector {
	return &RoundRobinSelector{
		statsCollector: statsCollector,
	}
}

func (r *RoundRobinSelector) Name() string {
	return DefaultBalancerRoundRobin
}

// Select chooses endpoints in a round-robin fashion, filtering out non-routable endpoints
func (r *RoundRobinSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	routable := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status.IsRoutable() {
			routable = append(routable, endpoint)
		}
	}

	if len(routable) == 0 {
		return nil, fmt.Errorf("no routable endpoints available")
	}

	current := atomic.AddUint64(&r.counter, 1) - 1 // Subtract 1 to start from 0
	index := current % uint64(len(routable))

	return routable[index], nil
}

func (r *RoundRobinSelector) IncrementConnections(endpoint *domain.Endpoint) {
	r.statsCollector.RecordConnection(endpoint.URL.String(), 1)
}

func (r *RoundRobinSelector) DecrementConnections(endpoint *domain.Endpoint) {
	r.statsCollector.RecordConnection(endpoint.URL.String(), -1)
}
