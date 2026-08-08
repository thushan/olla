package olla

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/health"
)

// openCircuitBreaker builds a circuitBreaker already tripped into the open state,
// with its recovery timeout already elapsed so the next IsOpen() call is eligible
// to transition into half-open.
func openCircuitBreaker(threshold int64) *circuitBreaker {
	cb := &circuitBreaker{threshold: threshold}
	cb.state = 1 // open
	cb.lastFailure = time.Now().Add(-health.DefaultCircuitBreakerTimeout - time.Second).UnixNano()
	return cb
}

// TestCircuitBreaker_HalfOpen_SingleFlight is the regression test for the burst
// bug: once state CASes open -> half-open, every concurrent caller used to see
// state != 1 and pass straight through, flooding the recovering backend. Only
// exactly one concurrent probe should be admitted until RecordSuccess/RecordFailure
// resolves it.
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
			if open, _ := cb.IsOpen(); !open {
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
	if open, _ := cb.IsOpen(); !open {
		t.Fatal("second probe was admitted before RecordSuccess/RecordFailure resolved the first")
	}

	// A failure resolves the in-flight probe and, since threshold is far away,
	// leaves the breaker in half-open - exactly one further probe should now
	// be admitted. attempt=0 because this test resolves the gate directly, not
	// via a captured probe token.
	cb.RecordFailure(0)

	admitted = 0
	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)
	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			if open, _ := cb.IsOpen(); !open {
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}
	ready.Wait()
	start.Done()
	done.Wait()

	if admitted != 1 {
		t.Fatalf("after RecordFailure reset the gate: admitted = %d, want exactly 1", admitted)
	}
}

// TestCircuitBreaker_HalfOpen_SuccessCloses verifies RecordSuccess closes the
// breaker so subsequent traffic flows freely without going through the gate.
func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cb := openCircuitBreaker(3)

	open, attempt := cb.IsOpen()
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordSuccess(attempt)

	if atomic.LoadInt64(&cb.state) != 0 {
		t.Fatalf("state = %d after RecordSuccess, want closed (0)", cb.state)
	}
	if atomic.LoadInt64(&cb.lastAttempt) != 0 {
		t.Fatalf("lastAttempt = %d after RecordSuccess, want reset to 0", cb.lastAttempt)
	}
	for range 10 {
		if open, _ := cb.IsOpen(); open {
			t.Fatal("closed breaker rejected a request")
		}
	}
}

// TestCircuitBreaker_HalfOpen_StaleProbeReleased is the regression test for
// F2: a hung probe that never calls RecordSuccess/RecordFailure used to wedge
// the endpoint indefinitely, since lastAttempt only ever got reset by one of
// those two calls. Backdates lastAttempt past halfOpenStaleness directly
// (rather than sleeping in the test) to simulate a probe that has been
// in-flight too long without resolving.
func TestCircuitBreaker_HalfOpen_StaleProbeReleased(t *testing.T) {
	cb := openCircuitBreaker(1000)

	if open, _ := cb.IsOpen(); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	if open, _ := cb.IsOpen(); !open {
		t.Fatal("a fresh in-flight probe must still block a second one")
	}

	// Simulate the first probe hanging well past the staleness window without
	// ever resolving.
	atomic.StoreInt64(&cb.lastAttempt, time.Now().Add(-halfOpenStaleness-time.Millisecond).UnixNano())

	if open, _ := cb.IsOpen(); open {
		t.Fatal("a stale, never-resolved probe must not wedge the endpoint - a further probe should be admitted")
	}

	// The handover must re-stamp lastAttempt to now, not leave it stuck in the
	// past - otherwise every subsequent caller keeps reading a stale timestamp
	// and gets admitted too, defeating the single-flight gate entirely.
	if open, _ := cb.IsOpen(); !open {
		t.Fatal("the replacement probe just admitted must block the very next caller")
	}
}

// TestCircuitBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover races many
// goroutines against the same stale window to prove the last->now CAS handover
// admits exactly one replacement probe, not all of them. Before the fix, every
// goroutine that observed the stale window read the same never-updated
// lastAttempt and was admitted, regardless of how many arrived.
func TestCircuitBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover(t *testing.T) {
	const goroutines = 200
	cb := openCircuitBreaker(1000)

	// Win the initial half-open slot and immediately backdate it past the
	// staleness window, so every goroutine below races the handover CAS.
	if open, _ := cb.IsOpen(); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	atomic.StoreInt64(&cb.lastAttempt, time.Now().Add(-halfOpenStaleness-time.Millisecond).UnixNano())

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
			if open, _ := cb.IsOpen(); !open {
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

// TestCircuitBreaker_HalfOpen_FailureReopens verifies a probe failure that
// crosses the threshold trips the breaker back open and blocks further traffic.
func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := openCircuitBreaker(1) // single failure re-opens

	open, attempt := cb.IsOpen()
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordFailure(attempt)

	if atomic.LoadInt64(&cb.state) != 1 {
		t.Fatalf("state = %d after threshold-crossing RecordFailure, want open (1)", cb.state)
	}
	if open, _ := cb.IsOpen(); !open {
		t.Fatal("re-opened breaker admitted a request before its timeout elapsed again")
	}
}

// TestCircuitBreaker_ABARace_SupersededProbeResultDropped pins the ABA-race
// guard: a late result from a probe that a stale-handover already superseded
// must not clobber the replacement's state. Without attempt correlation, the
// original hung probe's late RecordSuccess would close a circuit the
// replacement had just legitimately re-opened via RecordFailure (and the
// inverse: a stale failure re-opening a circuit the replacement just closed).
func TestCircuitBreaker_ABARace_SupersededProbeResultDropped(t *testing.T) {
	// Direction 1: stale success must not close a circuit the replacement
	// re-opened.
	cb := openCircuitBreaker(1) // single failure re-opens

	// Admit probe A (the call transitions open -> half-open and stamps the
	// single-flight gate); discard its token and backdate lastAttempt past the
	// staleness window directly (rather than sleeping) to simulate a hung probe.
	// attemptA is taken as that backdated stamp so it is guaranteed distinct
	// from attemptB - two real IsOpen() calls a few lines apart can share the
	// same UnixNano() value on a coarser system clock.
	open, _ := cb.IsOpen()
	if open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	attemptA := time.Now().Add(-halfOpenStaleness - time.Millisecond).UnixNano()
	atomic.StoreInt64(&cb.lastAttempt, attemptA)

	// A is stale; a stale-handover admits replacement probe B with a fresh token.
	open, attemptB := cb.IsOpen()
	if open {
		t.Fatal("expected the stale handover to admit exactly one replacement probe")
	}
	if attemptB == 0 || attemptB == attemptA {
		t.Fatalf("expected a distinct non-zero attempt token for the replacement probe, got %d (A was %d)", attemptB, attemptA)
	}

	// B fails first and legitimately re-opens the circuit (threshold == 1).
	cb.RecordFailure(attemptB)
	if atomic.LoadInt64(&cb.state) != 1 {
		t.Fatal("expected the circuit to be re-opened after the replacement probe's failure")
	}

	// A's hung request finally resolves - as a SUCCESS. Without attempt
	// correlation this would close the circuit B just re-opened.
	cb.RecordSuccess(attemptA)
	if atomic.LoadInt64(&cb.state) != 1 {
		t.Fatal("a superseded probe's late success must not close the circuit the replacement re-opened")
	}

	// Direction 2 (inverse): stale failure must not re-open a circuit the
	// replacement just closed.
	cb2 := openCircuitBreaker(1000) // high threshold so a failure does not reopen

	open, _ = cb2.IsOpen()
	if open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	attemptA2 := time.Now().Add(-halfOpenStaleness - time.Millisecond).UnixNano()
	atomic.StoreInt64(&cb2.lastAttempt, attemptA2)

	open, attemptB2 := cb2.IsOpen()
	if open {
		t.Fatal("expected the stale handover to admit exactly one replacement probe")
	}
	if attemptB2 == 0 || attemptB2 == attemptA2 {
		t.Fatalf("expected a distinct non-zero attempt token for the replacement probe, got %d (A was %d)", attemptB2, attemptA2)
	}

	// B succeeds first and legitimately closes the circuit.
	cb2.RecordSuccess(attemptB2)
	if atomic.LoadInt64(&cb2.state) != 0 {
		t.Fatal("expected the circuit to be closed after the replacement probe's success")
	}

	// A's late result arrives as a FAILURE. Without correlation this would
	// re-open the circuit B just closed and bump the failure counter.
	cb2.RecordFailure(attemptA2)
	if atomic.LoadInt64(&cb2.state) != 0 {
		t.Fatal("a superseded probe's late failure must not re-open the circuit the replacement closed")
	}
	if failures := atomic.LoadInt64(&cb2.failures); failures != 0 {
		t.Fatalf("superseded probe's failure must not increment the failure counter, got %d", failures)
	}
}
