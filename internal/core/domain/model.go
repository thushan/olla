package domain

import (
	"context"
	"fmt"
	"time"
)

// ModelDetails holds detailed model metadata for intelligent routing
// based off ollama model details, guessing others won't be this detailed
type ModelDetails struct {
	ParameterSize     *string    `json:"parameter_size,omitempty"`
	QuantizationLevel *string    `json:"quantization_level,omitempty"`
	Publisher         *string    `json:"publisher,omitempty"`
	Type              *string    `json:"type,omitempty"` // from LMStudio: llm, vlm
	Family            *string    `json:"family,omitempty"`
	Format            *string    `json:"format,omitempty"`
	ParentModel       *string    `json:"parent_model,omitempty"`
	State             *string    `json:"state,omitempty"`              // loaded / not-loaded (LMStudio gives this now)
	Digest            *string    `json:"digest,omitempty"`             // super important for comparison checks
	MaxContextLength  *int64     `json:"max_context_length,omitempty"` // Max context length in tokens (LMStudio gives this)
	ModifiedAt        *time.Time `json:"modified_at,omitempty"`
	Checkpoint        *string    `json:"checkpoint,omitempty"` // Lemonade: HuggingFace model path
	Recipe            *string    `json:"recipe,omitempty"`     // Lemonade: inference engine (oga-cpu, oga-npu, llamacpp, flm)
	Families          []string   `json:"families,omitempty"`
}

type ModelInfo struct {
	LastSeen    time.Time     `json:"last_seen"`
	Details     *ModelDetails `json:"details,omitempty"`
	Name        string        `json:"name"`
	Type        string        `json:"type,omitempty"`
	Description string        `json:"description,omitempty"`
	Size        int64         `json:"size,omitempty"` // Disk size in bytes
}

type EndpointModels struct {
	LastUpdated time.Time    `json:"last_updated"`
	EndpointURL string       `json:"endpoint_url"`
	Models      []*ModelInfo `json:"models"`
}

type ModelRegistry interface {
	RegisterModel(ctx context.Context, endpointURL string, model *ModelInfo) error
	RegisterModels(ctx context.Context, endpointURL string, models []*ModelInfo) error
	GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*ModelInfo, error)
	GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error)
	IsModelAvailable(ctx context.Context, modelName string) bool
	GetAllModels(ctx context.Context) (map[string][]*ModelInfo, error)
	GetEndpointModelMap(ctx context.Context) (map[string]*EndpointModels, error)
	RemoveEndpoint(ctx context.Context, endpointURL string) error
	GetStats(ctx context.Context) (RegistryStats, error)
	ModelsToString(models []*ModelInfo) string
	ModelsToStrings(models []*ModelInfo) []string
	GetModelsByCapability(ctx context.Context, capability string) ([]*UnifiedModel, error)

	// GetRoutableEndpointsForModel returns endpoints that should handle a model request based on routing strategy
	GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*Endpoint) ([]*Endpoint, *ModelRoutingDecision, error)
}

// ModelRoutingDecision captures routing decision information
type ModelRoutingDecision struct {
	Strategy   string // strategy name
	Action     string // routed, fallback, rejected
	Reason     string // human-readable reason
	StatusCode int    // suggested HTTP status for failures
}

type RegistryStats struct {
	LastUpdated       time.Time      `json:"last_updated"`
	ModelsPerEndpoint map[string]int `json:"models_per_endpoint"`
	TotalEndpoints    int            `json:"total_endpoints"`
	TotalModels       int            `json:"total_models"`
}

type ModelRegistryError struct {
	Err         error
	Operation   string
	EndpointURL string
	ModelName   string
}

func (e *ModelRegistryError) Error() string {
	return fmt.Sprintf("model registry %s failed for endpoint %s, model %s: %v",
		e.Operation, e.EndpointURL, e.ModelName, e.Err)
}

func (e *ModelRegistryError) Unwrap() error {
	return e.Err
}

func NewModelRegistryError(operation, endpointURL, modelName string, err error) *ModelRegistryError {
	return &ModelRegistryError{
		Operation:   operation,
		EndpointURL: endpointURL,
		ModelName:   modelName,
		Err:         err,
	}
}
