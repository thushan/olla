package ports

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
	"net/http"
)

// ProxyService defines the interface for the proxy service
type ProxyService interface {
	ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error)
	GetStats(ctx context.Context) (ProxyStats, error)
}

// ProxyStats contains statistics about the proxy service
type ProxyStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     int64 // in milliseconds
}

// DiscoveryService defines the interface for service discovery
type DiscoveryService interface {
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	RefreshEndpoints(ctx context.Context) error
}
