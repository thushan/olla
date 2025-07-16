package unifier

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenDuration:     100 * time.Millisecond,
		HalfOpenRequests: 1,
	}

	cb := NewCircuitBreaker(config)
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_OpenOnFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenDuration:     100 * time.Millisecond,
		HalfOpenRequests: 1,
	}

	cb := NewCircuitBreaker(config)

	// Record failures
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
		assert.Equal(t, CircuitClosed, cb.GetState())
		assert.True(t, cb.Allow())
	}

	// Third failure should open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenRequests: 2,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
	assert.False(t, cb.Allow())

	// Wait for open duration
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	assert.True(t, cb.Allow())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Should allow limited requests
	assert.True(t, cb.Allow())
	assert.False(t, cb.Allow()) // Exceeds half-open limit
}

func TestCircuitBreaker_CloseOnSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenRequests: 3,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// Wait and transition to half-open
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.Allow())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Record successes
	cb.RecordSuccess()
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Second success should close the circuit
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_ReopenOnHalfOpenFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenRequests: 2,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// Wait and transition to half-open
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.Allow())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Failure in half-open should reopen
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_Disabled(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          false,
		FailureThreshold: 1,
	}

	cb := NewCircuitBreaker(config)

	// Should always allow when disabled
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
		assert.True(t, cb.Allow())
		assert.Equal(t, CircuitClosed, cb.GetState())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// Reset
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.True(t, cb.Allow())
	
	stats := cb.GetStats()
	assert.Equal(t, 0, stats.Failures)
	assert.Equal(t, 0, stats.Successes)
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 100,
		SuccessThreshold: 50,
		OpenDuration:     100 * time.Millisecond,
		HalfOpenRequests: 10,
	}

	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	var allowed atomic.Int32
	var denied atomic.Int32

	// Concurrent workers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if cb.Allow() {
					allowed.Add(1)
					if id%2 == 0 {
						cb.RecordSuccess()
					} else {
						cb.RecordFailure()
					}
				} else {
					denied.Add(1)
				}
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()

	// Should have processed requests without panic
	total := allowed.Load() + denied.Load()
	assert.Equal(t, int32(1000), total)
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
	}

	cb := NewCircuitBreaker(config)

	// Initial stats
	stats := cb.GetStats()
	assert.Equal(t, "closed", stats.State)
	assert.Equal(t, 0, stats.Failures)
	assert.Equal(t, 0, stats.Successes)

	// Record some activity
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure()

	stats = cb.GetStats()
	assert.Equal(t, "closed", stats.State)
	assert.Equal(t, 2, stats.Failures)
	assert.Equal(t, 0, stats.Successes) // Successes reset on failure in closed state
}