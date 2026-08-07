package health

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

const testEndpoint = "http://endpoint-under-test"

// testCheckTimeout mirrors config.go's own default for Endpoint.CheckTimeout
// (2s), so probeStaleness(testCheckTimeout) == 5s (2*2s + 1s margin) in these
// tests - the same budget a real endpoint would get.
const testCheckTimeout = 2 * time.Second

// openCircuitBreaker builds a CircuitBreaker with a single endpoint already
// tripped into the open state, its recovery timeout already elapsed so the
// next IsOpen() call is eligible to transition into half-open. Mirrors
// proxy/olla/circuit_breaker_test.go's openCircuitBreaker helper.
func openCircuitBreaker(threshold int) *CircuitBreaker {
	cb := &CircuitBreaker{
		endpoints:        xsync.NewMap[string, *circuitState](),
		failureThreshold: threshold,
		timeout:          DefaultCircuitBreakerTimeout,
	}
	state := cb.loadOrCreateState(testEndpoint)
	atomic.StoreInt32(&state.isOpen, 1)
	atomic.StoreInt64(&state.lastFailure, time.Now().Add(-DefaultCircuitBreakerTimeout-time.Second).UnixNano())
	return cb
}

// TestProbeStaleness_DerivedFromCheckTimeout pins the window-derivation fix:
// the half-open staleness window must scale with the endpoint's own
// CheckTimeout (checker.go wraps each probe attempt, including internal
// retries, in a CheckTimeout*2 context), not a single fixed constant that
// could be tighter than a legitimately slow probe's real budget.
func TestProbeStaleness_DerivedFromCheckTimeout(t *testing.T) {
	tests := []struct {
		name         string
		checkTimeout time.Duration
		want         time.Duration
	}{
		{"2s config default", 2 * time.Second, 5 * time.Second},
		{"scales with a larger configured timeout", 10 * time.Second, 21 * time.Second},
		{"zero falls back to the config default", 0, 5 * time.Second},
		{"negative falls back to the config default", -time.Second, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := probeStaleness(tt.checkTimeout)
			if got != tt.want {
				t.Errorf("probeStaleness(%v) = %v, want %v", tt.checkTimeout, got, tt.want)
			}
		})
	}
}

// TestCircuitBreaker_HalfOpen_SlowButAliveProbeWithinBudget_NoHandover is the
// regression test for the review finding: a fixed 1s staleness window was
// tighter than checker.go's real CheckTimeout*2 probe budget (5s for the 2s
// config default), so a legitimately slow-but-alive probe could trigger a
// spurious stale-handover and admit a second concurrent probe against a
// backend that was merely slow, not hung. A probe backdated to within the
// derived window must still block a second caller.
func TestCircuitBreaker_HalfOpen_SlowButAliveProbeWithinBudget_NoHandover(t *testing.T) {
	cb := openCircuitBreaker(1000)

	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}

	// Backdate to 4s ago - within the 5s budget (2*2s + 1s margin) for a 2s
	// CheckTimeout, i.e. still legitimately in flight, not hung.
	state, _ := cb.endpoints.Load(testEndpoint)
	backdated := time.Now().Add(-4 * time.Second).UnixNano()
	atomic.StoreInt64(&state.lastAttempt, backdated)

	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
		t.Fatal("a probe still within its CheckTimeout-derived budget must not be superseded - a second caller must be rejected")
	}

	// Sanity: the in-budget probe's stamp is still authoritative - IsOpen must
	// not have performed a handover CAS against it.
	if atomic.LoadInt64(&state.lastAttempt) != backdated {
		t.Fatalf("lastAttempt changed even though the probe was within budget: got %d, want unchanged %d", atomic.LoadInt64(&state.lastAttempt), backdated)
	}
}

