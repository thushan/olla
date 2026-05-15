package domain

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	StatusStringHealthy     = "healthy"
	StatusStringBusy        = "busy"
	StatusStringOffline     = "offline"
	StatusStringWarming     = "warming"
	StatusStringUnhealthy   = "unhealthy"
	StatusStringUnknown     = "unknown"
	StatusStringConfigError = "config_error"
	StatusStringRateLimited = "rate_limited"
)

type Endpoint struct {
	LastChecked   time.Time
	NextCheckTime time.Time
	// RateLimitedUntil is set when a health probe receives 429. The scheduler skips
	// probing this endpoint until the time passes. Never serialised.
	RateLimitedUntil time.Time `json:"-"`
	URL              *url.URL
	HealthCheckURL   *url.URL
	ModelUrl         *url.URL
	ModelFilter      *FilterConfig
	// Headers holds verbatim outbound headers copied from endpoint config at load time.
	Headers               map[string]string `json:"-"`
	Name                  string
	Type                  string `json:"type,omitempty"`
	Status                EndpointStatus
	URLString             string
	HealthCheckPathString string
	HealthCheckURLString  string
	ModelURLString        string
	// AuthHeaderName is the resolved header name for outbound auth (e.g. "Authorization", "X-Api-Key").
	// Precomputed at load time so the hot path pays no allocation cost.
	AuthHeaderName string
	// AuthHeaderValue is the fully composed header value (e.g. "Bearer tok", "Basic base64(...)").
	// Never serialised; leaking credentials through logs or status endpoints would be a security issue.
	AuthHeaderValue     string `json:"-"`
	LastLatency         time.Duration
	CheckInterval       time.Duration
	CheckTimeout        time.Duration
	Priority            int
	ConsecutiveFailures int
	BackoffMultiplier   int
	PreservePath        bool
}

func (e *Endpoint) GetURLString() string {
	return e.URLString
}

func (e *Endpoint) GetHealthCheckURLString() string {
	return e.HealthCheckURLString
}

type EndpointStatus string

const (
	StatusHealthy   EndpointStatus = StatusStringHealthy
	StatusBusy      EndpointStatus = StatusStringBusy
	StatusOffline   EndpointStatus = StatusStringOffline
	StatusWarming   EndpointStatus = StatusStringWarming
	StatusUnhealthy EndpointStatus = StatusStringUnhealthy
	StatusUnknown   EndpointStatus = StatusStringUnknown
	// StatusConfigError indicates the endpoint is reachable but the credentials
	// or headers are wrong. The operator must fix config; retrying achieves nothing.
	StatusConfigError EndpointStatus = StatusStringConfigError
	// StatusRateLimited indicates the endpoint returned 429. The scheduler should
	// honour the Retry-After delay before probing again.
	StatusRateLimited EndpointStatus = StatusStringRateLimited
)

func (s EndpointStatus) IsRoutable() bool {
	switch s {
	case StatusHealthy, StatusBusy, StatusWarming:
		return true
	default:
		return false
	}
}

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

func (s EndpointStatus) String() string {
	return string(s)
}

type EndpointNotFoundError struct {
	URL string
}

func (e *EndpointNotFoundError) Error() string {
	return fmt.Sprintf("endpoint not found: %s", e.URL)
}

type EndpointRepository interface {
	GetAll(ctx context.Context) ([]*Endpoint, error)
	GetRoutable(ctx context.Context) ([]*Endpoint, error)
	GetHealthy(ctx context.Context) ([]*Endpoint, error)
	UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error
	Exists(ctx context.Context, endpointURL *url.URL) bool
}

type EndpointSelector interface {
	Select(ctx context.Context, endpoints []*Endpoint) (*Endpoint, error)
	Name() string
	IncrementConnections(endpoint *Endpoint)
	DecrementConnections(endpoint *Endpoint)
}
