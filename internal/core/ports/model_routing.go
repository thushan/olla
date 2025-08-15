package ports

import (
	"context"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// ModelRoutingStrategy defines how to route requests when models aren't available on all endpoints
type ModelRoutingStrategy interface {
	// GetRoutableEndpoints returns endpoints that should handle the model request
	GetRoutableEndpoints(
		ctx context.Context,
		modelName string,
		healthyEndpoints []*domain.Endpoint,
		modelEndpoints []string,
	) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error)

	// Name returns the strategy name for logging
	Name() string
}

// routing decision actions
const (
	RoutingActionRouted   = "routed"
	RoutingActionFallback = "fallback"
	RoutingActionRejected = "rejected"
)

// NewRoutingDecision creates a routing decision
func NewRoutingDecision(strategy, action, reason string) *domain.ModelRoutingDecision {
	decision := &domain.ModelRoutingDecision{
		Strategy: strategy,
		Action:   action,
		Reason:   reason,
	}

	// set appropriate status codes based on action
	switch action {
	case RoutingActionRejected:
		if reason == constants.RoutingReasonModelNotFound {
			decision.StatusCode = http.StatusNotFound
		} else {
			decision.StatusCode = http.StatusServiceUnavailable
		}
	default:
		decision.StatusCode = http.StatusOK
	}

	return decision
}
