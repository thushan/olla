package resilience

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testOpenDuration = 30 * time.Second
const testProbeStaleness = 5 * time.Second

func testConfig(threshold int) Config {
	return Config{FailureThreshold: threshold, OpenDuration: testOpenDuration}
}

// TestBreaker_ClosedByDefault verifies a fresh Breaker starts closed and
// admits traffic without any half-open gating.
func TestBreaker_ClosedByDefault(t *testing.T) {
	b := New(testConfig(3))
	if open, attempt := b.IsOpen(testProbeStaleness); open || attempt != 0 {
		t.Fatalf("IsOpen() = (%v, %d) on a fresh breaker, want (false, 0)", open, attempt)
	}
}

// TestBreaker_TripsAtThreshold verifies the breaker opens exactly when
// consecutive failures reach the configured threshold, not before.
func TestBreaker_TripsAtThreshold(t *testing.T) {
	b := New(testConfig(3))

	b.RecordFailure(0)
	b.RecordFailure(0)
	if b.Tripped() {
		t.Fatal("breaker tripped before reaching the failure threshold")
	}

	b.RecordFailure(0)
	if !b.Tripped() {
		t.Fatal("breaker did not trip after reaching the failure threshold")
	}
	if open, _ := b.IsOpen(testProbeStaleness); !open {
		t.Fatal("expected IsOpen to block traffic immediately after tripping")
	}
}

// TestBreaker_SuccessResetsFailureCount verifies a success on the closed path
// resets the consecutive-failure counter, matching a standard circuit
// breaker's "consecutive" semantics rather than a rolling total.
func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := New(testConfig(3))

	b.RecordFailure(0)
	b.RecordFailure(0)
	b.RecordSuccess(0)
	if got := b.Failures(); got != 0 {
		t.Fatalf("Failures() = %d after a success, want 0", got)
	}

	b.RecordFailure(0)
	b.RecordFailure(0)
	if b.Tripped() {
		t.Fatal("breaker tripped despite the failure count having been reset by the intervening success")
	}
}

// TestBreaker_StaysOpenUntilRecoveryTimeout verifies a tripped breaker keeps
// rejecting every call, without ever admitting a half-open probe, until
// OpenDuration has elapsed since the last failure.
func TestBreaker_StaysOpenUntilRecoveryTimeout(t *testing.T) {
	b := NewTripped(testConfig(1), testOpenDuration/2) // tripped, but only half the recovery window has passed

	open, attempt := b.IsOpen(testProbeStaleness)
	if !open || attempt != 0 {
		t.Fatalf("IsOpen() = (%v, %d) before the recovery timeout elapsed, want (true, 0)", open, attempt)
	}
}

// TestBreaker_HalfOpen_SingleFlight is the regression test for the burst bug:
// once the recovery timeout elapses, only exactly one concurrent probe should
// be admitted until RecordSuccess/RecordFailure resolves it - not every
// caller that happens to observe the elapsed timeout simultaneously.
func TestBreaker_HalfOpen_SingleFlight(t *testing.T) {
	const goroutines = 200
	b := NewTripped(testConfig(1000), testOpenDuration+time.Second) // high threshold so a probe failure doesn't reopen mid-test

	var admitted int64
	var ready, start, done sync.WaitGroup
	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)

	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			if open, _ := b.IsOpen(testProbeStaleness); !open {
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

	if open, _ := b.IsOpen(testProbeStaleness); !open {
		t.Fatal("second probe was admitted before RecordSuccess/RecordFailure resolved the first")
	}
}

// TestBreaker_HalfOpen_SuccessCloses verifies RecordSuccess closes the
// breaker so subsequent traffic flows freely without going through the gate.
func TestBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	b := NewTripped(testConfig(3), testOpenDuration+time.Second)

	open, attempt := b.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	b.RecordSuccess(attempt)

	if b.Tripped() {
		t.Fatal("expected the breaker to be closed after RecordSuccess")
	}
	if got := b.LastAttemptNanos(); got != 0 {
		t.Fatalf("LastAttemptNanos() = %d after RecordSuccess, want reset to 0", got)
	}
	for range 10 {
		if open, _ := b.IsOpen(testProbeStaleness); open {
			t.Fatal("closed breaker rejected a request")
		}
	}
}

// TestBreaker_HalfOpen_FailureReopens verifies a probe failure that crosses
// the threshold trips the breaker back open and blocks further traffic.
func TestBreaker_HalfOpen_FailureReopens(t *testing.T) {
	b := NewTripped(testConfig(1), testOpenDuration+time.Second) // single failure re-opens

	open, attempt := b.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the first call after timeout elapsed to admit a half-open probe")
	}

	b.RecordFailure(attempt)

	if !b.Tripped() {
		t.Fatal("expected the breaker to be open after a threshold-crossing RecordFailure")
	}
	if open, _ := b.IsOpen(testProbeStaleness); !open {
		t.Fatal("re-opened breaker admitted a request before its timeout elapsed again")
	}
}

