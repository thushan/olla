package ports

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
)

// DiscoveryService defines the interface for service discovery
type DiscoveryService interface {
	// GetEndpoints returns all registered endpoints
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)

	// GetHealthyEndpoints returns only healthy endpoints
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)

	// RefreshEndpoints triggers a refresh of the endpoint list from the discovery source
	RefreshEndpoints(ctx context.Context) error

	// Start starts the discovery service
	Start(ctx context.Context) error

	// Stop stops the discovery service
	Stop(ctx context.Context) error
}
