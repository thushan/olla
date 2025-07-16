package unifier

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestLifecycleUnifier_StartStop(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond
	logger := createTestLogger()

	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	ctx := context.Background()

	// Test starting
	err := unifier.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, unifier.isRunning.Load())

	// Test double start
	err = unifier.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stopping
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err = unifier.Stop(stopCtx)
	assert.NoError(t, err)
	assert.False(t, unifier.isRunning.Load())

	// Test double stop
	err = unifier.Stop(stopCtx)
	assert.NoError(t, err) // Should be idempotent
}

func TestLifecycleUnifier_EndpointStateTracking(t *testing.T) {
	config := DefaultConfig()
	config.MaxConsecutiveFailures = 2
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

	// Test recording failures
	unifier.recordEndpointFailure(endpoint.URLString, fmt.Errorf("connection failed"))
	state := unifier.GetEndpointState(endpoint.URLString)
	assert.NotNil(t, state)
	assert.Equal(t, domain.EndpointStateDegraded, state.State)
	assert.Equal(t, 1, state.ConsecutiveFailures)

	// Second failure should mark endpoint as offline
	unifier.recordEndpointFailure(endpoint.URLString, fmt.Errorf("connection failed again"))
	state = unifier.GetEndpointState(endpoint.URLString)
	assert.Equal(t, domain.EndpointStateOffline, state.State)
	assert.Equal(t, 2, state.ConsecutiveFailures)

	// Success should restore online state
	unifier.recordEndpointSuccess(endpoint.URLString)
	state = unifier.GetEndpointState(endpoint.URLString)
	assert.Equal(t, domain.EndpointStateOnline, state.State)
	assert.Equal(t, 0, state.ConsecutiveFailures)
}

func TestLifecycleUnifier_ModelTTLCleanup(t *testing.T) {
	config := DefaultConfig()
	config.ModelTTL = 100 * time.Millisecond
	config.CleanupInterval = 50 * time.Millisecond
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

	// Add a model
	models := []*domain.ModelInfo{
		{
			Name: "test-model",
			Size: 1000,
			Details: &domain.ModelDetails{
				Digest: strPtr("abc123"),
				State:  strPtr("loaded"),
			},
		},
	}

	unified, err := unifier.UnifyModels(ctx, models, endpoint)
	assert.NoError(t, err)
	assert.Len(t, unified, 1)

	// Verify model exists
	model, err := unifier.ResolveModel(ctx, "test-model")
	assert.NoError(t, err)
	assert.NotNil(t, model)

	// Wait for TTL to expire
	time.Sleep(200 * time.Millisecond)

	// Model should be removed
	model, err = unifier.ResolveModel(ctx, "test-model")
	assert.Error(t, err)
	assert.Nil(t, model)
}

func TestLifecycleUnifier_CircuitBreaker(t *testing.T) {
	config := DefaultConfig()
	config.CircuitBreaker.Enabled = true
	config.CircuitBreaker.FailureThreshold = 3
	config.CircuitBreaker.OpenDuration = 100 * time.Millisecond
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

	// Record failures to trip the circuit breaker
	for i := 0; i < 3; i++ {
		unifier.recordEndpointFailure(endpoint.URLString, fmt.Errorf("failure %d", i))
	}

	// Circuit should be open
	cb := unifier.getOrCreateCircuitBreaker(endpoint.URLString)
	assert.Equal(t, CircuitOpen, cb.GetState())

	// UnifyModels should fail when circuit is open
	_, err = unifier.UnifyModels(ctx, []*domain.ModelInfo{}, endpoint)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")

	// Wait for circuit to transition to half-open
	time.Sleep(150 * time.Millisecond)

	// Circuit should allow limited requests in half-open state
	assert.True(t, cb.Allow())
}

