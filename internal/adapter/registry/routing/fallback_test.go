package routing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func createTestLogger() logger.StyledLogger {
	logCfg := &logger.Config{Level: "error"}
	log, _, _ := logger.New(logCfg)
	return logger.NewPlainStyledLogger(log)
}

func TestOptimisticStrategy_FallbackBehavior(t *testing.T) {
	ctx := context.Background()
	testLogger := createTestLogger()

	healthyEndpoints := []*domain.Endpoint{
		{Name: "ep1", URLString: "http://ep1", Status: domain.StatusHealthy},
		{Name: "ep2", URLString: "http://ep2", Status: domain.StatusHealthy},
	}

	modelEndpoints := []string{"http://ep3"} // Model only on unhealthy endpoint

	t.Run("compatible_only rejects when model not on healthy endpoints", func(t *testing.T) {
		strategy := &OptimisticStrategy{
			fallbackBehavior: "compatible_only",
			logger:          testLogger,
		}

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		assert.NoError(t, err)
		assert.Empty(t, result)
		assert.Equal(t, "rejected", string(decision.Action))
		assert.Equal(t, "model_unavailable_compatible_only", decision.Reason)
	})

	t.Run("none rejects when model not on healthy endpoints", func(t *testing.T) {
		strategy := &OptimisticStrategy{
			fallbackBehavior: "none",
			logger:          testLogger,
		}

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		assert.NoError(t, err)
		assert.Empty(t, result)
		assert.Equal(t, "rejected", string(decision.Action))
		assert.Equal(t, "model_unavailable_no_fallback", decision.Reason)
	})

	t.Run("all returns all healthy when model not on healthy endpoints", func(t *testing.T) {
		strategy := &OptimisticStrategy{
			fallbackBehavior: "all",
			logger:          testLogger,
		}

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		assert.NoError(t, err)
		assert.Equal(t, healthyEndpoints, result)
		assert.Equal(t, "fallback", string(decision.Action))
		assert.Equal(t, "all_healthy_fallback", decision.Reason)
	})

	t.Run("returns model endpoints when available", func(t *testing.T) {
		strategy := &OptimisticStrategy{
			fallbackBehavior: "compatible_only",
			logger:          testLogger,
		}

		// Model on healthy endpoint
		modelEndpointsHealthy := []string{"http://ep1"}

		result, decision, err := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpointsHealthy)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "ep1", result[0].Name)
		assert.Equal(t, "routed", string(decision.Action))
		assert.Equal(t, "model_found", decision.Reason)
	})
}

func TestDiscoveryStrategy_FallbackBehavior(t *testing.T) {
	ctx := context.Background()
	testLogger := createTestLogger()

	healthyEndpoints := []*domain.Endpoint{
		{Name: "ep1", URLString: "http://ep1", Status: domain.StatusHealthy},
		{Name: "ep2", URLString: "http://ep2", Status: domain.StatusHealthy},
	}

	modelEndpoints := []string{"http://ep3"} // Model only on unhealthy endpoint

	t.Run("compatible_only rejects after discovery when model not found", func(t *testing.T) {
		mockDiscovery := &mockDiscoveryForTest{
			healthyEndpoints: healthyEndpoints,
			shouldFail:       false,
		}

		strategy := &DiscoveryStrategy{
			discovery: mockDiscovery,
			options: config.ModelRoutingStrategyOptions{
				FallbackBehavior:       "compatible_only",
				DiscoveryRefreshOnMiss: true,
			},
			logger:         testLogger,
			strictFallback: NewStrictStrategy(testLogger),
		}

		result, decision, _ := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		assert.Empty(t, result)
		assert.Equal(t, "rejected", string(decision.Action))
	})

	t.Run("all returns all healthy after discovery when model not found", func(t *testing.T) {
		mockDiscovery := &mockDiscoveryForTest{
			healthyEndpoints: healthyEndpoints,
			shouldFail:       false,
		}

		strategy := &DiscoveryStrategy{
			discovery: mockDiscovery,
			options: config.ModelRoutingStrategyOptions{
				FallbackBehavior:       "all",
				DiscoveryRefreshOnMiss: true,
			},
			logger:         testLogger,
			strictFallback: NewStrictStrategy(testLogger),
		}

		result, decision, _ := strategy.GetRoutableEndpoints(ctx, "test-model", healthyEndpoints, modelEndpoints)

		assert.Equal(t, healthyEndpoints, result)
		assert.Equal(t, "fallback", string(decision.Action))
	})
}

type mockDiscoveryForTest struct {
	healthyEndpoints []*domain.Endpoint
	shouldFail       bool
	refreshCalled    bool
}

func (m *mockDiscoveryForTest) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.healthyEndpoints, nil
}

func (m *mockDiscoveryForTest) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.healthyEndpoints, nil
}

func (m *mockDiscoveryForTest) RefreshEndpoints(ctx context.Context) error {
	m.refreshCalled = true
	if m.shouldFail {
		return assert.AnError
	}
	return nil
}

func (m *mockDiscoveryForTest) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}