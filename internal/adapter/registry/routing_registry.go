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

		// Attempt to create the configured strategy
		strategy, err := factory.Create(*routingConfig, discoveryAdapter)
		if err != nil || strategy == nil {
			// Log the error and fall back to strict strategy
			logger.Error("Failed to create routing strategy, falling back to strict",
				"configured_type", routingConfig.Type,
				"error", err)
			routingStrategy = routing.NewStrictStrategy(logger)
		} else {
			routingStrategy = strategy
		}
	} else {
		// default to strict strategy
		routingStrategy = routing.NewStrictStrategy(logger)
	}

	// Ensure routingStrategy is never nil
	if routingStrategy == nil {
		logger.Warn("Routing strategy was nil, using strict strategy as fallback")
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

// endpointAwareRegistry is implemented by registries that take the endpoint's
// type along with its models, which the Ollama default-tag rule needs.
type endpointAwareRegistry interface {
	RegisterModelsWithEndpoint(ctx context.Context, endpoint *domain.Endpoint, models []*domain.ModelInfo) error
}

// canonicalNamer is implemented by registries that can name the model a request resolves to.
type canonicalNamer interface {
	CanonicalModelName(name string) string
}

// RegisterModelsWithEndpoint forwards to the wrapped registry when it takes endpoint types.
func (r *RoutingRegistry) RegisterModelsWithEndpoint(ctx context.Context, endpoint *domain.Endpoint, models []*domain.ModelInfo) error {
	if aware, ok := r.ModelRegistry.(endpointAwareRegistry); ok {
		return aware.RegisterModelsWithEndpoint(ctx, endpoint, models)
	}
	return r.ModelRegistry.RegisterModels(ctx, endpoint.GetURLString(), models)
}

// CanonicalModelName forwards to the wrapped registry; see MemoryModelRegistry.CanonicalModelName.
func (r *RoutingRegistry) CanonicalModelName(name string) string {
	if namer, ok := r.ModelRegistry.(canonicalNamer); ok {
		return namer.CanonicalModelName(name)
	}
	return name
}
