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
	mu               sync.RWMutex
	endpoints        map[string]*circuitState
	failureThreshold int
	timeout          time.Duration
}

type circuitState struct {
	failures    int64
	lastFailure time.Time
	isOpen      bool
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		endpoints:        make(map[string]*circuitState),
		failureThreshold: DefaultCircuitBreakerThreshold,
		timeout:          DefaultCircuitBreakerTimeout,
	}
}

func (cb *CircuitBreaker) IsOpen(endpointURL string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	state, exists := cb.endpoints[endpointURL]
	if !exists {
		return false
	}

	if state.isOpen && time.Since(state.lastFailure) > cb.timeout {
		state.isOpen = false
		state.failures = 0
	}

	return state.isOpen
}

func (cb *CircuitBreaker) RecordSuccess(endpointURL string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if state, exists := cb.endpoints[endpointURL]; exists {
		state.failures = 0
		state.isOpen = false
	}
}

func (cb *CircuitBreaker) RecordFailure(endpointURL string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state, exists := cb.endpoints[endpointURL]
	if !exists {
		state = &circuitState{}
		cb.endpoints[endpointURL] = state
	}

	atomic.AddInt64(&state.failures, 1)
	state.lastFailure = time.Now()

	if state.failures >= int64(cb.failureThreshold) {
		state.isOpen = true
	}
}
