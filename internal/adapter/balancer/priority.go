package balancer

import (
	"context"
	"fmt"
	"math/rand"
	"sort"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// PrioritySelector implements priority-based endpoint selection with connection tracking
type PrioritySelector struct {
	statsCollector ports.StatsCollector
}

// NewPrioritySelector creates a new priority-based endpoint selector
func NewPrioritySelector(statsCollector ports.StatsCollector) *PrioritySelector {
	return &PrioritySelector{
		statsCollector: statsCollector,
	}
}

// Name returns the name of the selection strategy
func (p *PrioritySelector) Name() string {
	return DefaultBalancerPriority
}

// Select chooses the highest priority routable endpoint with weighted selection for non-healthy statuses
func (p *PrioritySelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Filter only routable endpoints
	routable := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status.IsRoutable() {
			routable = append(routable, endpoint)
		}
	}

	if len(routable) == 0 {
		return nil, fmt.Errorf("no routable endpoints available")
	}

	// Sort by priority (highest first)
	sort.Slice(routable, func(i, j int) bool {
		return routable[i].Priority > routable[j].Priority
	})

	// Get the highest priority tier
	highestPriority := routable[0].Priority
	highestPriorityEndpoints := make([]*domain.Endpoint, 0)

	for _, endpoint := range routable {
		if endpoint.Priority == highestPriority {
			highestPriorityEndpoints = append(highestPriorityEndpoints, endpoint)
		} else {
			break // Since sorted, we can break early
		}
	}

	// If only one endpoint at highest priority, return it
	if len(highestPriorityEndpoints) == 1 {
		return highestPriorityEndpoints[0], nil
	}

	// Multiple endpoints at same priority - use weighted selection
	return p.weightedSelect(highestPriorityEndpoints), nil
}

// weightedSelect performs weighted selection based on endpoint status
func (p *PrioritySelector) weightedSelect(endpoints []*domain.Endpoint) *domain.Endpoint {
	if len(endpoints) == 1 {
		return endpoints[0]
	}

	// Calculate total weight
	totalWeight := 0.0
	for _, endpoint := range endpoints {
		totalWeight += endpoint.Status.GetTrafficWeight()
	}

	// All endpoints have 0 weight, fallback to random selection
	if totalWeight == 0 {
		//nolint:gosec // fallback to non-secure random is fine here for endpoint shuffling
		return endpoints[rand.Intn(len(endpoints))]
	}

	// Weighted random selection
	//nolint:gosec // pseudo-random is okay for load balancing
	r := rand.Float64() * totalWeight
	weightSum := 0.0

	for _, endpoint := range endpoints {
		weightSum += endpoint.Status.GetTrafficWeight()
		if r <= weightSum {
			return endpoint
		}
	}

	// Fallback (shouldn't reach here)
	return endpoints[len(endpoints)-1]
}

// IncrementConnections increments the connection count for an endpoint
func (p *PrioritySelector) IncrementConnections(endpoint *domain.Endpoint) {
	p.statsCollector.RecordConnection(endpoint.URL.String(), 1)
}

// DecrementConnections decrements the connection count for an endpoint
func (p *PrioritySelector) DecrementConnections(endpoint *domain.Endpoint) {
	p.statsCollector.RecordConnection(endpoint.URL.String(), -1)
}
