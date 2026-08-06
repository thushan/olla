package ports

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
)

// ModelUnifier provides unified naming and aliasing for models across different platforms
type ModelUnifier interface {
	// UnifyModel converts a platform-specific model to unified format
	UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpoint *domain.Endpoint) (*domain.UnifiedModel, error)

	// UnifyModels batch processes multiple models for efficiency
	UnifyModels(ctx context.Context, sourceModels []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error)

	// ResolveAlias finds unified model by any known alias
	ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error)

	// GetAliases returns all known aliases for a unified model ID
	GetAliases(ctx context.Context, unifiedID string) ([]string, error)

	// GetStats returns unification performance metrics
	GetStats() domain.UnificationStats

	// MergeUnifiedModels merges models from different endpoints
	MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error)

	// Clear removes all cached unified models
	Clear(ctx context.Context) error
}
