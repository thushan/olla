package filter

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// GlobFilter implements the Filter interface using glob pattern matching
type GlobFilter struct {
	// Cache for compiled patterns to improve performance
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
		// No filtering needed, return all items as accepted
		return f.createResultFromItems(items, nil), nil
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter configuration: %w", err)
	}

	// Use reflection to handle any slice type
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
		// No filtering needed, return a copy of the original map
		result := make(map[string]interface{}, len(items))
		for k, v := range items {
			result[k] = v
		}
		return result, nil
	}

	// Validate the configuration
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

	// First check if item should be included
	included := false

	// If no include patterns or include has "*", include everything initially
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

	// If not included, no need to check exclude patterns
	if !included {
		return false
	}

	// Check if item matches any exclude pattern
	// Exclude takes precedence over include
	for _, pattern := range config.Exclude {
		if f.matchesPattern(itemName, pattern) {
			return false
		}
	}

	return true
}

// matchesPattern checks if a string matches a glob pattern
// This replicates the logic from configurable_profile.go for consistency
func (f *GlobFilter) matchesPattern(s, pattern string) bool {
	// Use cache for performance
	cacheKey := fmt.Sprintf("%s::%s", s, pattern)

	f.cacheMu.RLock()
	if result, exists := f.patternCache[cacheKey]; exists {
		f.cacheMu.RUnlock()
		return result
	}
	f.cacheMu.RUnlock()

	// Simple glob matching for * wildcard
	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)

	var matches bool

	switch {
	case pattern == "*":
		matches = true
	case strings.Contains(pattern, "*"):
		// Handle patterns like "*llava*" or "llava*" or "*llava"
		switch {
		case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
			// *text* - contains
			core := strings.Trim(pattern, "*")
			matches = strings.Contains(s, core)
		case strings.HasPrefix(pattern, "*"):
			// *text - ends with
			suffix := strings.TrimPrefix(pattern, "*")
			matches = strings.HasSuffix(s, suffix)
		case strings.HasSuffix(pattern, "*"):
			// text* - starts with
			prefix := strings.TrimSuffix(pattern, "*")
			matches = strings.HasPrefix(s, prefix)
		default:
			// Shouldn't happen with our validation, but be safe
			matches = s == pattern
		}
	default:
		// Exact match
		matches = s == pattern
	}

	// Cache the result
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
