package unifier

import (
	"sync"
	"sync/atomic"
	"time"
)

type CircuitBreakerState int32

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker prevents cascade failures by blocking requests to failing endpoints.
// It transitions between closed (normal), open (blocking), and half-open (testing) states.
type CircuitBreaker struct {
	config           CircuitBreakerConfig
	state            atomic.Int32
	failures         atomic.Int32
	successes        atomic.Int32
	lastFailureTime  atomic.Int64
	halfOpenRequests atomic.Int32
	mu               sync.RWMutex
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		config: config,
	}
	cb.state.Store(int32(CircuitClosed))
	return cb
}

func (cb *CircuitBreaker) Allow() bool {
	if !cb.config.Enabled {
		return true
	}

	state := CircuitBreakerState(cb.state.Load())

	switch state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		lastFailure := time.Unix(0, cb.lastFailureTime.Load())
		if time.Since(lastFailure) > cb.config.OpenDuration {
			cb.transitionToHalfOpen()
			return cb.allowHalfOpen()
		}
		return false

	case CircuitHalfOpen:
		return cb.allowHalfOpen()

	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	if !cb.config.Enabled {
		return
	}

	state := CircuitBreakerState(cb.state.Load())

	switch state {
	case CircuitClosed:
		cb.failures.Store(0)

	case CircuitHalfOpen:
		successes := cb.successes.Add(1)
		if successes >= int32(cb.config.SuccessThreshold) {
			cb.transitionToClosed()
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	if !cb.config.Enabled {
		return
	}

	cb.lastFailureTime.Store(time.Now().UnixNano())
	failures := cb.failures.Add(1)

	state := CircuitBreakerState(cb.state.Load())

	switch state {
	case CircuitClosed:
		if failures >= int32(cb.config.FailureThreshold) {
			cb.transitionToOpen()
		}

	case CircuitHalfOpen:
		// Single failure in half-open immediately reopens to prevent cascading failures
		cb.transitionToOpen()
	}
}

func (cb *CircuitBreaker) Reset() {
	cb.state.Store(int32(CircuitClosed))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.halfOpenRequests.Store(0)
}

func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return CircuitBreakerState(cb.state.Load())
}

func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	return CircuitBreakerStats{
		State:            cb.GetState().String(),
		Failures:         int(cb.failures.Load()),
		Successes:        int(cb.successes.Load()),
		LastFailureTime:  time.Unix(0, cb.lastFailureTime.Load()),
		HalfOpenRequests: int(cb.halfOpenRequests.Load()),
	}
}

func (cb *CircuitBreaker) transitionToOpen() {
	cb.state.Store(int32(CircuitOpen))
	cb.successes.Store(0)
	cb.halfOpenRequests.Store(0)
}

func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.state.Store(int32(CircuitHalfOpen))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.halfOpenRequests.Store(0)
}

func (cb *CircuitBreaker) transitionToClosed() {
	cb.state.Store(int32(CircuitClosed))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.halfOpenRequests.Store(0)
}

func (cb *CircuitBreaker) allowHalfOpen() bool {
	current := cb.halfOpenRequests.Add(1)
	return current <= int32(cb.config.HalfOpenRequests)
}

func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreakerStats struct {
	State            string    `json:"state"`
	Failures         int       `json:"failures"`
	Successes        int       `json:"successes"`
	LastFailureTime  time.Time `json:"last_failure_time"`
	HalfOpenRequests int       `json:"half_open_requests"`
}