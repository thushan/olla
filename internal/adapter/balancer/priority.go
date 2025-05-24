package balancer

import (
	"context"
	"fmt"
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
	return "priority"
}

// Select chooses the highest priority healthy endpoint
func (p *PrioritySelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Filter only healthy endpoints
	healthy := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status == domain.StatusHealthy {
			healthy = append(healthy, endpoint)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy endpoints available")
	}

	// Sort by priority (highest first)
	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].Priority > healthy[j].Priority
	})

	// Return the highest priority endpoint
	return healthy[0], nil
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

// RoundRobinSelector implements round-robin endpoint selection
type RoundRobinSelector struct {
	connections map[string]int64
	counter     int64
	mu          sync.RWMutex
}

// NewRoundRobinSelector creates a new round-robin endpoint selector
func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{
		connections: make(map[string]int64),
	}
}

// Name returns the name of the selection strategy
func (r *RoundRobinSelector) Name() string {
	return "round_robin"
}

// Select chooses the next endpoint in round-robin fashion
func (r *RoundRobinSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Filter only healthy endpoints
	healthy := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status == domain.StatusHealthy {
			healthy = append(healthy, endpoint)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy endpoints available")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Select next endpoint in round-robin fashion
	index := r.counter % int64(len(healthy))
	r.counter++

	return healthy[index], nil
}

// IncrementConnections increments the connection count for an endpoint
func (r *RoundRobinSelector) IncrementConnections(endpoint *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	r.connections[key]++
}

// DecrementConnections decrements the connection count for an endpoint
func (r *RoundRobinSelector) DecrementConnections(endpoint *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	if count, exists := r.connections[key]; exists && count > 0 {
		r.connections[key]--
	}
}

// LeastConnectionsSelector implements least connections endpoint selection
type LeastConnectionsSelector struct {
	connections map[string]int64
	mu          sync.RWMutex
}

// NewLeastConnectionsSelector creates a new least connections endpoint selector
func NewLeastConnectionsSelector() *LeastConnectionsSelector {
	return &LeastConnectionsSelector{
		connections: make(map[string]int64),
	}
}

// Name returns the name of the selection strategy
func (l *LeastConnectionsSelector) Name() string {
	return "least_connections"
}

// Select chooses the endpoint with the fewest active connections
func (l *LeastConnectionsSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Filter only healthy endpoints
	healthy := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status == domain.StatusHealthy {
			healthy = append(healthy, endpoint)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy endpoints available")
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	// Find endpoint with least connections
	var selected *domain.Endpoint
	minConnections := int64(-1)

	for _, endpoint := range healthy {
		key := endpoint.URL.String()
		connections := l.connections[key]

		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = endpoint
		}
	}

	return selected, nil
}

// IncrementConnections increments the connection count for an endpoint
func (l *LeastConnectionsSelector) IncrementConnections(endpoint *domain.Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := endpoint.URL.String()
	l.connections[key]++
}

// DecrementConnections decrements the connection count for an endpoint
func (l *LeastConnectionsSelector) DecrementConnections(endpoint *domain.Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := endpoint.URL.String()
	if count, exists := l.connections[key]; exists && count > 0 {
		l.connections[key]--
	}
}
