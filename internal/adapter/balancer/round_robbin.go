package balancer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/thushan/olla/internal/core/domain"
)

type RoundRobinSelector struct {
	counter     int64
	connections map[string]int64
	mu          sync.RWMutex
}

func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{
		connections: make(map[string]int64),
	}
}

func (r *RoundRobinSelector) Name() string {
	return "round-robin"
}
// Select chooses endpoints in a round-robin fashion, filtering out non-routable endpoints
func (r *RoundRobinSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
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

	// Round-robin selection
	index := atomic.AddInt64(&r.counter, 1) % int64(len(routable))
	return routable[index], nil
}

func (r *RoundRobinSelector) IncrementConnections(endpoint *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	r.connections[key]++
}

func (r *RoundRobinSelector) DecrementConnections(endpoint *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	if count, exists := r.connections[key]; exists && count > 0 {
		r.connections[key]--
	}
}

func (r *RoundRobinSelector) GetConnectionCount(endpoint *domain.Endpoint) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := endpoint.URL.String()
	return r.connections[key]
}

func (r *RoundRobinSelector) GetConnectionStats() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]int64)
	for endpoint, count := range r.connections {
		stats[endpoint] = count
	}

	return stats
}