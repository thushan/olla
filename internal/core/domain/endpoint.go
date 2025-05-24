package domain

import (
	"context"
	"net/url"
	"time"
)

// Endpoint represents an Ollama server endpoint
type Endpoint struct {
	URL            *url.URL
	Priority       int
	HealthCheckURL *url.URL
	CheckInterval  time.Duration
	CheckTimeout   time.Duration
	Status         EndpointStatus
	LastChecked    time.Time
}

// EndpointStatus represents the health status of an endpoint
type EndpointStatus string

const (
	// StatusHealthy indicates the endpoint is healthy and available
	StatusHealthy EndpointStatus = "healthy"

	// StatusUnhealthy indicates the endpoint is unhealthy or unavailable
	StatusUnhealthy EndpointStatus = "unhealthy"

	// StatusUnknown indicates the endpoint health is unknown (not yet checked)
	StatusUnknown EndpointStatus = "unknown"
)

// EndpointRepository defines the interface for endpoint storage and retrieval
type EndpointRepository interface {
	// GetAll returns all registered endpoints
	GetAll(ctx context.Context) ([]*Endpoint, error)

	// GetHealthy returns only healthy endpoints
	GetHealthy(ctx context.Context) ([]*Endpoint, error)

	// UpdateStatus updates the health status of an endpoint
	UpdateStatus(ctx context.Context, endpointURL *url.URL, status EndpointStatus) error

	// Add adds a new endpoint to the repository
	Add(ctx context.Context, endpoint *Endpoint) error

	// Remove removes an endpoint from the repository
	Remove(ctx context.Context, endpointURL *url.URL) error
}

// HealthChecker defines the interface for checking endpoint health
type HealthChecker interface {
	// Check performs a health check on the endpoint and returns its status
	Check(ctx context.Context, endpoint *Endpoint) (EndpointStatus, error)

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
