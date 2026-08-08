// Package resilience holds the shared per-endpoint circuit breaker state
// machine used by both proxy/olla (proxied inference requests) and
// adapter/health (health-check probes). The two call sites hand-rolled
// functionally identical breakers - closed/open/half-open, single-flight
// half-open probe gating, stale-handover CAS, ABA-safe attempt tokens - and
// converged on the same bug fixes independently, with one lagging the other
// until a fourth review caught the gap. Keeping ONE implementation means a
// correctness fix only has to land once.
//
// This package deliberately knows nothing about endpoints, URLs, maps,
// cleanup sweeps or event emission - those are call-site concerns and stay in
// proxy/olla and adapter/health respectively. Neither of those two packages
// may import the other, so the shared type lives here instead, one level up,
// with no dependency on either.
package resilience

import (
	"sync/atomic"
	"time"
)

// Config configures a Breaker's failure threshold and open-state duration.
//
// The half-open single-flight staleness window (how long an in-flight probe
// is trusted before a replacement is admitted) is deliberately NOT part of
// Config - it is supplied per IsOpen call instead. The two adapters derive it
// differently: adapter/health computes it per endpoint from that endpoint's
// own CheckTimeout (one CircuitBreaker instance is shared across every
// endpoint, keyed by health-check URL, so the window has to vary per call),
// while proxy/olla uses one fixed constant sized to inference response
// budgets, applied uniformly (one Breaker instance per endpoint, so the
// constant could live on the Config there too, but taking it as a call
// parameter keeps both adapters' derivation logic entirely their own).
type Config struct {
	FailureThreshold int
	OpenDuration     time.Duration
}

// Breaker states. half-open is a real, persisted state (not merely derived on
// each call) - see IsOpen for why that matters.
const (
	stateClosed int32 = iota
	stateOpen
	stateHalfOpen
)

// Breaker is a single-endpoint circuit breaker: closed, open, half-open, with
// the half-open state gated to exactly one in-flight probe at a time. It
// holds no key and no map - callers own one Breaker per endpoint (or key it
// themselves, as adapter/health does with a single shared map).
type Breaker struct {
	failures     atomic.Int64
	lastFailure  atomic.Int64
	lastAttempt  atomic.Int64
	state        atomic.Int32
	threshold    int64
	openDuration time.Duration
}

// New builds a closed Breaker with the given configuration.
func New(cfg Config) *Breaker {
	return &Breaker{
		threshold:    int64(cfg.FailureThreshold),
		openDuration: cfg.OpenDuration,
	}
}

// NewTripped builds a Breaker already in the open state, with its last
// failure backdated by failedAgo before now. Exported for test setup that
// needs a breaker whose recovery timeout has already elapsed (eligible to
// transition to half-open) without sleeping in the test.
func NewTripped(cfg Config, failedAgo time.Duration) *Breaker {
	b := New(cfg)
	b.state.Store(stateOpen)
	b.lastFailure.Store(time.Now().Add(-failedAgo).UnixNano())
	return b
}

// IsOpen reports whether requests should be blocked. probeStaleness is the
// half-open single-flight staleness window - see Config for why it is a call
// parameter rather than fixed at construction.
//
// The second return value, attempt, identifies the specific half-open probe
// this call admitted (0 when the circuit is closed and no half-open gating
// applies, and 0 when this caller was rejected). Callers that proceed to
// perform the probe MUST pass attempt back into the matching
// RecordSuccess/RecordFailure call - see those for why: a late result from a
// probe that was superseded by a stale-handover replacement must not clobber
// the replacement's state.
func (b *Breaker) IsOpen(probeStaleness time.Duration) (open bool, attempt int64) {
	state := b.state.Load()
	if state == stateClosed {
		return false, 0
	}

	if state == stateOpen {
		lastFailure := b.lastFailure.Load()
		if !time.Unix(0, lastFailure).Add(b.openDuration).Before(time.Now()) {
			// Recovery timeout hasn't elapsed yet - stay fully open.
			return true, 0
		}
		// Timeout elapsed: attempt open -> half-open. Every caller that
		// reaches here (whether it won this CAS or another goroutine already
		// flipped it) falls through to the half-open single-flight gate
		// below. half-open is a persisted state, not merely "isOpen &&
		// timeout elapsed" recomputed on every call, mirroring olla's
		// original mechanics exactly (this is a pure refactor, not a
		// behaviour change). The practical effect - RecordFailure only
		// escalates back to stateOpen when failures >= threshold, so a
		// half-open failure below threshold would leave the breaker in
		// half-open rather than forcing a fresh openDuration wait - is not
		// reachable through the public API today: failures only resets on
		// success, so every non-closed state already has failures >=
		// threshold by construction, and RecordFailure always re-escalates.
		// The branch exists for future-proofing (e.g. a caller configuring a
		// threshold that legitimately allows several half-open attempts), not
		// because current callers depend on it.
		b.state.CompareAndSwap(stateOpen, stateHalfOpen)
	}

	// Half-open: admit exactly one probe. lastAttempt is the single-flight
	// gate - only the goroutine that wins the 0->now CAS is let through
	// immediately; every other concurrent caller is rejected until
	// RecordSuccess/RecordFailure resolves the probe and resets it. If the
	// outstanding probe is stale (older than probeStaleness - e.g. a hung
	// probe that never resolved), the slot is handed to exactly one
	// replacement caller via a last->now CAS: a plain read-and-compare here
	// (the bug this replaces) would admit every caller for as long as the
	// window keeps being exceeded, since nothing re-stamps lastAttempt on the
	// read path - the circuit effectively stays wide open forever after a
	// single stuck probe.
	now := time.Now().UnixNano()
	if b.lastAttempt.CompareAndSwap(0, now) {
		return false, now
	}
	lastAttempt := b.lastAttempt.Load()
	if time.Unix(0, lastAttempt).Add(probeStaleness).After(time.Now()) {
		return true, 0
	}
	if b.lastAttempt.CompareAndSwap(lastAttempt, now) {
		return false, now
	}
	return true, 0
}

