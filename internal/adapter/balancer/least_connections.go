package balancer

import (
	"context"
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
)

// LeastConnectionsSelector implements a load balancer that selects the endpoint with the least number of active connections.
type LeastConnectionsSelector struct {
	connections map[string]int64
	mu          sync.RWMutex
}

func NewLeastConnectionsSelector() *LeastConnectionsSelector {
	return &LeastConnectionsSelector{
		connections: make(map[string]int64),
	}
}

func (l *LeastConnectionsSelector) Name() string {
	return DefaultBalancerLeastConnections
}

func (l *LeastConnectionsSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
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

	l.mu.RLock()
	defer l.mu.RUnlock()

	// Find endpoint with least number of connections
	var selected *domain.Endpoint
	minConnections := int64(-1)

	for _, endpoint := range routable {
		key := endpoint.URL.String()
		connections := l.connections[key]

		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = endpoint
		}
	}

	return selected, nil
}

func (l *LeastConnectionsSelector) IncrementConnections(endpoint *domain.Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := endpoint.URL.String()
	l.connections[key]++
}

func (l *LeastConnectionsSelector) DecrementConnections(endpoint *domain.Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := endpoint.URL.String()
	if count, exists := l.connections[key]; exists && count > 0 {
		l.connections[key]--
	}
}

func (l *LeastConnectionsSelector) GetConnectionCount(endpoint *domain.Endpoint) int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	key := endpoint.URL.String()
	return l.connections[key]
}

func (l *LeastConnectionsSelector) GetConnectionStats() map[string]int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := make(map[string]int64, len(l.connections))
	for endpoint, count := range l.connections {
		stats[endpoint] = count
	}

	return stats
}
