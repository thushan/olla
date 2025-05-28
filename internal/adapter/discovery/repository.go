package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"sync"
	"time"
)

type StaticEndpointRepository struct {
	endpoints            map[string]*domain.Endpoint
	mu                   sync.RWMutex
	cacheMu              sync.RWMutex
	cachedHealthyCopies  []*domain.Endpoint
	cachedRoutableCopies []*domain.Endpoint
	cacheValid           bool
	lastModified         time.Time
	cacheHits            int64
	cacheMisses          int64
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
	r.cachedHealthyCopies = nil
	r.cachedRoutableCopies = nil
	r.cacheMu.Unlock()
}

// rebuildCacheIfNeeded now caches the actual copies, not just the filtering logic
func (r *StaticEndpointRepository) rebuildCacheIfNeeded() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cacheValid {
		return
	}

	// Pre-allocate with reasonable estimates
	endpointCount := len(r.endpoints)
	healthyEstimate := endpointCount / 4  // Assume 25% healthy
	routableEstimate := endpointCount / 2 // Assume 50% routable

	if healthyEstimate < 4 {
		healthyEstimate = 4
	}
	if routableEstimate < 8 {
		routableEstimate = 8
	}

	r.cachedHealthyCopies = make([]*domain.Endpoint, 0, healthyEstimate)
	r.cachedRoutableCopies = make([]*domain.Endpoint, 0, routableEstimate)

	// Build both caches in one pass, storing actual copies
	for _, endpoint := range r.endpoints {
		if endpoint.Status == domain.StatusHealthy {
			healthyCopy := *endpoint
			r.cachedHealthyCopies = append(r.cachedHealthyCopies, &healthyCopy)
		}

		if endpoint.Status.IsRoutable() {
			routableCopy := *endpoint
			r.cachedRoutableCopies = append(r.cachedRoutableCopies, &routableCopy)
		}
	}

	r.cacheValid = true
	r.cacheMisses++
}

// GetAll returns all registered endpoints with fresh copies for mutation safety
func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return []*domain.Endpoint{}, nil
	}

	// Always create fresh copies for GetAll - no caching since this changes frequently
	endpoints := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		endpointCopy := *endpoint
		endpoints = append(endpoints, &endpointCopy)
	}
	return endpoints, nil
}

func (r *StaticEndpointRepository) getCachedEndpoints(getSlice func() []*domain.Endpoint) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.rebuildCacheIfNeeded()

	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	src := getSlice()
	result := make([]*domain.Endpoint, len(src))
	for i, endpoint := range src {
		endpointCopy := *endpoint // mutation safety
		result[i] = &endpointCopy
	}

	r.cacheHits++
	return result, nil
}

func (r *StaticEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	return r.getCachedEndpoints(func() []*domain.Endpoint {
		return r.cachedHealthyCopies
	})
}

func (r *StaticEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return r.getCachedEndpoints(func() []*domain.Endpoint {
		return r.cachedRoutableCopies
	})
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

	existing.Status = endpoint.Status
	existing.LastChecked = endpoint.LastChecked
	existing.ConsecutiveFailures = endpoint.ConsecutiveFailures
	existing.BackoffMultiplier = endpoint.BackoffMultiplier
	existing.NextCheckTime = endpoint.NextCheckTime
	existing.LastLatency = endpoint.LastLatency

	// Always invalidate cache when UpdateEndpoint is called
	// The caller wouldn't call this unless something changed
	// NOTE: we test this in the repository_test.go!
	r.invalidateCache()

	return nil
}

func (r *StaticEndpointRepository) Add(ctx context.Context, endpoint *domain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()

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

// GetCacheStats returns cache performance statistics
func (r *StaticEndpointRepository) GetCacheStats() map[string]interface{} {
	// TODO: Remove later if we dont have a reporting endpoint
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	totalAccesses := r.cacheHits + r.cacheMisses
	hitRate := float64(0)
	if totalAccesses > 0 {
		hitRate = float64(r.cacheHits) / float64(totalAccesses)
	}

	return map[string]interface{}{
		"cache_hits":        r.cacheHits,
		"cache_misses":      r.cacheMisses,
		"cache_hit_rate":    hitRate,
		"cache_valid":       r.cacheValid,
		"cached_healthy":    len(r.cachedHealthyCopies),
		"cached_routable":   len(r.cachedRoutableCopies),
		"last_invalidation": r.lastModified,
	}
}
