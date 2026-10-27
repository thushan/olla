package ports

import (
	"fmt"

	"github.com/thushan/olla/internal/core/domain"
)

// ModelResponseConverter converts unified models to different API response formats
type ModelResponseConverter interface {
	// ConvertToFormat converts unified models to the specified format with optional filters
	ConvertToFormat(models []*domain.UnifiedModel, filters ModelFilters) (interface{}, error)
	// GetFormatName returns the name of the format this converter handles
	GetFormatName() string
}

// ModelFilters contains filtering options for model queries
type ModelFilters struct {
	Endpoint  string // Filter by specific endpoint name
	Available *bool  // nil = all, true = available only, false = unavailable only
	Family    string // Filter by model family (e.g., "llama", "phi")
	Type      string // Filter by model kind (e.g., "llm", "vlm", "embeddings"); exact match on the reported kind
	// Capability selects on what a model can do rather than what kind it is,
	// so a vlm that can chat still matches "chat". Set by the legacy
	// ?capability=chat parameter.
	Capability string
}

// QueryParameterError represents an error in query parameter parsing
type QueryParameterError struct {
	Parameter string
	Value     string
	Reason    string
}

func (e *QueryParameterError) Error() string {
	return fmt.Sprintf("invalid query parameter %s=%s: %s", e.Parameter, e.Value, e.Reason)
}
