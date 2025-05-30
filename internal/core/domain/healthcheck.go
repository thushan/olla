package domain

import (
	"context"
	"time"
)

type HealthCheckResult struct {
	Error      error
	Status     EndpointStatus
	Latency    time.Duration
	ErrorType  HealthCheckErrorType
	StatusCode int
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
