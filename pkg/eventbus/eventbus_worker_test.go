package eventbus

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWorkerPool_NoGoroutineLeaks verifies the worker pool doesn't leak goroutines
func TestWorkerPool_NoGoroutineLeaks(t *testing.T) {
	// Get baseline goroutine count
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	// Create EventBus with worker pool
	eb := New[int]()

	// Subscribe to events
	ctx, cancel := context.WithCancel(context.Background())
	ch, cleanup := eb.Subscribe(ctx)
	defer cleanup()
	defer cancel()

	// Publish many events asynchronously
	const numEvents = 10000
	for i := range numEvents {
		eb.PublishAsync(i)
	}

	// Count received events
	received := 0
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case <-ch:
			received++
			if received >= numEvents/2 { // Just check we got a good portion
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	// Shutdown EventBus. Shutdown() itself blocks on wg.Wait() until every
	// worker goroutine has exited, so the worker pool is deterministically
	// gone by the time this call returns - no polling needed for that part.
	eb.Shutdown()

	// What's left to settle asynchronously is the cleanup-ticker loop exiting
	// on close(stopCleanup) and (if still running) the per-subscriber
	// ctx-cancellation watcher goroutine - neither exposes a completion
	// signal, and goroutine-count-based leak detection has no synchronisation
	// seam by nature (it's runtime/scheduler state, not an event the
	// component emits). Both exit unconditionally given enough wall-clock, so
	// polling here is guaranteed eventually true by construction.
	var finalGoroutines, leaked int
	require.Eventually(t, func() bool {
		runtime.GC()
		finalGoroutines = runtime.NumGoroutine()
		leaked = finalGoroutines - baselineGoroutines
		return leaked <= 5 // small tolerance for test framework overhead
	}, pollCeiling, pollInterval, "goroutine leak: expected leaked <= 5")

	t.Logf("Baseline goroutines: %d", baselineGoroutines)
	t.Logf("Final goroutines: %d", finalGoroutines)
	t.Logf("Events published: %d", numEvents)
	t.Logf("Events received: %d", received)
	t.Logf("Leaked goroutines: %d", leaked)
}

// TestWorkerPool_HandlesBackpressure verifies the worker pool handles backpressure
func TestWorkerPool_HandlesBackpressure(t *testing.T) {
	// Create EventBus with small buffer
	config := EventBusConfig{
		BufferSize:    10,
		CleanupPeriod: 0, // Disable cleanup for this test
	}
	eb := NewWithConfig[int](config)

	// Create a slow subscriber
	ctx := context.Background()
	ch, _ := eb.Subscribe(ctx)
	// Don't use cleanup in this test - let Shutdown handle it
	defer eb.Shutdown()

	// Track dropped events
	var published atomic.Int64
	var received atomic.Int64

	// Slow consumer
	go func() {
		for range ch {
			received.Add(1)
			time.Sleep(time.Millisecond) // Simulate slow processing
		}
	}()

	// Publish many events rapidly, synchronising on the publisher goroutine's
	// own completion (a WaitGroup it owns) instead of polling a counter.
	var publishWg sync.WaitGroup
	publishWg.Add(1)
	go func() {
		defer publishWg.Done()
		for i := range 1000 {
			eb.PublishAsync(i)
			published.Add(1)
		}
	}()
	publishWg.Wait()

	// Drain blocks until every queued event has run through bus.Publish, so
	// the subscriber-buffer-full decision for every one of them is final by
	// the time it returns. With a 10-slot buffer against 1000 events and a
	// consumer that only drains one event per millisecond, overflow is
	// guaranteed by construction (workers dispatch far faster than the
	// consumer can keep the buffer clear), so the drop count check below
	// needs no polling.
	eb.Drain()

	require.Greater(t, eb.Stats().TotalDropped, uint64(0), "expected some events to be dropped due to backpressure")

	t.Logf("Published: %d", published.Load())
	t.Logf("Received: %d", received.Load())
}

// TestWorkerPool_PublishAsyncShutdownRace exercises the TOCTOU window that existed
// between PublishAsync's ctx check and its eventChan send when Shutdown closed the
// channel concurrently. The fix removes close(eventChan) from Shutdown; workers
// exit via ctx cancellation so the close was never necessary.
// Run with -race to verify no data race or send-on-closed-channel panic.
func TestWorkerPool_PublishAsyncShutdownRace(t *testing.T) {
	t.Parallel()

	const goroutines = 50
	const iterations = 200

	for trial := range 5 {
		_ = trial
		eb := New[int]()

		var wg sync.WaitGroup

		// Hammer PublishAsync from many goroutines.
		for g := range goroutines {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := range iterations {
					eb.PublishAsync(id*iterations + i)
				}
			}(g)
		}

		// Shutdown concurrently with the senders. This used to race and could
		// panic on close(eventChan) being observed by a concurrent sender.
		eb.Shutdown()
		wg.Wait()
	}
}

