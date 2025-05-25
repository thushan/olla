package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"sync"
	"time"
)

// StaticEndpointRepository implements domain.EndpointRepository for static endpoints
type StaticEndpointRepository struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex
}

func NewStaticEndpointRepository() *StaticEndpointRepository {
	return &StaticEndpointRepository{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

// GetAll returns all registered endpoints
func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return []*domain.Endpoint{}, nil
	}

	endpoints := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		endpointCopy := *endpoint
		endpoints = append(endpoints, &endpointCopy)
	}
	return endpoints, nil
}

// GetHealthy returns only healthy endpoints
func (r *StaticEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endpoints := make([]*domain.Endpoint, 0)
	for _, endpoint := range r.endpoints {
		if endpoint.Status == domain.StatusHealthy {
			endpointCopy := *endpoint
			endpoints = append(endpoints, &endpointCopy)
		}
	}
	return endpoints, nil
}

// GetRoutable returns endpoints that can receive traffic
func (r *StaticEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endpoints := make([]*domain.Endpoint, 0)
	for _, endpoint := range r.endpoints {
		if endpoint.Status.IsRoutable() {
			endpointCopy := *endpoint
			endpoints = append(endpoints, &endpointCopy)
		}
	}
	return endpoints, nil
}

// UpdateStatus updates the health status of an endpoint
func (r *StaticEndpointRepository) UpdateStatus(ctx context.Context, endpointURL *url.URL, status domain.EndpointStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpointURL.String()
	endpoint, exists := r.endpoints[key]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	endpoint.Status = status
	endpoint.LastChecked = time.Now()
	return nil
}

// UpdateEndpoint updates endpoint state including backoff and timing
func (r *StaticEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	existing, exists := r.endpoints[key]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	// Update the existing endpoint with new state
	existing.Status = endpoint.Status
	existing.LastChecked = endpoint.LastChecked
	existing.ConsecutiveFailures = endpoint.ConsecutiveFailures
	existing.BackoffMultiplier = endpoint.BackoffMultiplier
	existing.NextCheckTime = endpoint.NextCheckTime
	existing.LastLatency = endpoint.LastLatency

	return nil
}

// Add adds a new endpoint to the repository
func (r *StaticEndpointRepository) Add(ctx context.Context, endpoint *domain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	// Initialize state fields for new endpoints
	if endpoint.BackoffMultiplier == 0 {
		endpoint.BackoffMultiplier = 1
	}
	if endpoint.NextCheckTime.IsZero() {
		endpoint.NextCheckTime = time.Now()
	}

	r.endpoints[key] = endpoint
	return nil
}

// Remove removes an endpoint from the repository
func (r *StaticEndpointRepository) Remove(ctx context.Context, endpointURL *url.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpointURL.String()
	if _, exists := r.endpoints[key]; !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	delete(r.endpoints, key)
	return nil
}
