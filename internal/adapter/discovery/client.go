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
	Error       error
	EndpointURL string
	ProfileUsed string
	Models      []*domain.ModelInfo
	Duration    time.Duration
	StatusCode  int
}

// DiscoveryMetrics tracks discovery operation statistics
type DiscoveryMetrics struct {
	LastDiscoveryTime  time.Time
	ErrorsByEndpoint   map[string]int64
	TotalDiscoveries   int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
}
