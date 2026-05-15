package domain

import (
	"context"
	"time"
)

type HealthCheckResult struct {
	// RateLimitedUntil is populated when the probe received a 429 with a Retry-After
	// header. The scheduler uses this to skip probing until the window has elapsed.
	RateLimitedUntil time.Time
	Error            error
	Status           EndpointStatus
	Latency          time.Duration
	ErrorType        HealthCheckErrorType
	StatusCode       int
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
