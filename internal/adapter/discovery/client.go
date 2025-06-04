package discovery

import (
	"context"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// ModelDiscoveryClient handles HTTP-based model discovery for different llm frontends
type ModelDiscoveryClient interface {

	// DiscoverModels discovers available models from an endpoint using its platform profile
	DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error)

	// HealthCheck does a health check geared more towards model discovery endpoints
	HealthCheck(ctx context.Context, endpoint *domain.Endpoint) error

	// GetMetrics metrics for discovery operations
	GetMetrics() DiscoveryMetrics
}

// DiscoveryResult contains the result of a model discovery operation
type DiscoveryResult struct {
	EndpointURL string
	Models      []*domain.ModelInfo
	Error       error
	Duration    time.Duration
	StatusCode  int
	ProfileUsed string
}

// DiscoveryMetrics tracks discovery operation statistics
type DiscoveryMetrics struct {
	TotalDiscoveries   int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	LastDiscoveryTime  time.Time
	ErrorsByEndpoint   map[string]int64
}
