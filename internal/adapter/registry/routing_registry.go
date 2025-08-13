package registry

import (
	"context"

	"github.com/thushan/olla/internal/adapter/registry/routing"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// RoutingRegistry wraps a base registry with routing strategy support
type RoutingRegistry struct {
	domain.ModelRegistry
	routingStrategy ports.ModelRoutingStrategy
	logger          logger.StyledLogger
}

// NewRoutingRegistry creates a registry wrapper with routing support
func NewRoutingRegistry(base domain.ModelRegistry, routingConfig *config.ModelRoutingStrategy,
	discovery DiscoveryService, logger logger.StyledLogger) *RoutingRegistry {

	// create routing strategy
	var routingStrategy ports.ModelRoutingStrategy
	if routingConfig != nil {
		factory := routing.NewFactory(logger)
		// adapt discovery interface if provided
		var discoveryAdapter ports.DiscoveryService
		if discovery != nil {
			discoveryAdapter = &discoveryServiceAdapter{discovery: discovery}
		}
		routingStrategy, _ = factory.Create(*routingConfig, discoveryAdapter)
	} else {
		// default to strict strategy
		routingStrategy = routing.NewStrictStrategy(logger)
	}

	return &RoutingRegistry{
		ModelRegistry:   base,
		routingStrategy: routingStrategy,
		logger:          logger,
	}
}

// GetRoutableEndpointsForModel implements model routing strategy
func (r *RoutingRegistry) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// get endpoints that have this model
	modelEndpoints, err := r.GetEndpointsForModel(ctx, modelName)
	if err != nil {
		r.logger.Error("Failed to get endpoints for model", "model", modelName, "error", err)
		modelEndpoints = []string{} // treat error as model not found
	}

	// delegate to routing strategy
	return r.routingStrategy.GetRoutableEndpoints(ctx, modelName, healthyEndpoints, modelEndpoints)
}
