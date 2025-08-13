package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// DiscoveryStrategy refreshes model discovery before deciding
type DiscoveryStrategy struct {
	discovery      ports.DiscoveryService
	logger         logger.StyledLogger
	strictFallback *StrictStrategy // use strict strategy after discovery
	options        config.ModelRoutingStrategyOptions
}

// NewDiscoveryStrategy creates a new discovery routing strategy
func NewDiscoveryStrategy(discovery ports.DiscoveryService, options config.ModelRoutingStrategyOptions, logger logger.StyledLogger) *DiscoveryStrategy {
	return &DiscoveryStrategy{
		discovery:      discovery,
		options:        options,
		logger:         logger,
		strictFallback: NewStrictStrategy(logger),
	}
}

// Name returns the strategy name
func (s *DiscoveryStrategy) Name() string {
	return StrategyDiscovery
}

// GetRoutableEndpoints refreshes discovery then routes based on updated model information
func (s *DiscoveryStrategy) GetRoutableEndpoints(
	ctx context.Context,
	modelName string,
	healthyEndpoints []*domain.Endpoint,
	modelEndpoints []string,
) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	// first check if we already have healthy endpoints with the model
	modelEndpointMap := make(map[string]bool)
	for _, url := range modelEndpoints {
		modelEndpointMap[url] = true
	}

	var currentlyRoutable []*domain.Endpoint
	for _, endpoint := range healthyEndpoints {
		if modelEndpointMap[endpoint.URLString] {
			currentlyRoutable = append(currentlyRoutable, endpoint)
		}
	}

	// if we already have routable endpoints, use them
	if len(currentlyRoutable) > 0 {
		s.logger.Debug("Discovery strategy found existing endpoints with model",
			"model", modelName,
			"routable", len(currentlyRoutable))

		return currentlyRoutable, ports.NewRoutingDecision(
			s.Name(),
			ports.RoutingActionRouted,
			"model_found_no_refresh",
		), nil
	}

	// no healthy endpoints have the model - trigger discovery refresh if configured
	if !s.options.DiscoveryRefreshOnMiss {
		s.logger.Debug("Discovery refresh disabled, rejecting request",
			"model", modelName)

		return nil, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				"model_unavailable_no_refresh",
			), domain.NewModelRoutingError(
				modelName,
				s.Name(),
				"rejected",
				len(healthyEndpoints),
				modelEndpoints,
				fmt.Errorf("model %s not available and discovery refresh disabled", modelName),
			)
	}

	s.logger.Info("Triggering discovery refresh for model",
		"model", modelName,
		"timeout", s.options.DiscoveryTimeout)

	// create timeout context for discovery
	discoveryCtx, cancel := context.WithTimeout(ctx, s.options.DiscoveryTimeout)
	defer cancel()

	// trigger discovery refresh
	startTime := time.Now()
	if err := s.discovery.RefreshEndpoints(discoveryCtx); err != nil {
		s.logger.Warn("Discovery refresh failed",
			"model", modelName,
			"error", err,
			"duration", time.Since(startTime))

		// fallback based on configuration
		if s.options.FallbackBehavior == "compatible_only" {
			return healthyEndpoints, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionFallback,
				"discovery_failed_fallback",
			), nil
		}

		return nil, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				"discovery_failed",
			), domain.NewModelRoutingError(
				modelName,
				s.Name(),
				"rejected",
				len(healthyEndpoints),
				modelEndpoints,
				fmt.Errorf("discovery refresh failed: %w", err),
			)
	}

	s.logger.Debug("Discovery refresh completed",
		"model", modelName,
		"duration", time.Since(startTime))

	// get updated endpoints after discovery
	updatedHealthy, err := s.discovery.GetHealthyEndpoints(ctx)
	if err != nil {
		s.logger.Error("Failed to get endpoints after discovery",
			"model", modelName,
			"error", err)

		// use original endpoints as fallback
		return healthyEndpoints, ports.NewRoutingDecision(
			s.Name(),
			ports.RoutingActionFallback,
			"discovery_error_fallback",
		), nil
	}

	// note: we can't get updated model endpoints here without registry access
	// in practice, the registry would need to be updated during discovery
	// for now, fall back to all healthy endpoints after refresh
	s.logger.Info("Discovery refresh completed, using updated endpoints",
		"model", modelName,
		"updated_healthy", len(updatedHealthy),
		"original_healthy", len(healthyEndpoints))

	if len(updatedHealthy) == 0 {
		return nil, ports.NewRoutingDecision(
				s.Name(),
				ports.RoutingActionRejected,
				"no_healthy_after_discovery",
			), domain.NewModelRoutingError(
				modelName,
				s.Name(),
				"rejected",
				0,
				modelEndpoints,
				fmt.Errorf("no healthy endpoints after discovery refresh"),
			)
	}

	// return all updated healthy endpoints as discovery may have found the model
	return updatedHealthy, ports.NewRoutingDecision(
		s.Name(),
		ports.RoutingActionFallback,
		"discovery_complete_fallback",
	), nil
}
