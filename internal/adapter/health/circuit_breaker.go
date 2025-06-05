package health

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)

// HTTPClient interface for better testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CircuitBreaker tracks failure rates and prevents cascading failures
type CircuitBreaker struct {
	endpoints        sync.Map
	failureThreshold int
	timeout          time.Duration
}

type circuitState struct {
	failures    int64
	lastFailure int64
	lastAttempt int64
	isOpen      int32
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: DefaultCircuitBreakerThreshold,
		timeout:          DefaultCircuitBreakerTimeout,
	}
}

func (cb *CircuitBreaker) IsOpen(endpointURL string) bool {
	value, exists := cb.endpoints.Load(endpointURL)
	if !exists {
		return false
	}

	state := value.(*circuitState)
	now := time.Now().UnixNano()

	// Check if circuit should auto-recover
	if atomic.LoadInt32(&state.isOpen) == 1 {
		lastFailure := atomic.LoadInt64(&state.lastFailure)
		if time.Unix(0, lastFailure).Add(cb.timeout).Before(time.Now()) {
			// Allow one request through (half-open state)
			if atomic.CompareAndSwapInt64(&state.lastAttempt, 0, now) {
				return false
			}

			// Another request is already in flight, check if it's been too long
			lastAttempt := atomic.LoadInt64(&state.lastAttempt)
			return time.Unix(0, lastAttempt).Add(time.Second).After(time.Now())
		}
		return true
	}

	return false
}

func (cb *CircuitBreaker) RecordSuccess(endpointURL string) {
	value, exists := cb.endpoints.Load(endpointURL)
	if !exists {
		return
	}

	state := value.(*circuitState)
	atomic.StoreInt64(&state.failures, 0)
	atomic.StoreInt32(&state.isOpen, 0)
	atomic.StoreInt64(&state.lastAttempt, 0)
}

func (cb *CircuitBreaker) RecordFailure(endpointURL string) {
	value, exists := cb.endpoints.Load(endpointURL)
	if !exists {
		state := &circuitState{}
		value, _ = cb.endpoints.LoadOrStore(endpointURL, state)
	}

	state := value.(*circuitState)
	failures := atomic.AddInt64(&state.failures, 1)
	atomic.StoreInt64(&state.lastFailure, time.Now().UnixNano())
	atomic.StoreInt64(&state.lastAttempt, 0)

	if failures >= int64(cb.failureThreshold) {
		atomic.StoreInt32(&state.isOpen, 1)
	}
}

func (cb *CircuitBreaker) CleanupEndpoint(endpointURL string) {
	cb.endpoints.Delete(endpointURL)
}

func (cb *CircuitBreaker) GetActiveEndpoints() []string {
	var endpoints []string
	cb.endpoints.Range(func(key, value interface{}) bool {
		endpoints = append(endpoints, key.(string))
		return true
	})
	return endpoints
}
