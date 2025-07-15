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

	// RegisterCustomRule allows platform-specific unification rules
	RegisterCustomRule(platformType string, rule UnificationRule) error

	// GetStats returns unification performance metrics
	GetStats() domain.UnificationStats

	// MergeUnifiedModels merges models from different endpoints
	MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error)

	// Clear removes all cached unified models
	Clear(ctx context.Context) error
}

// UnificationRule defines how to process models from specific platforms
type UnificationRule interface {
	// CanHandle determines if this rule applies to the given model
	CanHandle(modelInfo *domain.ModelInfo) bool

	// Apply processes the model and returns unified format
	Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error)

	// GetPriority returns rule priority (higher values = higher priority)
	GetPriority() int

	// GetName returns the rule name for debugging
	GetName() string
}

// PlatformDetector identifies which platform a model comes from
type PlatformDetector interface {
	// DetectPlatform determines the platform type from model info
	DetectPlatform(modelInfo *domain.ModelInfo) string
}

// ModelNormalizer handles normalisation of model attributes
type ModelNormalizer interface {
	// NormalizeFamily extracts and normalises the model family
	NormalizeFamily(modelName string, platformFamily string) (family string, variant string)

	// NormalizeSize converts various size formats to canonical format
	NormalizeSize(size string) (normalised string, parameterCount int64)

	// NormalizeQuantization converts quantization formats to canonical format
	NormalizeQuantization(quant string) string

	// GenerateCanonicalID creates the unified model ID
	GenerateCanonicalID(family, variant, size, quant string) string

	// GenerateAliases creates platform-specific aliases
	GenerateAliases(unified *domain.UnifiedModel, platformType string, nativeName string) []domain.AliasEntry

	// NormaliseAlias normalises an alias for consistent lookups
	NormaliseAlias(alias string) string
}
