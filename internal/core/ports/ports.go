package ports

import (
	"context"
	"net/http"

	"github.com/thushan/olla/internal/core/domain"
)

// ProxyService defines the interface for the proxy service
type ProxyService interface {
	// ProxyRequest forwards an HTTP request to an appropriate Ollama endpoint
	ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error

	// GetStats returns statistics about the proxy service
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
	// GetEndpoints returns all registered endpoints
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)

	// GetHealthyEndpoints returns only healthy endpoints
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)

	// RefreshEndpoints triggers a refresh of the endpoint list from the discovery source
	RefreshEndpoints(ctx context.Context) error

	// Start starts the discovery service
	Start(ctx context.Context) error

	// Stop stops the discovery service
	Stop(ctx context.Context) error
}

// PluginService defines the interface for plugin management
type PluginService interface {
	// LoadPlugins loads all enabled plugins
	LoadPlugins(ctx context.Context) error

	// GetLoadBalancers returns all available load balancer plugins
	GetLoadBalancers(ctx context.Context) ([]domain.EndpointSelector, error)

	// GetAuthProviders returns all available authentication provider plugins
	GetAuthProviders(ctx context.Context) ([]AuthProvider, error)

	// GetRateLimiters returns all available rate limiter plugins
	GetRateLimiters(ctx context.Context) ([]RateLimiter, error)

	// GetMetricsEmitters returns all available metrics emitter plugins
	GetMetricsEmitters(ctx context.Context) ([]MetricsEmitter, error)
}

// AuthProvider defines the interface for authentication providers
type AuthProvider interface {
	// Authenticate authenticates a request
	Authenticate(ctx context.Context, r *http.Request) (bool, error)

	// Name returns the name of the auth provider
	Name() string
}

// RateLimiter defines the interface for rate limiters
type RateLimiter interface {
	// Allow checks if a request is allowed based on rate limiting rules
	Allow(ctx context.Context, r *http.Request) (bool, error)

	// Name returns the name of the rate limiter
	Name() string
}

// MetricsEmitter defines the interface for metrics emitters
type MetricsEmitter interface {
	// RecordRequest records metrics for a request
	RecordRequest(ctx context.Context, endpoint *domain.Endpoint, latency int64, success bool) error

	// Name returns the name of the metrics emitter
	Name() string
}
