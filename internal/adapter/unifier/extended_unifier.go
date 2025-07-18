package unifier

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// ExtendedUnifier extends the basic ModelUnifier interface with additional methods
type ExtendedUnifier interface {
	ports.ModelUnifier

	// ResolveModel finds a model by name or ID
	ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error)

	// GetAllModels returns all unified models
	GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error)
}
