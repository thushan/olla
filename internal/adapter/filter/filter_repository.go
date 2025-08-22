package filter

import (
	"context"
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// MemoryFilterRepository implements FilterRepository using in-memory storage
type MemoryFilterRepository struct {
	filters map[string]*domain.FilterConfig
	mu      sync.RWMutex
}

// NewMemoryFilterRepository creates a new in-memory filter repository
func NewMemoryFilterRepository() ports.FilterRepository {
	return &MemoryFilterRepository{
		filters: make(map[string]*domain.FilterConfig),
	}
}

// GetFilterConfig retrieves a filter configuration by key
func (r *MemoryFilterRepository) GetFilterConfig(ctx context.Context, key string) (*domain.FilterConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.filters[key]
	if !exists {
		return nil, fmt.Errorf("filter configuration not found for key: %s", key)
	}

	// Return a clone to prevent external modification
	return config.Clone(), nil
}

// SetFilterConfig stores a filter configuration
func (r *MemoryFilterRepository) SetFilterConfig(ctx context.Context, key string, config *domain.FilterConfig) error {
	if key == "" {
		return fmt.Errorf("filter configuration key cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Store a clone to prevent external modification
	r.filters[key] = config.Clone()
	return nil
}

// GetAllFilterConfigs retrieves all filter configurations
func (r *MemoryFilterRepository) GetAllFilterConfigs(ctx context.Context) (map[string]*domain.FilterConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*domain.FilterConfig, len(r.filters))
	for key, config := range r.filters {
		result[key] = config.Clone()
	}

	return result, nil
}

// DeleteFilterConfig removes a filter configuration
func (r *MemoryFilterRepository) DeleteFilterConfig(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.filters[key]; !exists {
		return fmt.Errorf("filter configuration not found for key: %s", key)
	}

	delete(r.filters, key)
	return nil
}

// ValidateAndStore validates and stores a filter configuration
func (r *MemoryFilterRepository) ValidateAndStore(ctx context.Context, key string, config *domain.FilterConfig) error {
	if config == nil {
		return fmt.Errorf("filter configuration cannot be nil")
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid filter configuration: %w", err)
	}

	// Store the validated configuration
	return r.SetFilterConfig(ctx, key, config)
}

// LoadFromConfig loads filter configurations from a configuration map
// This is useful for loading filters from YAML configuration files
func (r *MemoryFilterRepository) LoadFromConfig(configs map[string]*domain.FilterConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, config := range configs {
		if config == nil {
			continue
		}

		// Validate before storing
		if err := config.Validate(); err != nil {
			return fmt.Errorf("invalid filter configuration for key '%s': %w", key, err)
		}

		r.filters[key] = config.Clone()
	}

	return nil
}

// Clear removes all filter configurations
func (r *MemoryFilterRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filters = make(map[string]*domain.FilterConfig)
}
