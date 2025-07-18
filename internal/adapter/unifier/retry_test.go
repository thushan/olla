package unifier

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetry_Success(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	err := Retry(context.Background(), policy, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       2,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	err := Retry(context.Background(), policy, func(ctx context.Context) error {
		attempts++
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retry attempts (2) exceeded")
	assert.Equal(t, 2, attempts)
}

func TestRetry_PermanentError(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	err := Retry(context.Background(), policy, func(ctx context.Context) error {
		attempts++
		return &PermanentError{Err: errors.New("permanent failure")}
	})

	assert.Error(t, err)
	assert.Equal(t, 1, attempts) // Should not retry permanent errors

	var permErr *PermanentError
	assert.True(t, errors.As(err, &permErr))
}

func TestRetry_ContextCancellation(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    50 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, policy, func(ctx context.Context) error {
		attempts++
		return errors.New("temporary error")
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Less(t, attempts, 5) // Should stop before max attempts
}

func TestRetryWithResult_Success(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	result, err := RetryWithResult(context.Background(), policy, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 2 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 2, attempts)
}

func TestRetryWithResult_Failure(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       2,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	result, err := RetryWithResult(context.Background(), policy, func(ctx context.Context) (int, error) {
		return 0, errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Equal(t, 0, result)
	assert.Contains(t, err.Error(), "max retry attempts")
}

func TestCalculateBackoff(t *testing.T) {
	policy := RetryPolicy{
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 2.0,
	}

	// Test exponential growth
	backoff0 := calculateBackoff(0, policy)
	backoff1 := calculateBackoff(1, policy)
	backoff2 := calculateBackoff(2, policy)

	// Allow for jitter
	assert.InDelta(t, 100*time.Millisecond, backoff0, float64(20*time.Millisecond))
	assert.InDelta(t, 200*time.Millisecond, backoff1, float64(40*time.Millisecond))
	assert.InDelta(t, 400*time.Millisecond, backoff2, float64(80*time.Millisecond))

	// Test max backoff cap
	backoff10 := calculateBackoff(10, policy)
	assert.LessOrEqual(t, backoff10, policy.MaxBackoff)
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "permanent error",
			err:       &PermanentError{Err: errors.New("permanent")},
			retryable: false,
		},
		{
			name:      "retryable error",
			err:       &RetryableError{Err: errors.New("temp"), Retryable: true},
			retryable: true,
		},
		{
			name:      "non-retryable error",
			err:       &RetryableError{Err: errors.New("non-retry"), Retryable: false},
			retryable: false,
		},
		{
			name:      "context deadline exceeded",
			err:       context.DeadlineExceeded,
			retryable: false,
		},
		{
			name:      "generic error",
			err:       errors.New("generic"),
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				return // Skip nil error test for IsRetryable
			}
			assert.Equal(t, tt.retryable, IsRetryable(tt.err))
		})
	}
}

func TestRetry_BackoffTiming(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    50 * time.Millisecond,
		MaxBackoff:        200 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	start := time.Now()
	attempts := 0

	err := Retry(context.Background(), policy, func(ctx context.Context) error {
		attempts++
		return errors.New("fail")
	})

	duration := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)

	// Should have waited at least for the backoffs (50ms + 100ms)
	// Allow some tolerance for execution time
	assert.GreaterOrEqual(t, duration, 120*time.Millisecond)
	assert.LessOrEqual(t, duration, 250*time.Millisecond)
}
