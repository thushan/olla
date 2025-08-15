package routing

import (
	"context"
	"fmt"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// StrictStrategy only routes to endpoints that have the model
type StrictStrategy struct {
	logger logger.StyledLogger
}

// NewStrictStrategy creates a new strict routing strategy
func NewStrictStrategy(logger logger.StyledLogger) *StrictStrategy {
	return &StrictStrategy{
		logger: logger,
	}
}

// Name returns the strategy name
func (s *StrictStrategy) Name() string {
	return StrategyStrict
}

// GetRoutableEndpoints returns only healthy endpoints that have the model
func (s *StrictStrategy) GetRoutableEndpoints(
	ctx context.Context,
	modelName string,
	healthyEndpoints []*domain.Endpoint,
	modelEndpoints []string,
) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// no endpoints have the model
	if len(modelEndpoints) == 0 {
		s.logger.Debug("Model not found in any endpoint",
			"model", modelName,
			"healthy_endpoints", len(healthyEndpoints))

		return nil, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelNotFound,
			), domain.NewModelRoutingError(
				modelName,
				s.Name(),
				"rejected",
				len(healthyEndpoints),
				modelEndpoints,
				fmt.Errorf("model %s not found on any endpoint", modelName),
			)
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

	// no healthy endpoints have the model
	if len(routable) == 0 {
		s.logger.Info("Model only available on unhealthy endpoints",
			"model", modelName,
			"model_endpoints", len(modelEndpoints),
			"healthy_endpoints", len(healthyEndpoints))

		return nil, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				constants.RoutingReasonModelUnavailable,
			), domain.NewModelRoutingError(
				modelName,
				s.Name(),
				"rejected",
				len(healthyEndpoints),
				modelEndpoints,
				fmt.Errorf("model %s only available on unhealthy endpoints", modelName),
			)
	}

	s.logger.Debug("Strict routing found endpoints with model",
		"model", modelName,
		"routable", len(routable),
		"total_healthy", len(healthyEndpoints))

	return routable, ports.NewRoutingDecision(
		s.Name(),
		ports.RoutingActionRouted,
		constants.RoutingReasonModelFound,
	), nil
}
