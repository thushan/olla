package registry

import (
	"context"
	"fmt"

	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

type RegistryConfig struct {
	Discovery       DiscoveryService             // injected dependency for discovery strategy
	UnificationConf *config.UnificationConfig    `yaml:"unification"`
	RoutingStrategy *config.ModelRoutingStrategy `yaml:"routing_strategy"`
	Type            string                       `yaml:"type"`
	EnableUnifier   bool                         `yaml:"enable_unifier"`
}

// DiscoveryService interface for discovery operations
type DiscoveryService interface {
	GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
	RefreshEndpoints(ctx context.Context) error
	UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error
}

func NewModelRegistry(regConfig RegistryConfig, logger logger.StyledLogger) (domain.ModelRegistry, error) {
	switch regConfig.Type {
	case "memory", "":
		if regConfig.EnableUnifier {
			return NewUnifiedMemoryModelRegistry(logger, regConfig.UnificationConf, regConfig.RoutingStrategy, regConfig.Discovery), nil
		}
		// for non-unified registry, wrap with routing support
		baseRegistry := NewMemoryModelRegistry(logger)
		return NewRoutingRegistry(baseRegistry, regConfig.RoutingStrategy, regConfig.Discovery, logger), nil
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", regConfig.Type)
	}
}
