package ports

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
)

// Filter defines the interface for filtering items based on patterns
type Filter interface {
	// Apply filters a slice of items based on the filter configuration
	// The items parameter can be any slice type, and the name extractor
	// function is used to get the string representation for pattern matching
	Apply(ctx context.Context, config *domain.FilterConfig, items interface{}, nameExtractor func(interface{}) string) (*domain.FilterResult, error)

	// ApplyToMap filters a map of items based on the filter configuration
	// Returns a new map containing only the items that pass the filter
	ApplyToMap(ctx context.Context, config *domain.FilterConfig, items map[string]interface{}) (map[string]interface{}, error)

	// Matches checks if a single item matches the filter configuration
	Matches(config *domain.FilterConfig, itemName string) bool
}

// FilterRepository manages filter configurations
type FilterRepository interface {
	// GetFilterConfig retrieves a filter configuration by key
	GetFilterConfig(ctx context.Context, key string) (*domain.FilterConfig, error)

	// SetFilterConfig stores a filter configuration
	SetFilterConfig(ctx context.Context, key string, config *domain.FilterConfig) error

	// GetAllFilterConfigs retrieves all filter configurations
	GetAllFilterConfigs(ctx context.Context) (map[string]*domain.FilterConfig, error)

	// DeleteFilterConfig removes a filter configuration
	DeleteFilterConfig(ctx context.Context, key string) error

	// ValidateAndStore validates and stores a filter configuration
	ValidateAndStore(ctx context.Context, key string, config *domain.FilterConfig) error
}
