package health

import (
	"errors"
	"net/http"
	"time"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/thushan/olla/internal/adapter/resilience"
)

var (
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)

// HTTPClient interface for better testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CircuitBreaker tracks failure rates and prevents cascading failures. It is
// a thin, endpoint-keyed adapter around resilience.Breaker - the shared
// closed/open/half-open state machine also used by proxy/olla's circuit
// breaker - adding only what is specific to health checks: one Breaker per
// health-check URL, and a per-endpoint half-open staleness window derived
// from that endpoint's CheckTimeout (see probeStaleness).
// TODO: (HOT-RELOAD) Add cleanup mechanism for removed endpoints when hot reload is implemented
// The endpoints xsync.Map will accumulate stale entries for removed/changed endpoints without TTL
type CircuitBreaker struct {
	endpoints        *xsync.Map[string, *resilience.Breaker]
	failureThreshold int
	timeout          time.Duration
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		endpoints:        xsync.NewMap[string, *resilience.Breaker](),
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
	return state.IsOpen(probeStaleness(checkTimeout))
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
	state.RecordSuccess(attempt)
}

// RecordFailure records a failed probe for endpointURL. See RecordSuccess
// for the attempt correlation contract.
func (cb *CircuitBreaker) RecordFailure(endpointURL string, attempt int64) {
	cb.loadOrCreateState(endpointURL).RecordFailure(attempt)
}

func (cb *CircuitBreaker) CleanupEndpoint(endpointURL string) {
	cb.endpoints.Delete(endpointURL)
}

func (cb *CircuitBreaker) GetActiveEndpoints() []string {
	var endpoints []string
	cb.endpoints.Range(func(key string, _ *resilience.Breaker) bool {
		endpoints = append(endpoints, key)
		return true
	})
	return endpoints
}

func (cb *CircuitBreaker) loadOrCreateState(endpointURL string) *resilience.Breaker {
	state, _ := cb.endpoints.LoadOrCompute(endpointURL, func() (newValue *resilience.Breaker, cancel bool) {
		return resilience.New(resilience.Config{
			FailureThreshold: cb.failureThreshold,
			OpenDuration:     cb.timeout,
		}), false
	})
	return state
}
