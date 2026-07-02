package routing

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// TestStrictStrategy_GetRoutableEndpoints covers the three outcomes strict routing
// can produce (#191): a healthy match routes, a match confined to unhealthy endpoints
// rejects with 503, and no match anywhere rejects with 404. Handlers rely on these
// exact status codes to fail fast instead of falling through to the proxy engine.
func TestStrictStrategy_GetRoutableEndpoints(t *testing.T) {
	ctx := context.Background()
	testLogger := createTestLogger()
	strategy := NewStrictStrategy(testLogger)

	healthyEndpoints := []*domain.Endpoint{
		{Name: "ep1", URLString: "http://ep1", Status: domain.StatusHealthy},
		{Name: "ep2", URLString: "http://ep2", Status: domain.StatusHealthy},
	}

	t.Run("model found on healthy endpoint routes", func(t *testing.T) {
		modelEndpoints := []string{"http://ep1"}

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "ep1", result[0].Name)
		assert.Equal(t, ports.RoutingActionRouted, decision.Action)
		assert.Equal(t, constants.RoutingReasonModelFound, decision.Reason)
		assert.Equal(t, http.StatusOK, decision.StatusCode)
	})

	t.Run("model only on unhealthy endpoint rejects with 503", func(t *testing.T) {
		modelEndpoints := []string{"http://ep3"} // not in healthyEndpoints

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		require.Error(t, err)
		assert.Empty(t, result)
		assert.Equal(t, ports.RoutingActionRejected, decision.Action)
		assert.Equal(t, constants.RoutingReasonModelUnavailable, decision.Reason)
		assert.Equal(t, http.StatusServiceUnavailable, decision.StatusCode)
	})

	t.Run("model nowhere in the fleet rejects with 404", func(t *testing.T) {
		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, nil)

		require.Error(t, err)
		assert.Empty(t, result)
		assert.Equal(t, ports.RoutingActionRejected, decision.Action)
		assert.Equal(t, constants.RoutingReasonModelNotFound, decision.Reason)
		assert.Equal(t, http.StatusNotFound, decision.StatusCode)
	})
}
