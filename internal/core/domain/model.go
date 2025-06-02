package domain

import (
	"context"
	"fmt"
	"time"
)

type ModelInfo struct {
	Name        string    `json:"name"`
	Size        int64     `json:"size,omitempty"`
	Type        string    `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
	LastSeen    time.Time `json:"last_seen"`
}

type EndpointModels struct {
	EndpointURL string       `json:"endpoint_url"`
	Models      []*ModelInfo `json:"models"`
	LastUpdated time.Time    `json:"last_updated"`
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
	TotalEndpoints    int            `json:"total_endpoints"`
	TotalModels       int            `json:"total_models"`
	ModelsPerEndpoint map[string]int `json:"models_per_endpoint"`
	LastUpdated       time.Time      `json:"last_updated"`
}

type ModelRegistryError struct {
	Operation   string
	EndpointURL string
	ModelName   string
	Err         error
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