func TestLifecycleUnifier_EndpointRemoval(t *testing.T) {
	config := DefaultConfig()
	logger := createTestLogger()

	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	ctx := context.Background()
	err := unifier.Start(ctx)
	require.NoError(t, err)
	defer unifier.Stop(ctx)

	endpoint1 := &domain.Endpoint{
		URLString: "http://endpoint1",
		Name:      "endpoint1",
	}
	endpoint2 := &domain.Endpoint{
		URLString: "http://endpoint2",
		Name:      "endpoint2",
	}

	// Add models from both endpoints
	models := []*domain.ModelInfo{
		{
			Name: "shared-model",
			Size: 1000,
			Details: &domain.ModelDetails{
				Digest: strPtr("shared123"),
				State:  strPtr("loaded"),
			},
		},
		{
			Name: "unique-model",
			Size: 2000,
			Details: &domain.ModelDetails{
				Digest: strPtr("unique123"),
				State:  strPtr("loaded"),
			},
		},
	}

	// Register from endpoint1
	unified1, err := unifier.UnifyModels(ctx, models, endpoint1)
	assert.NoError(t, err)
	assert.Len(t, unified1, 2)

	// Register shared model from endpoint2
	unified2, err := unifier.UnifyModels(ctx, models[:1], endpoint2)
	assert.NoError(t, err)
	assert.Len(t, unified2, 1)

	// Verify shared model has both endpoints
	sharedModel, err := unifier.ResolveModel(ctx, "shared-model")
	assert.NoError(t, err)
	assert.Len(t, sharedModel.SourceEndpoints, 2)

	// Remove endpoint1
	err = unifier.RemoveEndpoint(ctx, endpoint1.URLString)
	assert.NoError(t, err)

	// Shared model should still exist with only endpoint2
	sharedModel, err = unifier.ResolveModel(ctx, "shared-model")
	assert.NoError(t, err)
	assert.Len(t, sharedModel.SourceEndpoints, 1)
	assert.Equal(t, endpoint2.URLString, sharedModel.SourceEndpoints[0].EndpointURL)

	// Unique model should be removed
	uniqueModel, err := unifier.ResolveModel(ctx, "unique-model")
	assert.Error(t, err)
	assert.Nil(t, uniqueModel)
}

func TestLifecycleUnifier_StateTransitions(t *testing.T) {
	config := DefaultConfig()
	config.EnableStateTransitionLogging = true
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

	// Add model
	models := []*domain.ModelInfo{
		{
			Name: "test-model",
			Size: 1000,
			Details: &domain.ModelDetails{
				State: strPtr("loaded"),
			},
		},
	}

	unified, err := unifier.UnifyModels(ctx, models, endpoint)
	assert.NoError(t, err)
	assert.Len(t, unified, 1)

	// Mark endpoint offline
	unifier.markEndpointOffline(endpoint.URLString, "Test offline")

	// Verify model state is updated
	model := unified[0]
	endpointState := model.GetEndpointByURL(endpoint.URLString)
	assert.NotNil(t, endpointState)
	assert.Equal(t, "offline", endpointState.State)
}

func TestLifecycleUnifier_ConcurrentOperations(t *testing.T) {
	config := DefaultConfig()
	logger := createTestLogger()

	unifier := NewLifecycleUnifier(config, logger).(*LifecycleUnifier)

	ctx := context.Background()
	err := unifier.Start(ctx)
	require.NoError(t, err)
	defer unifier.Stop(ctx)

	// Run concurrent operations
	done := make(chan bool, 3)

	// Goroutine 1: Add models
	go func() {
		for i := 0; i < 10; i++ {
			endpoint := &domain.Endpoint{
				URLString: fmt.Sprintf("http://endpoint%d", i),
				Name:      fmt.Sprintf("endpoint%d", i),
			}
			models := []*domain.ModelInfo{
				{
					Name: fmt.Sprintf("model%d", i),
					Size: int64(i * 1000),
				},
			}
			unifier.UnifyModels(ctx, models, endpoint)
		}
		done <- true
	}()

	// Goroutine 2: Query models
	go func() {
		for i := 0; i < 20; i++ {
			unifier.GetAllModels(ctx)
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Record failures and successes
	go func() {
		for i := 0; i < 15; i++ {
			url := fmt.Sprintf("http://endpoint%d", i%5)
			if i%2 == 0 {
				unifier.recordEndpointFailure(url, fmt.Errorf("test error"))
			} else {
				unifier.recordEndpointSuccess(url)
			}
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify no panic and data is consistent
	models, err := unifier.GetAllModels(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, models)
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func createTestLogger() logger.StyledLogger {
	opts := &slog.HandlerOptions{Level: slog.LevelError}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slogLogger := slog.New(handler)
	return logger.NewPlainStyledLogger(slogLogger)
}