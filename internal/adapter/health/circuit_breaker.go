package health

import (
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

var (
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)

// HTTPClient interface for better testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CircuitBreaker tracks failure rates and prevents cascading failures
// TODO: (HOT-RELOAD) Add cleanup mechanism for removed endpoints when hot reload is implemented
// The endpoints xsync.Map will accumulate stale entries for removed/changed endpoints without TTL
type CircuitBreaker struct {
	endpoints        *xsync.Map[string, *circuitState]
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
		endpoints:        xsync.NewMap[string, *circuitState](),
		failureThreshold: DefaultCircuitBreakerThreshold,
		timeout:          DefaultCircuitBreakerTimeout,
	}
}

func (cb *CircuitBreaker) IsOpen(endpointURL string) bool {
	state, ok := cb.endpoints.Load(endpointURL)
	if !ok {
		return false
	}

	now := time.Now().UnixNano()

	// Check if circuit should auto-recover
	if atomic.LoadInt32(&state.isOpen) == 1 {
		lastFailure := atomic.LoadInt64(&state.lastFailure)
		if time.Unix(0, lastFailure).Add(cb.timeout).Before(time.Now()) {
			// Allow one request through (half-open state)
			if atomic.CompareAndSwapInt64(&state.lastAttempt, 0, now) {
				return false
			}

			// Another request is already in flight,
			// check if it's been a long time, shouldn't have left you
			// Without a dope beat to step to
			lastAttempt := atomic.LoadInt64(&state.lastAttempt)
			return time.Unix(0, lastAttempt).Add(time.Second).After(time.Now())
		}
		return true
	}

	return false
}

func (cb *CircuitBreaker) RecordSuccess(endpointURL string) {
	state, ok := cb.endpoints.Load(endpointURL)
	if !ok {
		return
	}

	atomic.StoreInt64(&state.failures, 0)
	atomic.StoreInt32(&state.isOpen, 0)
	atomic.StoreInt64(&state.lastAttempt, 0)
}

func (cb *CircuitBreaker) RecordFailure(endpointURL string) {
	state := cb.loadOrCreateState(endpointURL)

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
	cb.endpoints.Range(func(key string, _ *circuitState) bool {
		endpoints = append(endpoints, key)
		return true
	})
	return endpoints
}

func (cb *CircuitBreaker) loadOrCreateState(endpointURL string) *circuitState {
	state, _ := cb.endpoints.LoadOrCompute(endpointURL, func() (newValue *circuitState, cancel bool) {
		return &circuitState{}, false
	})
	return state
}
