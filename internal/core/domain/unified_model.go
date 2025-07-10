package domain

import (
	"fmt"
	"time"
)

// UnifiedModel represents the canonical model format with platform-agnostic naming
type UnifiedModel struct {
	ID               string                 `json:"id"`                // Canonical ID: {family}/{variant}:{size}-{quant}
	Family           string                 `json:"family"`            // Model family (e.g., phi, llama, qwen)
	Variant          string                 `json:"variant"`           // Version/variant (e.g., 4, 3.3, 3)
	ParameterSize    string                 `json:"parameter_size"`    // Normalised size (e.g., 14.7b, 70.6b)
	ParameterCount   int64                  `json:"parameter_count"`   // Actual parameter count for sorting
	Quantization     string                 `json:"quantization"`      // Normalised quantization (e.g., q4km, q3kl)
	Format           string                 `json:"format"`            // Model format (gguf, safetensors, etc.)
	Aliases          []string               `json:"aliases"`           // All known aliases for this model
	SourceEndpoints  []SourceEndpoint       `json:"source_endpoints"`  // Where this model is available
	Capabilities     []string               `json:"capabilities"`      // Inferred capabilities (chat, completion, vision)
	MaxContextLength *int64                 `json:"max_context_length,omitempty"`
	DiskSize         int64                  `json:"disk_size"`         // Total disk size across all endpoints
	LastSeen         time.Time              `json:"last_seen"`
	Metadata         map[string]interface{} `json:"metadata"`          // Platform-specific extras
}

// SourceEndpoint represents where a unified model is available
type SourceEndpoint struct {
	EndpointURL  string    `json:"endpoint_url"`
	NativeName   string    `json:"native_name"`   // Original name on this platform
	State        string    `json:"state"`         // loaded, not-loaded, etc.
	LastSeen     time.Time `json:"last_seen"`
	DiskSize     int64     `json:"disk_size"`
}

// UnificationStats tracks performance metrics for model unification
type UnificationStats struct {
	TotalUnified      int64     `json:"total_unified"`
	TotalErrors       int64     `json:"total_errors"`
	CacheHits         int64     `json:"cache_hits"`
	CacheMisses       int64     `json:"cache_misses"`
	LastUnificationAt time.Time `json:"last_unification_at"`
	AverageUnifyTimeMs float64   `json:"average_unify_time_ms"`
}

// UnificationError represents errors during model unification
type UnificationError struct {
	Err          error
	SourceModel  string
	PlatformType string
	Operation    string
}

func (e *UnificationError) Error() string {
	return fmt.Sprintf("unification %s failed for %s model %s: %v",
		e.Operation, e.PlatformType, e.SourceModel, e.Err)
}

func (e *UnificationError) Unwrap() error {
	return e.Err
}

func NewUnificationError(operation, platformType, sourceModel string, err error) *UnificationError {
	return &UnificationError{
		Operation:    operation,
		PlatformType: platformType,
		SourceModel:  sourceModel,
		Err:          err,
	}
}

// IsEquivalent checks if two unified models represent the same underlying model
func (u *UnifiedModel) IsEquivalent(other *UnifiedModel) bool {
	if u == nil || other == nil {
		return false
	}
	return u.ID == other.ID
}

// HasAlias checks if the unified model has a specific alias
func (u *UnifiedModel) HasAlias(alias string) bool {
	for _, a := range u.Aliases {
		if a == alias {
			return true
		}
	}
	return false
}

// GetEndpointByURL returns the source endpoint for a given URL
func (u *UnifiedModel) GetEndpointByURL(url string) *SourceEndpoint {
	for i := range u.SourceEndpoints {
		if u.SourceEndpoints[i].EndpointURL == url {
			return &u.SourceEndpoints[i]
		}
	}
	return nil
}

// AddOrUpdateEndpoint adds or updates a source endpoint
func (u *UnifiedModel) AddOrUpdateEndpoint(endpoint SourceEndpoint) {
	for i := range u.SourceEndpoints {
		if u.SourceEndpoints[i].EndpointURL == endpoint.EndpointURL {
			u.SourceEndpoints[i] = endpoint
			return
		}
	}
	u.SourceEndpoints = append(u.SourceEndpoints, endpoint)
}

// RemoveEndpoint removes a source endpoint by URL
func (u *UnifiedModel) RemoveEndpoint(url string) bool {
	for i := range u.SourceEndpoints {
		if u.SourceEndpoints[i].EndpointURL == url {
			u.SourceEndpoints = append(u.SourceEndpoints[:i], u.SourceEndpoints[i+1:]...)
			return true
		}
	}
	return false
}

// GetTotalDiskSize calculates total disk size across all endpoints
func (u *UnifiedModel) GetTotalDiskSize() int64 {
	var total int64
	for _, endpoint := range u.SourceEndpoints {
		total += endpoint.DiskSize
	}
	return total
}

// IsAvailable checks if the model is available on any endpoint
func (u *UnifiedModel) IsAvailable() bool {
	return len(u.SourceEndpoints) > 0
}

// GetLoadedEndpoints returns endpoints where the model is loaded
func (u *UnifiedModel) GetLoadedEndpoints() []SourceEndpoint {
	var loaded []SourceEndpoint
	for _, endpoint := range u.SourceEndpoints {
		if endpoint.State == "loaded" {
			loaded = append(loaded, endpoint)
		}
	}
	return loaded
}