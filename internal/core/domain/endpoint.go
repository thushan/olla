package domain

import (
	"context"
	"net/url"
	"time"
)

const (
	StatusStringHealthy   = "healthy"
	StatusStringBusy      = "busy"
	StatusStringOffline   = "offline"
	StatusStringWarming   = "warming"
	StatusStringUnhealthy = "unhealthy"
	StatusStringUnknown   = "unknown"
)

// Endpoint with pre-computed strings to avoid repeated allocations
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

	// Pre-computed strings to avoid url.String() allocations
	URLString            string
	HealthCheckURLString string
	ModelURLString       string

	// Health check state tracking
	ConsecutiveFailures int
	BackoffMultiplier   int
	NextCheckTime       time.Time
	LastLatency         time.Duration
}

// NewEndpoint creates an endpoint with pre-computed URL strings
func NewEndpoint(name, urlStr, healthCheckURL, modelURL string, priority int, checkInterval, checkTimeout time.Duration) (*Endpoint, error) {
	baseURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	healthURL, err := url.Parse(healthCheckURL)
	if err != nil {
		return nil, err
	}

	modURL, err := url.Parse(modelURL)
	if err != nil {
		return nil, err
	}

	// Pre-compute all URL strings once
	return &Endpoint{
		Name:                 name,
		URL:                  baseURL,
		Priority:             priority,
		HealthCheckURL:       healthURL,
		ModelUrl:             modURL,
		CheckInterval:        checkInterval,
		CheckTimeout:         checkTimeout,
		Status:               StatusUnknown,
		URLString:            baseURL.String(),
		HealthCheckURLString: healthURL.String(),
		ModelURLString:       modURL.String(),
		BackoffMultiplier:    1,
		NextCheckTime:        time.Now(),
	}, nil
}

// GetURLString returns pre-computed URL string
func (e *Endpoint) GetURLString() string {
	return e.URLString
}

// GetHealthCheckURLString returns pre-computed health check URL string
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

type EndpointRepository interface {
	GetAll(ctx context.Context) ([]*Endpoint, error)
	GetHealthy(ctx context.Context) ([]*Endpoint, error)
	GetRoutable(ctx context.Context) ([]*Endpoint, error)
	UpdateStatus(ctx context.Context, endpointURL *url.URL, status EndpointStatus) error
	UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error
	Add(ctx context.Context, endpoint *Endpoint) error
	Remove(ctx context.Context, endpointURL *url.URL) error
}

type HealthCheckResult struct {
	Status    EndpointStatus
	Latency   time.Duration
	Error     error
	ErrorType HealthCheckErrorType
}

type HealthCheckErrorType int

const (
	ErrorTypeNone HealthCheckErrorType = iota
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeHTTPError
	ErrorTypeCircuitOpen
)

type HealthChecker interface {
	Check(ctx context.Context, endpoint *Endpoint) (HealthCheckResult, error)
	StartChecking(ctx context.Context) error
	StopChecking(ctx context.Context) error
}

type EndpointSelector interface {
	Select(ctx context.Context, endpoints []*Endpoint) (*Endpoint, error)
	Name() string
	IncrementConnections(endpoint *Endpoint)
	DecrementConnections(endpoint *Endpoint)
}