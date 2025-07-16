package unifier

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// mockDiscoveryClient implements DiscoveryClient for testing
type mockDiscoveryClient struct {
	discoverFunc func(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error)
	callCount    int
}

func (m *mockDiscoveryClient) DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
	m.callCount++
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, endpoint)
	}
	return []*domain.ModelInfo{
		{
			Name: "test-model",
			Size: 1000,
			Details: &domain.ModelDetails{
				State: strPtr("loaded"),
			},
		},
	}, nil
}

func TestLifecycleUnifier_ForceEndpointCheck(t *testing.T) {
	config := DefaultConfig()
	logger := createTestLogger()
	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	ctx := context.Background()
	err := unifier.Start(ctx)
	require.NoError(t, err)
	defer unifier.Stop(ctx)

	// Test without discovery client
	err = unifier.ForceEndpointCheck(ctx, "http://test-endpoint")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discovery client not configured")

	// Set up discovery client
	mockClient := &mockDiscoveryClient{
		discoverFunc: func(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
			return []*domain.ModelInfo{
				{
					Name: "test-model",
					Size: 1000,
					Details: &domain.ModelDetails{
						State: strPtr("loaded"),
					},
				},
			}, nil
		},
	}
	unifier.SetDiscoveryClient(mockClient)

	// Add an endpoint first
	endpoint := &domain.Endpoint{
		URLString: "http://test-endpoint",
		Name:      "test-endpoint",
	}
	models := []*domain.ModelInfo{
		{
			Name: "test-model",
			Size: 1000,
		},
	}
	_, err = unifier.UnifyModels(ctx, models, endpoint)
	assert.NoError(t, err)

	// Force check should succeed
	err = unifier.ForceEndpointCheck(ctx, "http://test-endpoint")
	assert.NoError(t, err)
	assert.Equal(t, 1, mockClient.callCount)

	// Test with discovery failure
	mockClient.discoverFunc = func(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
		return nil, fmt.Errorf("discovery failed")
	}

	err = unifier.ForceEndpointCheck(ctx, "http://test-endpoint")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint check failed")

	// Verify failure was recorded
	state := unifier.GetEndpointState("http://test-endpoint")
	assert.NotNil(t, state)
	assert.Equal(t, 1, state.ConsecutiveFailures)

	// Test with non-existent endpoint
	err = unifier.ForceEndpointCheck(ctx, "http://non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint not found")
}

func TestLifecycleUnifier_GetCircuitBreakerStats(t *testing.T) {
	config := DefaultConfig()
	config.CircuitBreaker.Enabled = true
	config.CircuitBreaker.FailureThreshold = 2
	config.MaxConsecutiveFailures = 3

	logger := createTestLogger()
	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	ctx := context.Background()
	err := unifier.Start(ctx)
	require.NoError(t, err)
	defer unifier.Stop(ctx)

	endpoint := &domain.Endpoint{
		URLString: "http://test-endpoint",
		Name:      "test-endpoint",
	}

	// Initially no circuit breakers
	stats := unifier.GetCircuitBreakerStats()
	assert.Empty(t, stats)

	// Create circuit breaker by recording failure
	unifier.recordEndpointFailure(endpoint.URLString, fmt.Errorf("test error"))

	// Should have stats now
	stats = unifier.GetCircuitBreakerStats()
	assert.Len(t, stats, 1)
	assert.Contains(t, stats, endpoint.URLString)
	assert.Equal(t, "closed", stats[endpoint.URLString].State)
	assert.Equal(t, 1, stats[endpoint.URLString].Failures)

	// Get stats for specific endpoint
	endpointStats, exists := unifier.GetCircuitBreakerStatsForEndpoint(endpoint.URLString)
	assert.True(t, exists)
	assert.Equal(t, "closed", endpointStats.State)
	assert.Equal(t, 1, endpointStats.Failures)

	// Trip the circuit breaker
	unifier.recordEndpointFailure(endpoint.URLString, fmt.Errorf("test error 2"))

	stats = unifier.GetCircuitBreakerStats()
	assert.Equal(t, "open", stats[endpoint.URLString].State)
	assert.Equal(t, 2, stats[endpoint.URLString].Failures)

	// Non-existent endpoint
	_, exists = unifier.GetCircuitBreakerStatsForEndpoint("http://non-existent")
	assert.False(t, exists)
}

func TestLifecycleUnifier_DiscoveryIntegration(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond
	logger := createTestLogger()

	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	// Mock discovery client that simulates endpoint going offline
	callCount := 0
	mockClient := &mockDiscoveryClient{
		discoverFunc: func(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
			callCount++
			if callCount > 2 {
				return nil, fmt.Errorf("endpoint offline")
			}
			return []*domain.ModelInfo{
				{
					Name: "model-1",
					Size: 1000,
					Details: &domain.ModelDetails{
						State: strPtr("loaded"),
					},
				},
			}, nil
		},
	}
	unifier.SetDiscoveryClient(mockClient)

	ctx := context.Background()
	err := unifier.Start(ctx)
	require.NoError(t, err)
	defer unifier.Stop(ctx)

	endpoint := &domain.Endpoint{
		URLString: "http://test-endpoint",
		Name:      "test-endpoint",
	}

	// Initial discovery
	models, err := mockClient.DiscoverModels(ctx, endpoint)
	assert.NoError(t, err)

	unified, err := unifier.UnifyModels(ctx, models, endpoint)
	assert.NoError(t, err)
	assert.Len(t, unified, 1)

	// Force check - should succeed
	err = unifier.ForceEndpointCheck(ctx, endpoint.URLString)
	assert.NoError(t, err)

	// Force check again - should fail (callCount > 2)
	err = unifier.ForceEndpointCheck(ctx, endpoint.URLString)
	assert.Error(t, err)

	// Verify endpoint state changed
	state := unifier.GetEndpointState(endpoint.URLString)
	assert.NotNil(t, state)
	assert.Greater(t, state.ConsecutiveFailures, 0)

	// Verify circuit breaker stats
	cbStats := unifier.GetCircuitBreakerStats()
	assert.NotEmpty(t, cbStats)
}