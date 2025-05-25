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

// StaticEndpointRepository implements domain.EndpointRepository with optimized caching
type StaticEndpointRepository struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex

	// Copy-on-write caching for filtered results
	cacheMu        sync.RWMutex
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

func (r *StaticEndpointRepository) invalidateCache() {
	r.cacheMu.Lock()
	r.cacheValid = false
	r.lastModified = time.Now()
	// Clear cached slices to prevent memory leaks
	r.cachedHealthy = nil
	r.cachedRoutable = nil
	r.cacheMu.Unlock()
}

func (r *StaticEndpointRepository) rebuildCacheIfNeeded() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cacheValid {
		return
	}

	// Pre-allocate with reasonable estimates
	endpointCount := len(r.endpoints)
	r.cachedHealthy = make([]*domain.Endpoint, 0, endpointCount/4)  // Assume 25% healthy
	r.cachedRoutable = make([]*domain.Endpoint, 0, endpointCount/2) // Assume 50% routable

	// Rebuild both caches in one pass
	for _, endpoint := range r.endpoints {
		if endpoint.Status == domain.StatusHealthy {
			endpointCopy := *endpoint
			r.cachedHealthy = append(r.cachedHealthy, &endpointCopy)
		}

		if endpoint.Status.IsRoutable() {
			endpointCopy := *endpoint
			r.cachedRoutable = append(r.cachedRoutable, &endpointCopy)
		}
	}

	r.cacheValid = true
}

// GetAll returns all registered endpoints with fresh copies for mutation safety
func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return []*domain.Endpoint{}, nil
	}

	// Always create fresh copies for mutation safety - no caching for GetAll
	endpoints := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		endpointCopy := *endpoint
		endpoints = append(endpoints, &endpointCopy)
	}
	return endpoints, nil
}

func (r *StaticEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.rebuildCacheIfNeeded()

	r.cacheMu.RLock()
	// Create fresh copies for return (mutation safety)
	result := make([]*domain.Endpoint, len(r.cachedHealthy))
	for i, endpoint := range r.cachedHealthy {
		endpointCopy := *endpoint
		result[i] = &endpointCopy
	}
	r.cacheMu.RUnlock()

	return result, nil
}

func (r *StaticEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.rebuildCacheIfNeeded()

	r.cacheMu.RLock()
	// Create fresh copies for return (mutation safety)
	result := make([]*domain.Endpoint, len(r.cachedRoutable))
	for i, endpoint := range r.cachedRoutable {
		endpointCopy := *endpoint
		result[i] = &endpointCopy
	}
	r.cacheMu.RUnlock()

	return result, nil
}

func (r *StaticEndpointRepository) UpdateStatus(ctx context.Context, endpointURL *url.URL, status domain.EndpointStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpointURL.String()
	endpoint, exists := r.endpoints[key]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", key)
	}

	// Only invalidate if status actually changed
	if endpoint.Status != status {
		endpoint.Status = status
		endpoint.LastChecked = time.Now()
		r.invalidateCache()
	}

	return nil
}

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

	// Always invalidate cache when UpdateEndpoint is called
	// The caller wouldn't call this unless something changed
	r.invalidateCache()

	return nil
}

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
