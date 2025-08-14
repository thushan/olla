package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// Local constants aliasing the shared backoff defaults
const (
	maxBackoffSeconds    = constants.DefaultMaxBackoffSeconds
	maxBackoffMultiplier = constants.DefaultMaxBackoffMultiplier
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name               string
		endpoint           *domain.Endpoint
		success            bool
		expectedInterval   time.Duration
		expectedMultiplier int
		description        string
	}{
		{
			name: "success_resets_backoff",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: 8,
			},
			success:            true,
			expectedInterval:   5 * time.Second,
			expectedMultiplier: 1,
			description:        "Successful check should reset backoff to normal interval",
		},
		{
			name: "first_failure_keeps_normal_interval",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: 1,
			},
			success:            false,
			expectedInterval:   5 * time.Second, // First failure uses normal interval
			expectedMultiplier: 2,
			description:        "First failure should keep normal interval but set multiplier to 2",
		},
		{
			name: "second_failure_doubles_interval",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: 2,
			},
			success:            false,
			expectedInterval:   10 * time.Second, // Uses current multiplier (2)
			expectedMultiplier: 4,
			description:        "Second failure should double the interval (using multiplier 2)",
		},
		{
			name: "third_failure_exponential_growth",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: 4,
			},
			success:            false,
			expectedInterval:   20 * time.Second, // Uses current multiplier (4)
			expectedMultiplier: 8,
			description:        "Third failure uses multiplier 4 (20s interval)",
		},
		{
			name: "backoff_capped_at_max_multiplier",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: 8,
			},
			success:            false,
			expectedInterval:   40 * time.Second,     // Uses current multiplier (8)
			expectedMultiplier: maxBackoffMultiplier, // Next multiplier capped at max
			description:        "Fourth failure uses multiplier 8, next capped at max",
		},
		{
			name: "backoff_capped_at_max_seconds",
			endpoint: &domain.Endpoint{
				CheckInterval:     10 * time.Second,
				BackoffMultiplier: 6,
			},
			success:            false,
			expectedInterval:   maxBackoffSeconds, // 10s * 6 = 60s
			expectedMultiplier: maxBackoffMultiplier,
			description:        "Interval should be capped at MaxBackoffSeconds",
		},
		{
			name: "already_at_max_multiplier_stays_at_max",
			endpoint: &domain.Endpoint{
				CheckInterval:     5 * time.Second,
				BackoffMultiplier: maxBackoffMultiplier,
			},
			success:            false,
			expectedInterval:   maxBackoffSeconds,
			expectedMultiplier: maxBackoffMultiplier,
			description:        "Once at max multiplier, should stay at max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval, multiplier := calculateBackoff(tt.endpoint, tt.success)

			assert.Equal(t, tt.expectedInterval, interval,
				"Interval mismatch: %s", tt.description)
			assert.Equal(t, tt.expectedMultiplier, multiplier,
				"Multiplier mismatch: %s", tt.description)
		})
	}
}

// TestBackoffProgressionSequence verifies the complete exponential backoff sequence
func TestBackoffProgressionSequence(t *testing.T) {
	endpoint := &domain.Endpoint{
		CheckInterval:       5 * time.Second,
		BackoffMultiplier:   1,
		ConsecutiveFailures: 0,
	}

	// Expected exponential progression
	expectedSequence := []struct {
		interval   time.Duration
		multiplier int
	}{
		{5 * time.Second, 2},                      // First failure - normal interval
		{10 * time.Second, 4},                     // Second failure - 2x interval
		{20 * time.Second, 8},                     // Third failure - 4x interval
		{40 * time.Second, maxBackoffMultiplier},  // Fourth failure - 8x interval (next multiplier capped)
		{maxBackoffSeconds, maxBackoffMultiplier}, // Fifth failure - capped at max
	}

	for i, expected := range expectedSequence {
		interval, multiplier := calculateBackoff(endpoint, false)

		assert.Equal(t, expected.interval, interval,
			"Failure %d: Expected interval %v, got %v", i+1, expected.interval, interval)
		assert.Equal(t, expected.multiplier, multiplier,
			"Failure %d: Expected multiplier %d, got %d", i+1, expected.multiplier, multiplier)

		// Update endpoint for next iteration
		endpoint.BackoffMultiplier = multiplier
		endpoint.ConsecutiveFailures++
	}

	// Verify recovery resets backoff
	interval, multiplier := calculateBackoff(endpoint, true)
	assert.Equal(t, 5*time.Second, interval, "Success should reset to original interval")
	assert.Equal(t, 1, multiplier, "Success should reset multiplier to 1")
}

// TestBackoffDoesNotUseConsecutiveFailures ensures we don't accidentally use ConsecutiveFailures
// instead of BackoffMultiplier for the calculation (this was the bug we fixed)
func TestBackoffDoesNotUseConsecutiveFailures(t *testing.T) {
	// This test specifically guards against the regression where we used
	// ConsecutiveFailures instead of BackoffMultiplier

	endpoint := &domain.Endpoint{
		CheckInterval:       5 * time.Second,
		BackoffMultiplier:   2,  // Exponential multiplier
		ConsecutiveFailures: 10, // This should NOT affect the calculation
	}

	interval, multiplier := calculateBackoff(endpoint, false)

	// With BackoffMultiplier=2, we use current multiplier for interval, next will be 4
	assert.Equal(t, 10*time.Second, interval,
		"Backoff should use current BackoffMultiplier (2), not ConsecutiveFailures (10)")
	assert.Equal(t, 4, multiplier,
		"Multiplier should double from 2 to 4, not be affected by ConsecutiveFailures")

	// If we were incorrectly using ConsecutiveFailures, we'd get:
	// interval = 5s * (10 * 2) = 100s (wrong!)
	assert.NotEqual(t, 100*time.Second, interval,
		"Interval should NOT be based on ConsecutiveFailures")
}
