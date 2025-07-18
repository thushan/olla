package domain

import (
	"fmt"
	"time"
)

// AliasEntry represents an alias with its source attribution
type AliasEntry struct {
	Name   string `json:"name"`
	Source string `json:"source"` // e.g., "ollama", "lmstudio", "generated"
}

// UnifiedModel represents the canonical model format with platform-agnostic naming
type UnifiedModel struct {
	LastSeen         time.Time              `json:"last_seen"`
	MaxContextLength *int64                 `json:"max_context_length,omitempty"`
	Metadata         map[string]interface{} `json:"metadata"`                     // Platform-specific extras
	ID               string                 `json:"id"`                           // Canonical ID: {family}/{variant}:{size}-{quant}
	Family           string                 `json:"family"`                       // Model family (e.g., phi, llama, qwen)
	Variant          string                 `json:"variant"`                      // Version/variant (e.g., 4, 3.3, 3)
	ParameterSize    string                 `json:"parameter_size"`               // Normalised size (e.g., 14.7b, 70.6b)
	Quantization     string                 `json:"quantization"`                 // Normalised quantization (e.g., q4km, q3kl)
	Format           string                 `json:"format"`                       // Model format (gguf, safetensors, etc.)
	PromptTemplateID string                 `json:"prompt_template_id,omitempty"` // Associated prompt template
	Aliases          []AliasEntry           `json:"aliases"`                      // All known aliases with source attribution
	SourceEndpoints  []SourceEndpoint       `json:"source_endpoints"`             // Where this model is available
	Capabilities     []string               `json:"capabilities"`                 // Inferred capabilities (chat, completion, vision)
	ParameterCount   int64                  `json:"parameter_count"`              // Actual parameter count for sorting
	DiskSize         int64                  `json:"disk_size"`                    // Total disk size across all endpoints
}

// SourceEndpoint represents where a unified model is available
type SourceEndpoint struct {
	LastSeen       time.Time          `json:"last_seen"`
	LastStateCheck time.Time          `json:"last_state_check"`
	StateInfo      *EndpointStateInfo `json:"state_info,omitempty"`
	EndpointURL    string             `json:"-"` // Hidden from JSON output for security
	EndpointName   string             `json:"endpoint_name"`
	NativeName     string             `json:"native_name"` // Original name on this platform
	State          string             `json:"state"`       // loaded, not-loaded, etc.
	ModelState     ModelState         `json:"model_state"` // New typed state
	DiskSize       int64              `json:"disk_size"`
}

// UnificationStats tracks performance metrics for model unification
type UnificationStats struct {
	LastUnificationAt  time.Time `json:"last_unification_at"`
	TotalUnified       int64     `json:"total_unified"`
	TotalErrors        int64     `json:"total_errors"`
	CacheHits          int64     `json:"cache_hits"`
	CacheMisses        int64     `json:"cache_misses"`
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
		if a.Name == alias {
			return true
		}
	}
	return false
}

// GetAliasStrings returns a slice of alias names for backward compatibility
func (u *UnifiedModel) GetAliasStrings() []string {
	aliases := make([]string, len(u.Aliases))
	for i, a := range u.Aliases {
		aliases[i] = a.Name
	}
	return aliases
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
		if endpoint.State == string(ModelStateLoaded) || endpoint.ModelState == ModelStateLoaded {
			loaded = append(loaded, endpoint)
		}
	}
	return loaded
}

// GetHealthyEndpoints returns endpoints that are online and have the model available
func (u *UnifiedModel) GetHealthyEndpoints() []SourceEndpoint {
	var healthy []SourceEndpoint
	for _, endpoint := range u.SourceEndpoints {
		if endpoint.IsHealthy() {
			healthy = append(healthy, endpoint)
		}
	}
	return healthy
}

// UpdateEndpointState updates the state of a specific endpoint
func (u *UnifiedModel) UpdateEndpointState(endpointURL string, state ModelState, stateInfo *EndpointStateInfo) {
	for i := range u.SourceEndpoints {
		if u.SourceEndpoints[i].EndpointURL == endpointURL {
			u.SourceEndpoints[i].ModelState = state
			u.SourceEndpoints[i].StateInfo = stateInfo
			u.SourceEndpoints[i].LastStateCheck = time.Now()
			if state == ModelStateOffline {
				u.SourceEndpoints[i].State = "offline"
			}
			return
		}
	}
}

// MarkEndpointOffline marks a specific endpoint as offline
func (u *UnifiedModel) MarkEndpointOffline(endpointURL string, reason string) {
	u.UpdateEndpointState(endpointURL, ModelStateOffline, &EndpointStateInfo{
		State:           EndpointStateOffline,
		LastStateChange: time.Now(),
		LastError:       reason,
	})
}

// IsHealthy checks if the endpoint is healthy
func (s *SourceEndpoint) IsHealthy() bool {
	// Check new typed state first
	if s.ModelState != "" {
		return s.ModelState.IsHealthy()
	}
	// Fall back to string state for compatibility
	return s.State == "loaded" || s.State == "available"
}

// GetEffectiveState returns the effective state considering both endpoint and model states
func (s *SourceEndpoint) GetEffectiveState() ModelState {
	// If endpoint is offline, model is offline too
	if s.StateInfo != nil && !s.StateInfo.State.IsHealthy() {
		return ModelStateOffline
	}

	// Use typed state if available
	if s.ModelState != "" {
		return s.ModelState
	}

	// Map string state to typed state for compatibility
	switch s.State {
	case "loaded":
		return ModelStateLoaded
	case "available", "not-loaded":
		return ModelStateAvailable
	case "offline":
		return ModelStateOffline
	case "error":
		return ModelStateError
	default:
		return ModelStateUnknown
	}
}
