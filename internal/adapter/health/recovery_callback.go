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
