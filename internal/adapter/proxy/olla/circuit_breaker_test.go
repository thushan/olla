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
			if !cb.IsOpen() {
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
	if !cb.IsOpen() {
		t.Fatal("second probe was admitted before RecordSuccess/RecordFailure resolved the first")
	}

	// A failure resolves the in-flight probe and, since threshold is far away,
	// leaves the breaker in half-open - exactly one further probe should now
	// be admitted.
	cb.RecordFailure()

	admitted = 0
	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)
	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			if !cb.IsOpen() {
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

	if cb.IsOpen() {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordSuccess()

	if atomic.LoadInt64(&cb.state) != 0 {
		t.Fatalf("state = %d after RecordSuccess, want closed (0)", cb.state)
	}
	if atomic.LoadInt64(&cb.lastAttempt) != 0 {
		t.Fatalf("lastAttempt = %d after RecordSuccess, want reset to 0", cb.lastAttempt)
	}
	for range 10 {
		if cb.IsOpen() {
			t.Fatal("closed breaker rejected a request")
		}
	}
}

// TestCircuitBreaker_HalfOpen_FailureReopens verifies a probe failure that
// crosses the threshold trips the breaker back open and blocks further traffic.
func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := openCircuitBreaker(1) // single failure re-opens

	if cb.IsOpen() {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	cb.RecordFailure()

	if atomic.LoadInt64(&cb.state) != 1 {
		t.Fatalf("state = %d after threshold-crossing RecordFailure, want open (1)", cb.state)
	}
	if !cb.IsOpen() {
		t.Fatal("re-opened breaker admitted a request before its timeout elapsed again")
	}
}
