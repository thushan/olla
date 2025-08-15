package routing

import (
	"context"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// OptimisticStrategy falls back to healthy endpoints when model not found
type OptimisticStrategy struct {
	logger           logger.StyledLogger
	fallbackBehavior string
}

// NewOptimisticStrategy creates a new optimistic routing strategy
func NewOptimisticStrategy(fallbackBehavior string, logger logger.StyledLogger) *OptimisticStrategy {
	if fallbackBehavior == "" {
		fallbackBehavior = constants.FallbackBehaviorCompatibleOnly
	}
	return &OptimisticStrategy{
		fallbackBehavior: fallbackBehavior,
		logger:           logger,
	}
}

// Name returns the strategy name
func (s *OptimisticStrategy) Name() string {
	return StrategyOptimistic
}

// GetRoutableEndpoints returns healthy endpoints that have the model, or falls back to all healthy endpoints
func (s *OptimisticStrategy) GetRoutableEndpoints(
	ctx context.Context,
	modelName string,
	healthyEndpoints []*domain.Endpoint,
	modelEndpoints []string,
) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// no endpoints have the model - fall back to all healthy
	if len(modelEndpoints) == 0 {
		s.logger.Debug("Model not found, using all healthy endpoints",
			"model", modelName,
			"healthy_endpoints", len(healthyEndpoints),
			"fallback", s.fallbackBehavior)

		switch s.fallbackBehavior {
		case constants.FallbackBehaviorNone:
			return []*domain.Endpoint{}, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelNotFound,
			), nil
		case constants.FallbackBehaviorCompatibleOnly:
			// For compatible_only, reject when model not found
			return []*domain.Endpoint{}, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelNotFound,
			), nil
		default:
			// "all" or any other value - return all healthy with fallback
			return healthyEndpoints, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionFallback,
				constants.RoutingReasonModelNotFoundFallback,
			), nil
		}
	}

	// create map for fast lookup
	modelEndpointMap := make(map[string]bool)
	for _, url := range modelEndpoints {
		modelEndpointMap[url] = true
	}

	// filter healthy endpoints to those that have the model
	var routable []*domain.Endpoint
	for _, endpoint := range healthyEndpoints {
		if modelEndpointMap[endpoint.URLString] {
			routable = append(routable, endpoint)
		}
	}

	// no healthy endpoints have the model - fall back
	if len(routable) == 0 {
		s.logger.Warn("Model only on unhealthy endpoints, applying fallback behavior",
			"model", modelName,
			"model_endpoints", len(modelEndpoints),
			"healthy_endpoints", len(healthyEndpoints),
			"fallback", s.fallbackBehavior)

		switch s.fallbackBehavior {
		case constants.FallbackBehaviorNone:
			return []*domain.Endpoint{}, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelUnavailableNoFallback,
			), nil
		case constants.FallbackBehaviorCompatibleOnly:
			// For compatible_only, we don't fall back at all if no healthy endpoints have the model
			// This prevents routing to endpoints that don't support the requested model
			return []*domain.Endpoint{}, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelUnavailableCompatibleOnly,
			), nil
		default:
			// "all" or any other value - return all healthy
			return healthyEndpoints, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionFallback,
				constants.RoutingReasonAllHealthyFallback,
			), nil
		}
	}

	s.logger.Debug("Optimistic routing found endpoints with model",
		"model", modelName,
		"routable", len(routable),
		"total_healthy", len(healthyEndpoints))

	return routable, ports.NewRoutingDecision(
		s.Name(),
		ports.RoutingActionRouted,
		constants.RoutingReasonModelFound,
	), nil
}
