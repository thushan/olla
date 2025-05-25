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
	endpoints        sync.Map // map[string]*circuitState
	failureThreshold int
	timeout          time.Duration
}

type circuitState struct {
	failures    int64
	lastFailure int64 // Unix nano timestamp for atomic access
	isOpen      int32 // 0 = closed, 1 = open
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

	// Check if circuit should auto-recover
	if atomic.LoadInt32(&state.isOpen) == 1 {
		lastFailure := time.Unix(0, atomic.LoadInt64(&state.lastFailure))
		if time.Since(lastFailure) > cb.timeout {
			atomic.StoreInt32(&state.isOpen, 0)
			atomic.StoreInt64(&state.failures, 0)
			return false
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