// TestCircuitBreaker_HalfOpen_HungProbeBeyondBudget_TriggersHandover is the
// inverse: a probe backdated PAST the CheckTimeout-derived budget is
// legitimately hung, and a replacement must be admitted.
func TestCircuitBreaker_HalfOpen_HungProbeBeyondBudget_TriggersHandover(t *testing.T) {
	cb := openCircuitBreaker(1000)

	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}

	// Backdate past the 5s budget.
	state, _ := cb.endpoints.Load(testEndpoint)
	atomic.StoreInt64(&state.lastAttempt, time.Now().Add(-5*time.Second-time.Millisecond).UnixNano())

	open, replacementAttempt := cb.IsOpen(testEndpoint, testCheckTimeout)
	if open {
		t.Fatal("a hung probe past its CheckTimeout-derived budget must be superseded - a replacement should be admitted")
	}
	if replacementAttempt == 0 {
		t.Fatal("expected a non-zero attempt token for the admitted replacement probe")
	}
}

// TestCircuitBreaker_ABARace_SupersededProbeResultDropped pins the ABA-race
// guard: a late result from a probe that a stale-handover already superseded
// must not clobber the replacement's state. Without attempt correlation, the
// original hung probe's late RecordFailure would re-open a circuit the
// replacement had just legitimately closed via RecordSuccess.
func TestCircuitBreaker_ABARace_SupersededProbeResultDropped(t *testing.T) {
	cb := openCircuitBreaker(1000)
	state, _ := cb.endpoints.Load(testEndpoint)

	// Simulate probe A already in flight, stamped well in the past (rather
	// than chaining two real IsOpen() calls a few lines apart, whose
	// UnixNano() timestamps could otherwise coincide on a coarser system
	// clock and defeat the "distinct token" assertion below).
	attemptA := time.Now().Add(-10 * time.Second).UnixNano()
	atomic.StoreInt64(&state.lastAttempt, attemptA)

	// A is stale (10s ago, well past the 5s budget); a stale-handover admits
	// replacement probe B.
	_, attemptB := cb.IsOpen(testEndpoint, testCheckTimeout)
	if attemptB == 0 || attemptB == attemptA {
		t.Fatalf("expected a distinct non-zero attempt token for the replacement probe, got %d (A was %d)", attemptB, attemptA)
	}

	// B resolves first and legitimately closes the circuit.
	cb.RecordSuccess(testEndpoint, attemptB)
	if atomic.LoadInt32(&state.isOpen) != 0 {
		t.Fatal("expected the circuit to be closed after the replacement probe's success")
	}

	// A's hung request finally resolves - as a FAILURE. Without attempt
	// correlation this would re-open the circuit B just closed.
	cb.RecordFailure(testEndpoint, attemptA)

	if atomic.LoadInt32(&state.isOpen) != 0 {
		t.Fatal("a superseded probe's late result must not clobber the replacement's state - circuit must still be closed")
	}
	if atomic.LoadInt64(&state.failures) != 0 {
		t.Fatalf("superseded probe's failure must not increment the failure counter, got %d", atomic.LoadInt64(&state.failures))
	}
}

// TestCircuitBreaker_HalfOpen_StaleProbeReleased is the regression test for
// the same wide-open-degradation class F2 fixed in proxy/olla's circuit
// breaker: a hung probe that never calls RecordSuccess/RecordFailure used to
// wedge the endpoint indefinitely here too, since IsOpen() read lastAttempt
// without ever re-stamping it. Backdates lastAttempt past the derived
// staleness window directly (rather than sleeping in the test) to simulate a
// probe that has been in-flight too long without resolving.
func TestCircuitBreaker_HalfOpen_StaleProbeReleased(t *testing.T) {
	cb := openCircuitBreaker(1000)

	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
		t.Fatal("a fresh in-flight probe must still block a second one")
	}

	// Simulate the first probe hanging well past the staleness window without
	// ever resolving.
	state, _ := cb.endpoints.Load(testEndpoint)
	atomic.StoreInt64(&state.lastAttempt, time.Now().Add(-probeStaleness(testCheckTimeout)-time.Millisecond).UnixNano())

	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
		t.Fatal("a stale, never-resolved probe must not wedge the endpoint - a further probe should be admitted")
	}

	// The handover must re-stamp lastAttempt to now, not leave it stuck in the
	// past - otherwise every subsequent caller keeps reading a stale timestamp
	// and gets admitted too, defeating the single-flight gate entirely.
	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
		t.Fatal("the replacement probe just admitted must block the very next caller")
	}
}

// TestCircuitBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover races
// many goroutines against the same stale window to prove the last->now CAS
// handover admits exactly one replacement probe, not all of them. Before the
// fix, every goroutine that observed the stale window read the same
// never-updated lastAttempt and was admitted, regardless of how many
// arrived - the circuit stayed wide open for as long as the window kept
// being exceeded.
func TestCircuitBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover(t *testing.T) {
	const goroutines = 200
	cb := openCircuitBreaker(1000)

	// Win the initial half-open slot and immediately backdate it past the
	// staleness window, so every goroutine below races the handover CAS.
	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	state, _ := cb.endpoints.Load(testEndpoint)
	atomic.StoreInt64(&state.lastAttempt, time.Now().Add(-probeStaleness(testCheckTimeout)-time.Millisecond).UnixNano())

	var admitted int64
	var ready sync.WaitGroup
	var start sync.WaitGroup
	var done sync.WaitGroup

	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)

	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}

	ready.Wait()
	start.Done()
	done.Wait()

	if admitted != 1 {
		t.Fatalf("expected exactly 1 replacement probe admitted across the stale handover, got %d", admitted)
	}
}

// TestCircuitBreaker_HalfOpen_SingleFlight is the regression test for the
// burst bug: once the recovery timeout elapses, every concurrent caller used
// to be admitted as a half-open probe. Only exactly one concurrent probe
// should be admitted until RecordSuccess/RecordFailure resolves it.
func TestCircuitBreaker_HalfOpen_SingleFlight(t *testing.T) {
	const goroutines = 200
	cb := openCircuitBreaker(1000) // high threshold so a probe failure doesn't reopen mid-test

	var admitted int64
	var ready sync.WaitGroup
	var start sync.WaitGroup
	var done sync.WaitGroup

	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)

	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait() // release all goroutines at once to maximise contention
			if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}

	ready.Wait()
	start.Done()
	done.Wait()

	if admitted != 1 {
		t.Fatalf("admitted = %d concurrent probes in half-open, want exactly 1", admitted)
	}

	// Until the probe resolves, every further caller must still be rejected.
	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
		t.Fatal("second probe was admitted before RecordSuccess/RecordFailure resolved the first")
	}
}

// TestCircuitBreaker_HalfOpen_SuccessCloses verifies RecordSuccess closes the
// breaker so subsequent traffic flows freely without going through the gate.
func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cb := openCircuitBreaker(3)

	open, attempt := cb.IsOpen(testEndpoint, testCheckTimeout)
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordSuccess(testEndpoint, attempt)

	state, _ := cb.endpoints.Load(testEndpoint)
	if atomic.LoadInt32(&state.isOpen) != 0 {
		t.Fatalf("isOpen = %d after RecordSuccess, want closed (0)", state.isOpen)
	}
	if atomic.LoadInt64(&state.lastAttempt) != 0 {
		t.Fatalf("lastAttempt = %d after RecordSuccess, want reset to 0", state.lastAttempt)
	}
	for range 10 {
		if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); open {
			t.Fatal("closed breaker rejected a request")
		}
	}
}

// TestCircuitBreaker_HalfOpen_FailureReopens verifies a probe failure that
// crosses the threshold trips the breaker back open and blocks further
// traffic.
func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := openCircuitBreaker(1) // single failure re-opens

	open, attempt := cb.IsOpen(testEndpoint, testCheckTimeout)
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordFailure(testEndpoint, attempt)

	state, _ := cb.endpoints.Load(testEndpoint)
	if atomic.LoadInt32(&state.isOpen) != 1 {
		t.Fatalf("isOpen = %d after threshold-crossing RecordFailure, want open (1)", state.isOpen)
	}
	if open, _ := cb.IsOpen(testEndpoint, testCheckTimeout); !open {
		t.Fatal("re-opened breaker admitted a request before its timeout elapsed again")
	}
}
