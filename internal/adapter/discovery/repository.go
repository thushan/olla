package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/config"
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
		return &domain.ErrEndpointNotFound{URL: key}
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
		return &domain.ErrEndpointNotFound{URL: key}
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
		return &domain.ErrEndpointNotFound{URL: key}
	}

	delete(r.endpoints, key)
	r.invalidateCache()
	return nil
}

func (r *StaticEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := endpointURL.String()
	_, exists := r.endpoints[key]
	return exists
}

func (r *StaticEndpointRepository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = make(map[string]*domain.Endpoint)
	r.invalidateCache()
}
func (r *StaticEndpointRepository) UpsertFromConfig(ctx context.Context, configs []config.EndpointConfig) (*domain.EndpointChangeResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Capture current state for change detection
	oldEndpoints := make(map[string]*domain.Endpoint)
	for key, ep := range r.endpoints {
		oldEndpoints[key] = ep
	}
	oldCount := len(r.endpoints)

	endpointCount := len(configs)
	if endpointCount == 0 {
		r.endpoints = make(map[string]*domain.Endpoint)
		r.invalidateCache()
		return &domain.EndpointChangeResult{
			Changed:  oldCount > 0,
			Removed:  r.getEndpointChanges(oldEndpoints, "removed"),
			OldCount: oldCount,
			NewCount: 0,
		}, nil
	}

	newEndpoints := make(map[string]*domain.Endpoint, endpointCount)

	for _, cfg := range configs {
		if err := validateEndpointConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid endpoint config for %q: %w", cfg.Name, err)
		}

		endpointURL, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint URL %q: %w", cfg.URL, err)
		}

		healthCheckPath, err := url.Parse(cfg.HealthCheckURL)
		if err != nil {
			return nil, fmt.Errorf("invalid health check URL %q: %w", cfg.HealthCheckURL, err)
		}

		modelPath, err := url.Parse(cfg.ModelURL)
		if err != nil {
			return nil, fmt.Errorf("invalid model URL %q: %w", cfg.ModelURL, err)
		}

		healthCheckURL := endpointURL.ResolveReference(healthCheckPath)
		modelURL := endpointURL.ResolveReference(modelPath)
		key := endpointURL.String()

		// Check if this endpoint existed before and config is unchanged
		var newEndpoint *domain.Endpoint
		if existing, exists := oldEndpoints[key]; exists &&
			r.endpointConfigUnchanged(existing, cfg, healthCheckURL, modelURL) {
			newEndpoint = &domain.Endpoint{
				Name:                 cfg.Name,
				URL:                  endpointURL,
				Priority:             cfg.Priority,
				HealthCheckURL:       healthCheckURL,
				ModelUrl:             modelURL,
				CheckInterval:        cfg.CheckInterval,
				CheckTimeout:         cfg.CheckTimeout,
				URLString:            endpointURL.String(),
				HealthCheckURLString: healthCheckURL.String(),
				ModelURLString:       modelURL.String(),
				Status:              existing.Status,
				LastChecked:         existing.LastChecked,
				ConsecutiveFailures: existing.ConsecutiveFailures,
				BackoffMultiplier:   existing.BackoffMultiplier,
				NextCheckTime:       existing.NextCheckTime,
				LastLatency:         existing.LastLatency,
			}
		} else {
			newEndpoint = &domain.Endpoint{
				Name:                 cfg.Name,
				URL:                  endpointURL,
				Priority:             cfg.Priority,
				HealthCheckURL:       healthCheckURL,
				ModelUrl:             modelURL,
				CheckInterval:        cfg.CheckInterval,
				CheckTimeout:         cfg.CheckTimeout,
				Status:               domain.StatusUnknown,
				URLString:            endpointURL.String(),
				HealthCheckURLString: healthCheckURL.String(),
				ModelURLString:       modelURL.String(),
				BackoffMultiplier:    1,
				NextCheckTime:        time.Now(),
			}
		}

		newEndpoints[key] = newEndpoint
	}

	// Detect changes before applying them
	changeResult := r.detectChanges(oldEndpoints, newEndpoints)

	// Apply changes atomically
	r.endpoints = newEndpoints
	r.invalidateCache()

	return changeResult, nil
}

func (r *StaticEndpointRepository) detectChanges(oldEndpoints, newEndpoints map[string]*domain.Endpoint) *domain.EndpointChangeResult {
	result := &domain.EndpointChangeResult{
		OldCount: len(oldEndpoints),
		NewCount: len(newEndpoints),
	}

	// Find added endpoints
	for url, newEp := range newEndpoints {
		if _, exists := oldEndpoints[url]; !exists {
			result.Added = append(result.Added, &domain.EndpointChange{
				Name: newEp.Name,
				URL:  url,
			})
		}
	}

	// Find removed endpoints
	for url, oldEp := range oldEndpoints {
		if _, exists := newEndpoints[url]; !exists {
			result.Removed = append(result.Removed, &domain.EndpointChange{
				Name: oldEp.Name,
				URL:  url,
			})
		}
	}

	// Find modified endpoints with specific changes
	for url, newEp := range newEndpoints {
		if oldEp, exists := oldEndpoints[url]; exists {
			changes := r.getSpecificChanges(oldEp, newEp)
			if len(changes) > 0 {
				result.Modified = append(result.Modified, &domain.EndpointChange{
					Name:    newEp.Name,
					URL:     url,
					Changes: changes,
				})
			}
		}
	}

	result.Changed = len(result.Added) > 0 || len(result.Removed) > 0 || len(result.Modified) > 0

	return result
}

func (r *StaticEndpointRepository) getSpecificChanges(old, new *domain.Endpoint) []string {
	var changes []string

	if old.Name != new.Name {
		changes = append(changes, fmt.Sprintf("name: %s -> %s", old.Name, new.Name))
	}

	if old.Priority != new.Priority {
		changes = append(changes, fmt.Sprintf("priority: %d -> %d", old.Priority, new.Priority))
	}

	if old.HealthCheckURLString != new.HealthCheckURLString {
		changes = append(changes, fmt.Sprintf("health_url: %s -> %s", old.HealthCheckURLString, new.HealthCheckURLString))
	}

	if old.ModelURLString != new.ModelURLString {
		changes = append(changes, fmt.Sprintf("model_url: %s -> %s", old.ModelURLString, new.ModelURLString))
	}

	if old.CheckInterval != new.CheckInterval {
		changes = append(changes, fmt.Sprintf("check_interval: %v -> %v", old.CheckInterval, new.CheckInterval))
	}

	if old.CheckTimeout != new.CheckTimeout {
		changes = append(changes, fmt.Sprintf("check_timeout: %v -> %v", old.CheckTimeout, new.CheckTimeout))
	}

	return changes
}

func (r *StaticEndpointRepository) getEndpointChanges(endpoints map[string]*domain.Endpoint, changeType string) []*domain.EndpointChange {
	changes := make([]*domain.EndpointChange, 0, len(endpoints))
	for url, ep := range endpoints {
		changes = append(changes, &domain.EndpointChange{
			Name: ep.Name,
			URL:  url,
		})
	}
	return changes
}

func (r *StaticEndpointRepository) endpointConfigUnchanged(existing *domain.Endpoint, cfg config.EndpointConfig, healthCheckURL, modelURL *url.URL) bool {
	return existing.Name == cfg.Name &&
		existing.Priority == cfg.Priority &&
		existing.HealthCheckURLString == healthCheckURL.String() &&
		existing.ModelURLString == modelURL.String() &&
		existing.CheckInterval == cfg.CheckInterval &&
		existing.CheckTimeout == cfg.CheckTimeout
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
