package health

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
)

// RecoveryCallback is called when an endpoint recovers from unhealthy to healthy state
type RecoveryCallback interface {
	OnEndpointRecovered(ctx context.Context, endpoint *domain.Endpoint) error
}

// RecoveryCallbackFunc is a function adapter for RecoveryCallback
type RecoveryCallbackFunc func(ctx context.Context, endpoint *domain.Endpoint) error

func (f RecoveryCallbackFunc) OnEndpointRecovered(ctx context.Context, endpoint *domain.Endpoint) error {
	return f(ctx, endpoint)
}

// NoOpRecoveryCallback is a no-op implementation of RecoveryCallback
type NoOpRecoveryCallback struct{}

func (n NoOpRecoveryCallback) OnEndpointRecovered(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

// UnhealthyCallback is called when an endpoint transitions from healthy (or unknown) to an
// unhealthy state. Callers can use this to proactively clean up state tied to the dead backend
// (e.g. purging sticky session entries) rather than waiting for TTL expiry.
type UnhealthyCallback interface {
	OnEndpointUnhealthy(ctx context.Context, endpoint *domain.Endpoint)
}

// UnhealthyCallbackFunc is a function adapter for UnhealthyCallback
type UnhealthyCallbackFunc func(ctx context.Context, endpoint *domain.Endpoint)

func (f UnhealthyCallbackFunc) OnEndpointUnhealthy(ctx context.Context, endpoint *domain.Endpoint) {
	f(ctx, endpoint)
}

// NoOpUnhealthyCallback is a no-op implementation of UnhealthyCallback
type NoOpUnhealthyCallback struct{}

func (n NoOpUnhealthyCallback) OnEndpointUnhealthy(ctx context.Context, endpoint *domain.Endpoint) {
}
