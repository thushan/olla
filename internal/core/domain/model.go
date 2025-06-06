package domain

import (
	"context"
	"fmt"
	"time"
)

type ModelInfo struct {
	LastSeen    time.Time `json:"last_seen"`
	Name        string    `json:"name"`
	Type        string    `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
	Size        int64     `json:"size,omitempty"`
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
	RemoveEndpoint(ctx context.Context, endpointURL string) error
	GetStats(ctx context.Context) (RegistryStats, error)
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
