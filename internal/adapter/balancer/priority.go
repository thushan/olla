package balancer

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
)

// PrioritySelector implements priority-based endpoint selection with connection tracking
type PrioritySelector struct {
	connections map[string]int64
	mu          sync.RWMutex
}

// NewPrioritySelector creates a new priority-based endpoint selector
func NewPrioritySelector() *PrioritySelector {
	return &PrioritySelector{
		connections: make(map[string]int64),
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

	if totalWeight == 0 {
		// All endpoints have 0 weight, fallback to random selection
		return endpoints[rand.Intn(len(endpoints))]
	}

	// Weighted random selection
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
	p.mu.Lock()
	defer p.mu.Unlock()

	key := endpoint.URL.String()
	p.connections[key]++
}

// DecrementConnections decrements the connection count for an endpoint
func (p *PrioritySelector) DecrementConnections(endpoint *domain.Endpoint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := endpoint.URL.String()
	if count, exists := p.connections[key]; exists && count > 0 {
		p.connections[key]--
	}
}

// GetConnectionCount returns the current connection count for an endpoint
func (p *PrioritySelector) GetConnectionCount(endpoint *domain.Endpoint) int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := endpoint.URL.String()
	return p.connections[key]
}

// GetConnectionStats returns connection statistics for all tracked endpoints
func (p *PrioritySelector) GetConnectionStats() map[string]int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[string]int64)
	for endpoint, count := range p.connections {
		stats[endpoint] = count
	}

	return stats
}