// RecordSuccess records a successful probe. attempt must be the value IsOpen
// returned when it admitted this probe (0 for the normal closed-circuit path,
// where no correlation is needed or performed).
//
// ABA race note: when attempt is non-zero (a half-open probe), this only
// applies if attempt still matches the breaker's lastAttempt - i.e. this call
// is the most recent admitted probe, not one that a stale-handover already
// superseded. Without this check, a very late result from a hung probe that
// finally resolves AFTER a replacement probe has already run to completion
// would clobber the replacement's outcome (e.g. a stale success closing a
// circuit the replacement had just legitimately re-opened, or vice versa).
// This closes the gap for the common case - a superseded probe's result
// arriving after the replacement has already resolved - but is not a full
// fencing solution: two admitted probes racing to record at literally the
// same instant are not additionally serialised beyond the atomics already in
// play. That residual window is understood behaviour, not a bug.
func (b *Breaker) RecordSuccess(attempt int64) {
	if attempt != 0 && b.lastAttempt.Load() != attempt {
		return
	}
	b.failures.Store(0)
	b.lastAttempt.Store(0)
	b.state.Store(stateClosed)
}

// RecordFailure records a failed probe. See RecordSuccess for the attempt
// correlation contract.
func (b *Breaker) RecordFailure(attempt int64) {
	if attempt != 0 && b.lastAttempt.Load() != attempt {
		return
	}
	failures := b.failures.Add(1)
	b.lastFailure.Store(time.Now().UnixNano())
	b.lastAttempt.Store(0)

	if failures >= b.threshold {
		b.state.Store(stateOpen)
	}
	// Below threshold: leave the state as-is (not currently reachable via the
	// public API in a non-closed state - see the half-open persistence note
	// on IsOpen).
}

// Tripped reports whether the breaker is anything other than closed (open or
// half-open), without evaluating the half-open gate or admitting a probe.
// Used by adapters for state inspection outside the request path (e.g.
// proxy/olla's stale circuit-breaker cleanup sweep).
func (b *Breaker) Tripped() bool {
	return b.state.Load() != stateClosed
}

// Failures returns the current consecutive-failure count.
func (b *Breaker) Failures() int64 {
	return b.failures.Load()
}

// LastFailureNanos returns the UnixNano timestamp of the last recorded
// failure, or 0 if none has been recorded.
func (b *Breaker) LastFailureNanos() int64 {
	return b.lastFailure.Load()
}

// LastAttemptNanos returns the UnixNano timestamp of the current half-open
// single-flight gate, or 0 if none is in flight.
func (b *Breaker) LastAttemptNanos() int64 {
	return b.lastAttempt.Load()
}

// SetOpen forces the breaker to the open state (v true) or the closed state
// (v false). Exported for test construction of specific scenarios;
// production callers should go through RecordSuccess/RecordFailure, which
// own the state's lifecycle.
func (b *Breaker) SetOpen(v bool) {
	if v {
		b.state.Store(stateOpen)
	} else {
		b.state.Store(stateClosed)
	}
}

// SetLastFailureNanos backdates or clears the last-failure timestamp.
// Exported for test construction; see SetOpen.
func (b *Breaker) SetLastFailureNanos(v int64) {
	b.lastFailure.Store(v)
}

// SetLastAttemptNanos backdates or clears the half-open single-flight gate.
// Exported for test construction of specific race scenarios (e.g. a stale or
// still-in-flight probe); see SetOpen.
func (b *Breaker) SetLastAttemptNanos(v int64) {
	b.lastAttempt.Store(v)
}
