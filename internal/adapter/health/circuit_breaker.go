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

// probeStaleness derives the half-open single-flight staleness window from
// the endpoint's own CheckTimeout, rather than a single fixed constant: this
// CircuitBreaker is shared across every endpoint (one xsync.Map keyed by
// health-check URL), but CheckTimeout is per-endpoint config, so the window
// has to be computed per call from whatever the caller passes in, not baked
// into the CircuitBreaker at construction.
//
// checker.go's Check wraps each attempt - including HealthClient's own
// internal retries - in a context timeout of CheckTimeout*2, so a
// legitimately slow-but-alive probe can genuinely take that long to
// resolve. +1s margin absorbs scheduling jitter right around that boundary.
// A fixed 1s window (this package's original value) was tighter than that
// real probe budget and could trigger a spurious stale-handover - admitting
// a second concurrent probe - against a backend that was merely slow, not
// hung.
//
// checkTimeout <= 0 (a caller with no real endpoint, e.g. a test) falls back
// to defaultCheckTimeoutFallback, matching config.go's own endpoint default,
// so the window is never zero.
func probeStaleness(checkTimeout time.Duration) time.Duration {
	if checkTimeout <= 0 {
		checkTimeout = defaultCheckTimeoutFallback
	}
	return checkTimeout*2 + time.Second
}

// IsOpen reports whether the circuit is open for endpointURL, given that
// endpoint's own CheckTimeout (used to derive the half-open staleness
// window - see probeStaleness). checkTimeout may be zero if the caller has
// no endpoint context (e.g. a test), in which case a package default
// applies.
//
// The second return value, attempt, identifies the specific half-open probe
// this call admitted (0 when the circuit is closed and no half-open gating
// applies). Callers that proceed to perform the probe MUST pass attempt
// back into the matching RecordSuccess/RecordFailure call - see those for
// why: a late result from a probe that was superseded by a stale-handover
// replacement must not clobber the replacement's state.
func (cb *CircuitBreaker) IsOpen(endpointURL string, checkTimeout time.Duration) (open bool, attempt int64) {
	state, ok := cb.endpoints.Load(endpointURL)
	if !ok {
		return false, 0
	}

	now := time.Now().UnixNano()

	// Check if circuit should auto-recover
	if atomic.LoadInt32(&state.isOpen) == 1 {
		lastFailure := atomic.LoadInt64(&state.lastFailure)
		if time.Unix(0, lastFailure).Add(cb.timeout).Before(time.Now()) {
			// Half-open: admit exactly one probe. lastAttempt is the
			// single-flight gate - only the goroutine that wins the 0->now CAS
			// is let through immediately; every other concurrent caller is
			// rejected until RecordSuccess/RecordFailure resolves the probe
			// and resets it. If the outstanding probe is stale (older than
			// probeStaleness(checkTimeout) - e.g. a hung health check that
			// never resolved), the slot is handed to exactly one replacement
			// caller via a last->now CAS: a plain read-and-compare here (the
			// bug this replaces) would admit every caller for as long as the
			// window keeps being exceeded, since nothing re-stamps
			// lastAttempt on the read path - the circuit effectively stays
			// wide open forever after a single stuck probe.
			if atomic.CompareAndSwapInt64(&state.lastAttempt, 0, now) {
				return false, now
			}
			lastAttempt := atomic.LoadInt64(&state.lastAttempt)
			if time.Unix(0, lastAttempt).Add(probeStaleness(checkTimeout)).After(time.Now()) {
				return true, 0
			}
			if atomic.CompareAndSwapInt64(&state.lastAttempt, lastAttempt, now) {
				return false, now
			}
			return true, 0
		}
		return true, 0
	}

	return false, 0
}

// RecordSuccess records a successful probe for endpointURL. attempt must be
// the value IsOpen returned when it admitted this probe (0 for the normal
// closed-circuit path, where no correlation is needed or performed).
//
// ABA race note: when attempt is non-zero (a half-open probe), this only
// applies if attempt still matches state.lastAttempt - i.e. this call is the
// most recent admitted probe, not one that a stale-handover already
// superseded. Without this check, a very late result from a hung probe that
// finally resolves AFTER a replacement probe has already run to completion
// would clobber the replacement's outcome (e.g. a stale failure re-opening
// a circuit the replacement had just legitimately closed, or vice versa).
// This closes the gap for the common case - a superseded probe's result
// arriving after the replacement has already resolved - but is not a full
// fencing solution: two admitted probes racing to record at literally the
// same instant are not additionally serialised beyond the atomics already
// in play. That residual window did not previously exist as a correctness
// requirement (the code before this had no correlation at all) and is left
// as understood behaviour rather than a bug.
func (cb *CircuitBreaker) RecordSuccess(endpointURL string, attempt int64) {
	state, ok := cb.endpoints.Load(endpointURL)
	if !ok {
		return
	}
	if attempt != 0 && atomic.LoadInt64(&state.lastAttempt) != attempt {
		return
	}

	atomic.StoreInt64(&state.failures, 0)
	atomic.StoreInt32(&state.isOpen, 0)
	atomic.StoreInt64(&state.lastAttempt, 0)
}

// RecordFailure records a failed probe for endpointURL. See RecordSuccess
// for the attempt correlation contract.
func (cb *CircuitBreaker) RecordFailure(endpointURL string, attempt int64) {
	state := cb.loadOrCreateState(endpointURL)
	if attempt != 0 && atomic.LoadInt64(&state.lastAttempt) != attempt {
		return
	}

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
