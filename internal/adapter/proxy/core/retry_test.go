package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// testDiscoveryService for testing
type testDiscoveryService struct {
	updatedEndpoint *domain.Endpoint
}

func (t *testDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (t *testDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (t *testDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (t *testDiscoveryService) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	// Capture the endpoint for verification
	t.updatedEndpoint = &domain.Endpoint{}
	*t.updatedEndpoint = *endpoint
	return nil
}

func TestMarkEndpointUnhealthyBackoffProgression(t *testing.T) {
	tests := []struct {
		name                       string
		initialBackoffMultiplier   int
		initialConsecutiveFailures int
		expectedBackoffMultiplier  int
		expectedBackoffInterval    time.Duration
	}{
		{
			name:                       "first_failure",
			initialBackoffMultiplier:   0, // uninitialized
			initialConsecutiveFailures: 0,
			expectedBackoffMultiplier:  2,
			expectedBackoffInterval:    5 * time.Second, // First failure uses normal interval
		},
		{
			name:                       "second_failure",
			initialBackoffMultiplier:   2,
			initialConsecutiveFailures: 1,
			expectedBackoffMultiplier:  4,
			expectedBackoffInterval:    10 * time.Second, // 5s * 2
		},
		{
			name:                       "third_failure",
			initialBackoffMultiplier:   4,
			initialConsecutiveFailures: 2,
			expectedBackoffMultiplier:  8,
			expectedBackoffInterval:    20 * time.Second, // 5s * 4
		},
		{
			name:                       "fourth_failure_capped",
			initialBackoffMultiplier:   8,
			initialConsecutiveFailures: 3,
			expectedBackoffMultiplier:  12,               // capped at 12
			expectedBackoffInterval:    40 * time.Second, // 5s * 8
		},
		{
			name:                       "already_at_max",
			initialBackoffMultiplier:   12,
			initialConsecutiveFailures: 4,
			expectedBackoffMultiplier:  12,               // stays at 12
			expectedBackoffInterval:    60 * time.Second, // stays at 60s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test discovery service
			testDiscovery := &testDiscoveryService{}

			// Create test logger
			logConfig := &logger.Config{Level: "error"}
			log, _, _ := logger.New(logConfig)
			testLogger := logger.NewPlainStyledLogger(log)

			// Create retry handler
			handler := NewRetryHandler(testDiscovery, testLogger)

			// Create test endpoint
			endpoint := &domain.Endpoint{
				Name:                "test-endpoint",
				CheckInterval:       5 * time.Second,
				BackoffMultiplier:   tt.initialBackoffMultiplier,
				ConsecutiveFailures: tt.initialConsecutiveFailures,
			}

			// Mark endpoint as unhealthy
			handler.markEndpointUnhealthy(context.Background(), endpoint)

			// Verify the updated endpoint has correct values
			assert.NotNil(t, testDiscovery.updatedEndpoint)
			assert.Equal(t, tt.expectedBackoffMultiplier, testDiscovery.updatedEndpoint.BackoffMultiplier,
				"BackoffMultiplier should be %d", tt.expectedBackoffMultiplier)
			assert.Equal(t, tt.initialConsecutiveFailures+1, testDiscovery.updatedEndpoint.ConsecutiveFailures,
				"ConsecutiveFailures should increment")
			assert.Equal(t, domain.StatusOffline, testDiscovery.updatedEndpoint.Status,
				"Status should be Offline")

			// Verify NextCheckTime is set correctly
			actualBackoffInterval := testDiscovery.updatedEndpoint.NextCheckTime.Sub(time.Now())

			// Allow 1 second tolerance for test execution time
			assert.InDelta(t, tt.expectedBackoffInterval.Seconds(), actualBackoffInterval.Seconds(), 1,
				"Expected backoff interval ~%v, got %v", tt.expectedBackoffInterval, actualBackoffInterval)
		})
	}
}

func TestMarkEndpointUnhealthyNilEndpoint(t *testing.T) {
	// Create test discovery service
	testDiscovery := &testDiscoveryService{}

	// Create test logger
	logConfig := &logger.Config{Level: "error"}
	log, _, _ := logger.New(logConfig)
	testLogger := logger.NewPlainStyledLogger(log)

	// Create retry handler
	handler := NewRetryHandler(testDiscovery, testLogger)

	// Should not panic or call UpdateEndpointStatus with nil endpoint
	handler.markEndpointUnhealthy(context.Background(), nil)

	// Verify no update was made
	assert.Nil(t, testDiscovery.updatedEndpoint, "Should not update nil endpoint")
}
