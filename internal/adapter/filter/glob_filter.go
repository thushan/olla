package filter

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/util/pattern"
)

// GlobFilter implements the Filter interface using glob pattern matching
type GlobFilter struct {
	// cache for compiled patterns to improve performance
	patternCache map[string]bool
	cacheMu      sync.RWMutex
}

// NewGlobFilter creates a new GlobFilter instance
func NewGlobFilter() ports.Filter {
	return &GlobFilter{
		patternCache: make(map[string]bool),
	}
}

// Apply filters a slice of items based on the filter configuration
func (f *GlobFilter) Apply(ctx context.Context, config *domain.FilterConfig, items interface{}, nameExtractor func(interface{}) string) (*domain.FilterResult, error) {
	if config == nil || config.IsEmpty() {
		// no filtering needed, return all items as accepted
		return f.createResultFromItems(items, nil), nil
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter configuration: %w", err)
	}

	// tried without reflection but it was slower
	// so we'll consider it for now
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() != reflect.Slice {
		return nil, fmt.Errorf("items must be a slice, got %T", items)
	}

	accepted := make([]interface{}, 0)
	rejected := make([]interface{}, 0)

	for i := 0; i < itemsValue.Len(); i++ {
		item := itemsValue.Index(i).Interface()
		itemName := nameExtractor(item)

		if f.Matches(config, itemName) {
			accepted = append(accepted, item)
		} else {
			rejected = append(rejected, item)
		}
	}

	return &domain.FilterResult{
		Accepted: accepted,
		Rejected: rejected,
		Stats: domain.FilterStats{
			TotalItems:    itemsValue.Len(),
			AcceptedCount: len(accepted),
			RejectedCount: len(rejected),
		},
	}, nil
}

// ApplyToMap filters a map of items based on the filter configuration
func (f *GlobFilter) ApplyToMap(ctx context.Context, config *domain.FilterConfig, items map[string]interface{}) (map[string]interface{}, error) {
	if config == nil || config.IsEmpty() {
		// not filtering needed, return a copy of the original map
		result := make(map[string]interface{}, len(items))
		for k, v := range items {
			result[k] = v
		}
		return result, nil
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter configuration: %w", err)
	}

	result := make(map[string]interface{})

	for key, value := range items {
		if f.Matches(config, key) {
			result[key] = value
		}
	}

	return result, nil
}

// Matches checks if a single item matches the filter configuration
func (f *GlobFilter) Matches(config *domain.FilterConfig, itemName string) bool {
	if config == nil || config.IsEmpty() {
		// No filter means everything matches
		return true
	}

	// lets check if item should be included
	included := false

	// if there's no include patterns or include has "*", include everything initially
	if config.HasIncludeAll() {
		included = true
	} else {
		// Check if item matches any include pattern
		for _, pattern := range config.Include {
			if f.matchesPattern(itemName, pattern) {
				included = true
				break
			}
		}
	}

	// if not included, no need to check exclude patterns
	if !included {
		return false
	}

	// check if item matches any exclude pattern
	// exclude takes precedence over include
	for _, pattern := range config.Exclude {
		if f.matchesPattern(itemName, pattern) {
			return false
		}
	}

	return true
}

// matchesPattern checks if a string matches a glob pattern with caching
func (f *GlobFilter) matchesPattern(s, patternStr string) bool {
	// caching for perf
	cacheKey := fmt.Sprintf("%s::%s", s, patternStr)

	f.cacheMu.RLock()
	if result, exists := f.patternCache[cacheKey]; exists {
		f.cacheMu.RUnlock()
		return result
	}
	f.cacheMu.RUnlock()

	// Use the centralized pattern matching logic
	matches := pattern.MatchesGlob(s, patternStr)

	// cache this pup
	f.cacheMu.Lock()
	f.patternCache[cacheKey] = matches
	f.cacheMu.Unlock()

	return matches
}

// createResultFromItems creates a FilterResult with all items as accepted
func (f *GlobFilter) createResultFromItems(items interface{}, rejected []interface{}) *domain.FilterResult {
	itemsValue := reflect.ValueOf(items)
	accepted := make([]interface{}, 0, itemsValue.Len())

	for i := 0; i < itemsValue.Len(); i++ {
		accepted = append(accepted, itemsValue.Index(i).Interface())
	}

	if rejected == nil {
		rejected = make([]interface{}, 0)
	}

	return &domain.FilterResult{
		Accepted: accepted,
		Rejected: rejected,
		Stats: domain.FilterStats{
			TotalItems:    itemsValue.Len(),
			AcceptedCount: len(accepted),
			RejectedCount: len(rejected),
		},
	}
}

// ClearCache clears the pattern matching cache
func (f *GlobFilter) ClearCache() {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	f.patternCache = make(map[string]bool)
}