// TestBreaker_HalfOpen_StaleProbeReleased is the regression test for a hung
// probe that never calls RecordSuccess/RecordFailure: it used to wedge the
// endpoint indefinitely, since IsOpen read lastAttempt without ever
// re-stamping it on the read path. Backdates lastAttempt past the staleness
// window directly (rather than sleeping in the test) to simulate a probe
// that has been in flight too long without resolving.
func TestBreaker_HalfOpen_StaleProbeReleased(t *testing.T) {
	b := NewTripped(testConfig(1000), testOpenDuration+time.Second)

	if open, _ := b.IsOpen(testProbeStaleness); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	if open, _ := b.IsOpen(testProbeStaleness); !open {
		t.Fatal("a fresh in-flight probe must still block a second one")
	}

	b.SetLastAttemptNanos(time.Now().Add(-testProbeStaleness - time.Millisecond).UnixNano())

	if open, _ := b.IsOpen(testProbeStaleness); open {
		t.Fatal("a stale, never-resolved probe must not wedge the endpoint - a further probe should be admitted")
	}

	// The handover must re-stamp lastAttempt to now, not leave it stuck in
	// the past - otherwise every subsequent caller keeps reading a stale
	// timestamp and gets admitted too, defeating the single-flight gate.
	if open, _ := b.IsOpen(testProbeStaleness); !open {
		t.Fatal("the replacement probe just admitted must block the very next caller")
	}
}

// TestBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover races many
// goroutines against the same stale window to prove the last->now CAS
// handover admits exactly one replacement probe, not all of them.
func TestBreaker_HalfOpen_StaleProbeReleased_ConcurrentHandover(t *testing.T) {
	const goroutines = 200
	b := NewTripped(testConfig(1000), testOpenDuration+time.Second)

	if open, _ := b.IsOpen(testProbeStaleness); open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	b.SetLastAttemptNanos(time.Now().Add(-testProbeStaleness - time.Millisecond).UnixNano())

	var admitted int64
	var ready, start, done sync.WaitGroup
	ready.Add(goroutines)
	start.Add(1)
	done.Add(goroutines)

	for range goroutines {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			if open, _ := b.IsOpen(testProbeStaleness); !open {
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

// TestBreaker_ABARace_SupersededProbeResultDropped pins the ABA-race guard in
// both directions: a late result from a probe that a stale-handover already
// superseded must not clobber the replacement's state, whether that late
// result is a success closing a circuit the replacement re-opened, or a
// failure re-opening a circuit the replacement closed.
func TestBreaker_ABARace_SupersededProbeResultDropped(t *testing.T) {
	// Direction 1: stale success must not close a circuit the replacement
	// re-opened.
	b1 := NewTripped(testConfig(1), testOpenDuration+time.Second) // single failure re-opens

	open, _ := b1.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	attemptA1 := time.Now().Add(-testProbeStaleness - time.Millisecond).UnixNano()
	b1.SetLastAttemptNanos(attemptA1)

	open, attemptB1 := b1.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the stale handover to admit exactly one replacement probe")
	}
	if attemptB1 == 0 || attemptB1 == attemptA1 {
		t.Fatalf("expected a distinct non-zero attempt token for the replacement probe, got %d (A was %d)", attemptB1, attemptA1)
	}

	b1.RecordFailure(attemptB1)
	if !b1.Tripped() {
		t.Fatal("expected the circuit to be re-opened after the replacement probe's failure")
	}

	b1.RecordSuccess(attemptA1)
	if !b1.Tripped() {
		t.Fatal("a superseded probe's late success must not close the circuit the replacement re-opened")
	}

	// Direction 2: stale failure must not re-open a circuit the replacement
	// just closed.
	b2 := NewTripped(testConfig(1000), testOpenDuration+time.Second) // high threshold so a failure does not reopen

	open, _ = b2.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the first call to admit a half-open probe")
	}
	attemptA2 := time.Now().Add(-testProbeStaleness - time.Millisecond).UnixNano()
	b2.SetLastAttemptNanos(attemptA2)

	open, attemptB2 := b2.IsOpen(testProbeStaleness)
	if open {
		t.Fatal("expected the stale handover to admit exactly one replacement probe")
	}
	if attemptB2 == 0 || attemptB2 == attemptA2 {
		t.Fatalf("expected a distinct non-zero attempt token for the replacement probe, got %d (A was %d)", attemptB2, attemptA2)
	}

	b2.RecordSuccess(attemptB2)
	if b2.Tripped() {
		t.Fatal("expected the circuit to be closed after the replacement probe's success")
	}

	b2.RecordFailure(attemptA2)
	if b2.Tripped() {
		t.Fatal("a superseded probe's late failure must not re-open the circuit the replacement closed")
	}
	if got := b2.Failures(); got != 0 {
		t.Fatalf("superseded probe's failure must not increment the failure counter, got %d", got)
	}
}

// TestBreaker_Concurrent_MixedTrafficNoRace exercises IsOpen/RecordSuccess/
// RecordFailure concurrently under a realistic mixed workload. Its purpose is
// coverage for `go test -race`, not a specific assertion beyond "no panics
// and the breaker ends up in a self-consistent state".
func TestBreaker_Concurrent_MixedTrafficNoRace(t *testing.T) {
	const goroutines = 100
	const iterations = 200
	b := New(testConfig(5))

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				open, attempt := b.IsOpen(testProbeStaleness)
				if open {
					continue
				}
				if (id+j)%3 == 0 {
					b.RecordFailure(attempt)
				} else {
					b.RecordSuccess(attempt)
				}
			}
		}(i)
	}
	wg.Wait()

	// No panic and the breaker is still in a legal state (open or closed).
	_ = b.Tripped()
}