// TestWorkerPool_QueueDroppedCountsOverflow verifies that events which never
// make it into the worker queue (because it's full) are counted separately
// from subscriber-buffer drops. Workers are set to zero so nothing ever
// drains eventChan, giving a deterministic fill point instead of racing a
// slow consumer against the publisher.
func TestWorkerPool_QueueDroppedCountsOverflow(t *testing.T) {
	eb := New[int]()
	defer eb.Shutdown()

	const bufferSize = 5
	wp := NewWorkerPool(eb, 0, bufferSize)
	defer wp.cancel() // no workers were started, so Shutdown()'s wg.Wait() isn't needed

	const overflow = 7
	for i := range bufferSize + overflow {
		wp.PublishAsync(i)
	}

	require.EqualValues(t, overflow, wp.Dropped(), "expected exactly the overflow beyond the buffer to be dropped")
}

// TestWorkerPool_QueueDroppedZeroOnNormalFlow verifies that as long as the
// queue never fills, PublishAsync leaves QueueDropped at zero.
func TestWorkerPool_QueueDroppedZeroOnNormalFlow(t *testing.T) {
	config := EventBusConfig{
		BufferSize:    100,
		CleanupPeriod: 0,
	}
	eb := NewWithConfig[int](config)
	defer eb.Shutdown()

	ctx := context.Background()
	ch, cleanup := eb.Subscribe(ctx)
	defer cleanup()

	go func() {
		for range ch {
			// drain fast enough that the queue never backs up
		}
	}()

	for i := range 50 {
		eb.PublishAsync(i)
	}

	// Drain blocks until all 50 events have run through bus.Publish, so
	// QueueDropped is final by the time it returns - no polling needed.
	eb.Drain()

	require.EqualValues(t, 0, eb.Stats().QueueDropped, "queue drops should stay at zero when the queue never fills")
}

// TestWorkerPool_QueueDroppedMonotonic hammers a saturated queue from many
// concurrent publishers and checks the counter only ever climbs, never dips,
// confirming the atomic increment is race-free under -race.
func TestWorkerPool_QueueDroppedMonotonic(t *testing.T) {
	eb := New[int]()
	defer eb.Shutdown()

	const bufferSize = 4
	wp := NewWorkerPool(eb, 0, bufferSize) // no workers, so the buffer saturates immediately
	defer wp.cancel()

	const goroutines = 20
	const perGoroutine = 100

	var wg sync.WaitGroup
	var lastSeen atomic.Uint64
	var monotonic atomic.Bool
	monotonic.Store(true)

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				cur := wp.Dropped()
				if cur < lastSeen.Load() {
					monotonic.Store(false)
				}
				lastSeen.Store(cur)
			}
		}
	}()

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perGoroutine {
				wp.PublishAsync(i)
			}
		}()
	}
	wg.Wait()
	close(stop)

	require.True(t, monotonic.Load(), "QueueDropped must never decrease")
	require.EqualValues(t, goroutines*perGoroutine-bufferSize, wp.Dropped(),
		"expected all sends beyond the buffer capacity to be counted as dropped")
}

// TestWorkerPool_ConcurrentPublishing verifies concurrent publishing works
// correctly. numPublishers*eventsPerPublisher is deliberately sized to equal
// the bus's default subscriber buffer (100), so every event fits in the
// subscriber's channel even if nothing ever reads it - delivery is
// guaranteed by construction, not by outracing a slow consumer, which is
// what let this drop the old per-event sleeps and the 80%-floor poll.
func TestWorkerPool_ConcurrentPublishing(t *testing.T) {
	eb := New[string]()

	ctx := context.Background()
	ch, cleanup := eb.Subscribe(ctx)
	defer cleanup()
	defer eb.Shutdown() // Shutdown AFTER cleanup to avoid race

	// Track published vs received
	var published atomic.Int64
	var receivedCount atomic.Int64

	const numPublishers = 5
	const eventsPerPublisher = 20
	const totalEvents = numPublishers * eventsPerPublisher // == default BufferSize (100)

	// Start receiver first
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-ch:
				receivedCount.Add(1)
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for p := range numPublishers {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for i := range eventsPerPublisher {
				event := string(rune('A'+publisherID)) + string(rune('0'+i))
				eb.PublishAsync(event)
				published.Add(1)
			}
		}(p)
	}

	// Wait for all publishers to finish issuing PublishAsync calls.
	wg.Wait()

	// Drain blocks until every one of those events has run through
	// bus.Publish, so the delivery decision for all of them is final - no
	// polling for the receive count to catch up.
	eb.Drain()

	// Stop the background receiver, then flush anything it hadn't yet pulled
	// off the channel: nothing writes to ch after Drain returns, so this is a
	// safe, deterministic, non-blocking sweep rather than a race against the
	// receiver's own scheduling.
	close(done)
	<-stopped
flush:
	for {
		select {
		case <-ch:
			receivedCount.Add(1)
		default:
			break flush
		}
	}

	publishedTotal := published.Load()
	receivedTotal := receivedCount.Load()

	t.Logf("Published: %d", publishedTotal)
	t.Logf("Received: %d events", receivedTotal)

	require.EqualValues(t, totalEvents, publishedTotal, "expected to publish exactly the configured event count")
	// Buffer capacity (100) equals totalEvents exactly, so every event must
	// have been delivered - no drops possible by construction.
	require.EqualValues(t, totalEvents, receivedTotal, "expected every event to be delivered - buffer capacity equals the event count")
	require.EqualValues(t, 0, eb.Stats().TotalDropped, "no drops possible when total events fit the subscriber buffer exactly")
	require.EqualValues(t, 0, eb.Stats().QueueDropped, "no drops possible when total events fit the worker queue (1000) with room to spare")
}
