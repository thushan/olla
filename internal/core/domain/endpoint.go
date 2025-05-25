package domain

import (
	"context"
	"net/url"
	"time"
)

// Status string constants to reduce allocations
const (
	StatusStringHealthy   = "healthy"
	StatusStringBusy      = "busy"
	StatusStringOffline   = "offline"
	StatusStringWarming   = "warming"
	StatusStringUnhealthy = "unhealthy"
	StatusStringUnknown   = "unknown"
)

// Endpoint represents an Ollama server endpoint
type Endpoint struct {
	Name           string
	URL            *url.URL
	Priority       int
	HealthCheckURL *url.URL
	ModelUrl       *url.URL
	CheckInterval  time.Duration
	CheckTimeout   time.Duration
	Status         EndpointStatus
	LastChecked    time.Time

	// Health check state tracking
	ConsecutiveFailures int
	BackoffMultiplier   int
	NextCheckTime       time.Time
	LastLatency         time.Duration
}

// EndpointStatus represents the health status of an endpoint
type EndpointStatus string

const (
	// StatusHealthy indicates the endpoint is healthy and available
	StatusHealthy EndpointStatus = StatusStringHealthy

	// StatusBusy indicates the endpoint is responding but slowly/overloaded
	StatusBusy EndpointStatus = StatusStringBusy

	// StatusOffline indicates the endpoint is completely unreachable
	StatusOffline EndpointStatus = StatusStringOffline

	// StatusWarming indicates the endpoint is starting up but not ready for full traffic
	StatusWarming EndpointStatus = StatusStringWarming

	// StatusUnhealthy indicates the endpoint has other health issues
	StatusUnhealthy EndpointStatus = StatusStringUnhealthy

	// StatusUnknown indicates the endpoint health is unknown (not yet checked)
	StatusUnknown EndpointStatus = StatusStringUnknown
)

// IsRoutable returns true if the endpoint can receive traffic
func (s EndpointStatus) IsRoutable() bool {
	switch s {
	case StatusHealthy, StatusBusy, StatusWarming:
		return true
	default:
		return false
	}
}

// GetTrafficWeight returns the traffic weight for load balancing (0.0-1.0)
func (s EndpointStatus) GetTrafficWeight() float64 {
	switch s {
	case StatusHealthy:
		return 1.0
	case StatusBusy:
		return 0.3
	case StatusWarming:
		return 0.1
	default:
		return 0.0
	}
}

// String returns the string representation of the status
func (s EndpointStatus) String() string {
	return string(s)
}

// EndpointRepository defines the interface for endpoint storage and retrieval
type EndpointRepository interface {
	// GetAll returns all registered endpoints
	GetAll(ctx context.Context) ([]*Endpoint, error)

	// GetHealthy returns only healthy endpoints
	GetHealthy(ctx context.Context) ([]*Endpoint, error)

	// GetRoutable returns endpoints that can receive traffic
	GetRoutable(ctx context.Context) ([]*Endpoint, error)

	// UpdateStatus updates the health status of an endpoint
	UpdateStatus(ctx context.Context, endpointURL *url.URL, status EndpointStatus) error

	// UpdateEndpoint updates endpoint state including backoff and timing
	UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error

	// Add adds a new endpoint to the repository
	Add(ctx context.Context, endpoint *Endpoint) error

	// Remove removes an endpoint from the repository
	Remove(ctx context.Context, endpointURL *url.URL) error
}

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	Status    EndpointStatus
	Latency   time.Duration
	Error     error
	ErrorType HealthCheckErrorType
}

// HealthCheckErrorType categorises different types of health check errors
type HealthCheckErrorType int

const (
	ErrorTypeNone HealthCheckErrorType = iota
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeHTTPError
	ErrorTypeCircuitOpen
)

// HealthChecker defines the interface for checking endpoint health
type HealthChecker interface {
	// Check performs a health check on the endpoint and returns its status
	Check(ctx context.Context, endpoint *Endpoint) (HealthCheckResult, error)

	// StartChecking begins periodic health checks for all endpoints
	StartChecking(ctx context.Context) error

	// StopChecking stops periodic health checks
	StopChecking(ctx context.Context) error
}

// EndpointSelector defines the interface for selecting endpoints based on a strategy
type EndpointSelector interface {
	// Select chooses an appropriate endpoint from the available healthy endpoints
	Select(ctx context.Context, endpoints []*Endpoint) (*Endpoint, error)

	// Name returns the name of the selection strategy
	Name() string

	// IncrementConnections increments the connection count for an endpoint (for connection tracking)
	IncrementConnections(endpoint *Endpoint)

	// DecrementConnections decrements the connection count for an endpoint (for connection tracking)
	DecrementConnections(endpoint *Endpoint)
}