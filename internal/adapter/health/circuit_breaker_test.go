package health

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

const testEndpoint = "http://endpoint-under-test"

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

// TestCircuitBreaker_HalfOpen_StaleProbeReleased is the regression test for
// the same wide-open-degradation class F2 fixed in proxy/olla's circuit
// breaker: a hung probe that never calls RecordSuccess/RecordFailure used to
// wedge the endpoint indefinitely here too, since IsOpen() read lastAttempt
// without ever re-stamping it. Backdates lastAttempt past
// DefaultHalfOpenProbeStaleness directly (rather than sleeping in the test)
// to simulate a probe that has been in-flight too long without resolving.
func TestCircuitBreaker_HalfOpen_StaleProbeReleased(t *testing.T) {
	cb := openCircuitBreaker(1000)

	if cb.IsOpen(testEndpoint) {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	if !cb.IsOpen(testEndpoint) {
		t.Fatal("a fresh in-flight probe must still block a second one")
	}

	// Simulate the first probe hanging well past the staleness window without
	// ever resolving.
	state, _ := cb.endpoints.Load(testEndpoint)
	atomic.StoreInt64(&state.lastAttempt, time.Now().Add(-DefaultHalfOpenProbeStaleness-time.Millisecond).UnixNano())

	if cb.IsOpen(testEndpoint) {
		t.Fatal("a stale, never-resolved probe must not wedge the endpoint - a further probe should be admitted")
	}

	// The handover must re-stamp lastAttempt to now, not leave it stuck in the
	// past - otherwise every subsequent caller keeps reading a stale timestamp
	// and gets admitted too, defeating the single-flight gate entirely.
	if !cb.IsOpen(testEndpoint) {
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
	if cb.IsOpen(testEndpoint) {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	state, _ := cb.endpoints.Load(testEndpoint)
	atomic.StoreInt64(&state.lastAttempt, time.Now().Add(-DefaultHalfOpenProbeStaleness-time.Millisecond).UnixNano())

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
			if !cb.IsOpen(testEndpoint) {
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
			if !cb.IsOpen(testEndpoint) {
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
	if !cb.IsOpen(testEndpoint) {
		t.Fatal("second probe was admitted before RecordSuccess/RecordFailure resolved the first")
	}
}

// TestCircuitBreaker_HalfOpen_SuccessCloses verifies RecordSuccess closes the
// breaker so subsequent traffic flows freely without going through the gate.
func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cb := openCircuitBreaker(3)

	if cb.IsOpen(testEndpoint) {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordSuccess(testEndpoint)

	state, _ := cb.endpoints.Load(testEndpoint)
	if atomic.LoadInt32(&state.isOpen) != 0 {
		t.Fatalf("isOpen = %d after RecordSuccess, want closed (0)", state.isOpen)
	}
	if atomic.LoadInt64(&state.lastAttempt) != 0 {
		t.Fatalf("lastAttempt = %d after RecordSuccess, want reset to 0", state.lastAttempt)
	}
	for range 10 {
		if cb.IsOpen(testEndpoint) {
			t.Fatal("closed breaker rejected a request")
		}
	}
}

// TestCircuitBreaker_HalfOpen_FailureReopens verifies a probe failure that
// crosses the threshold trips the breaker back open and blocks further
// traffic.
func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := openCircuitBreaker(1) // single failure re-opens

	if cb.IsOpen(testEndpoint) {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordFailure(testEndpoint)

	state, _ := cb.endpoints.Load(testEndpoint)
	if atomic.LoadInt32(&state.isOpen) != 1 {
		t.Fatalf("isOpen = %d after threshold-crossing RecordFailure, want open (1)", state.isOpen)
	}
	if !cb.IsOpen(testEndpoint) {
		t.Fatal("re-opened breaker admitted a request before its timeout elapsed again")
	}
}
