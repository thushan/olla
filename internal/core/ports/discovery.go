package ports

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
)

type DiscoveryService interface {
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	RefreshEndpoints(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
