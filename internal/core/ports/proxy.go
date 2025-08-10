package ports

import (
	"context"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/core/domain"
)

// ProxyService defines the interface for the proxy service
type ProxyService interface {
	ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *RequestStats, rlog logger.StyledLogger) error
	ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *RequestStats, rlog logger.StyledLogger) error
	GetStats(ctx context.Context) (ProxyStats, error)
	UpdateConfig(configuration ProxyConfiguration)
}

// ProxyStats contains statistics about the proxy service
type ProxyStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     int64 // in milliseconds
	MinLatency         int64
	MaxLatency         int64
}

// ProxyConfiguration allows us to reuse the configuration across multiple proxy implementations
type ProxyConfiguration interface {
	GetProxyPrefix() string
	GetConnectionTimeout() time.Duration
	GetConnectionKeepAlive() time.Duration
	GetResponseTimeout() time.Duration
	GetReadTimeout() time.Duration
	GetStreamBufferSize() int
	GetProxyProfile() string
}

type ProxyFactory interface {
	Create(proxyTpe string, discoveryService DiscoveryService, selector domain.EndpointSelector, config ProxyConfiguration) (ProxyService, error)
	GetAvailableTypes() []string
}

type RequestStats struct {
	RequestID    string
	Model        string
	StartTime    time.Time
	EndTime      time.Time
	EndpointName string
	TargetUrl    string
	TotalBytes   int

	Latency             int64 // Total end-to-end time
	RequestProcessingMs int64 // Time spent in Olla before upstream call
	BackendResponseMs   int64 // Time for backend connection to response headers
	FirstDataMs         int64 // Time from start until first data sent to client
	StreamingMs         int64 // Time spent streaming response data
	HeaderProcessingMs  int64 // Time spent processing headers
	PathResolutionMs    int64 // Time spent resolving the target path to an endpoint
	SelectionMs         int64 // Time spent selecting endpoint
}

// DiscoveryService defines the interface for service discovery
type DiscoveryService interface {
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	RefreshEndpoints(ctx context.Context) error
}
