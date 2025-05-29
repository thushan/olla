package domain

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/thushan/olla/internal/config"
)

const (
	StatusStringHealthy   = "healthy"
	StatusStringBusy      = "busy"
	StatusStringOffline   = "offline"
	StatusStringWarming   = "warming"
	StatusStringUnhealthy = "unhealthy"
	StatusStringUnknown   = "unknown"
)

type Endpoint struct {
	Name                 string
	URL                  *url.URL
	Priority             int
	HealthCheckURL       *url.URL
	ModelUrl             *url.URL
	CheckInterval        time.Duration
	CheckTimeout         time.Duration
	Status               EndpointStatus
	LastChecked          time.Time
	URLString            string
	HealthCheckURLString string
	ModelURLString       string
	ConsecutiveFailures  int
	BackoffMultiplier    int
	NextCheckTime        time.Time
	LastLatency          time.Duration
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

type ErrEndpointNotFound struct {
	URL string
}

func (e *ErrEndpointNotFound) Error() string {
	return fmt.Sprintf("endpoint not found: %s", e.URL)
}

type EndpointRepository interface {
	GetAll(ctx context.Context) ([]*Endpoint, error)
	GetRoutable(ctx context.Context) ([]*Endpoint, error)
	GetHealthy(ctx context.Context) ([]*Endpoint, error)
	UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error
	LoadFromConfig(ctx context.Context, configs []config.EndpointConfig) error
	Exists(ctx context.Context, endpointURL *url.URL) bool
}

type EndpointSelector interface {
	Select(ctx context.Context, endpoints []*Endpoint) (*Endpoint, error)
	Name() string
	IncrementConnections(endpoint *Endpoint)
	DecrementConnections(endpoint *Endpoint)
}