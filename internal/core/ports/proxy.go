package ports

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
	"net/http"
	"time"
)

// ProxyService defines the interface for the proxy service
type ProxyService interface {
	ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (RequestStats, error)
	GetStats(ctx context.Context) (ProxyStats, error)
}

// ProxyStats contains statistics about the proxy service
type ProxyStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     int64 // in milliseconds
}

type RequestStats struct {
	RequestID           string
	StartTime           time.Time
	EndTime             time.Time
	EndpointName        string
	TargetUrl           string
	TotalBytes          int

	Latency             int64 // Total end-to-end time
	RequestProcessingMs int64 // Time spent in Olla before upstream call
	BackendResponseMs   int64 // Time for backend connection to response headers
	FirstDataMs         int64 // Time from start until first data sent to client
	StreamingMs         int64 // Time spent streaming response data
	HeaderProcessingMs  int64 // Time spent processing headers
	SelectionMs         int64 // Time spent selecting endpoint
}

// DiscoveryService defines the interface for service discovery
type DiscoveryService interface {
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	RefreshEndpoints(ctx context.Context) error
}
