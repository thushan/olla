package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"sync"
	"time"
)

const (
	CacheInvalidationDelay = 50 * time.Millisecond
)

// StaticEndpointRepository implements domain.EndpointRepository for static endpoints
type StaticEndpointRepository struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex

	// Copy-on-write caching
	cacheMu        sync.RWMutex
	cachedAll      []*domain.Endpoint
	cachedHealthy  []*domain.Endpoint
	cachedRoutable []*domain.Endpoint
	cacheValid     bool
	lastModified   time.Time
}

func NewStaticEndpointRepository() *StaticEndpointRepository {
	return &StaticEndpointRepository{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

// invalidateCache marks the cache as invalid - must be called with write lock held
func (r *StaticEndpointRepository) invalidateCache() {
	r.cacheValid = false
	r.lastModified = time.Now()
}

// rebuildCache rebuilds the cached endpoint lists - must be called with appropriate locks
func (r *StaticEndpointRepository) rebuildCache() {
	if r.cacheValid {
		return
	}

	all := make([]*domain.Endpoint, 0, len(r.endpoints))
	healthy := make([]*domain.Endpoint, 0, len(r.endpoints))
	routable := make([]*domain.Endpoint, 0, len(r.endpoints))

	for _, endpoint := range r.endpoints {
		// Create copies to maintain mutation safety
		endpointCopy := *endpoint
		all = append(all, &endpointCopy)

		if endpoint.Status == domain.StatusHealthy {
			healthyCopy := *endpoint
			healthy = append(healthy, &healthyCopy)
		}

		if endpoint.Status.IsRoutable() {
			routableCopy := *endpoint
			routable = append(routable, &routableCopy)
		}
	}

	r.cacheMu.Lock()
	r.cachedAll = all
	r.cachedHealthy = healthy
	r.cachedRoutable = routable
	r.cacheValid = true
	r.cacheMu.Unlock()
}

// GetAll returns all registered endpoints
func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return []*domain.Endpoint{}, nil
	}

	// Always return fresh copies to maintain mutation safety
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

	r.rebuildCache()

	r.cacheMu.RLock()
	result := r.cachedHealthy
	r.cacheMu.RUnlock()

	return result, nil
}

// GetRoutable returns endpoints that can receive traffic
func (r *StaticEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.rebuildCache()

	r.cacheMu.RLock()
	result := r.cachedRoutable
	r.cacheMu.RUnlock()

	return result, nil
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
	r.invalidateCache()
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

	r.invalidateCache()
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
	r.invalidateCache()
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
	r.invalidateCache()
	return nil
}
