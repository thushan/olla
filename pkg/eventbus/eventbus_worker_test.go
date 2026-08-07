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

	// Shutdown EventBus
	eb.Shutdown()

	// Poll for worker goroutines to actually exit instead of trusting a fixed
	// sleep to outlast scheduler delays on a loaded runner.
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

	// Publish many events rapidly
	go func() {
		for i := range 1000 {
			eb.PublishAsync(i)
			published.Add(1)
		}
	}()

	// Slow consumer
	go func() {
		for range ch {
			received.Add(1)
			time.Sleep(time.Millisecond) // Simulate slow processing
		}
	}()

	// Wait for every publish call to be issued, rather than hoping a fixed
	// sleep is long enough on a loaded runner.
	require.Eventually(t, func() bool {
		return published.Load() == 1000
	}, pollCeiling, pollInterval, "not all events were published")

	// The tiny buffer (10) against a consumer sleeping 1ms/event guarantees
	// backpressure drops well before the flood finishes - poll the drop
	// counter directly instead of inferring it from received vs published
	// counts after an arbitrary wait.
	require.Eventually(t, func() bool {
		return eb.Stats().TotalDropped > 0
	}, pollCeiling, pollInterval, "expected some events to be dropped due to backpressure")

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

// TestWorkerPool_ConcurrentPublishing verifies concurrent publishing works correctly
func TestWorkerPool_ConcurrentPublishing(t *testing.T) {
	eb := New[string]()

	ctx := context.Background()
	ch, cleanup := eb.Subscribe(ctx)
	defer cleanup()
	defer eb.Shutdown() // Shutdown AFTER cleanup to avoid race

	// Track published vs received
	var published atomic.Int64
	var receivedCount atomic.Int64

	// Use smaller numbers for more reliable test
	const numPublishers = 5
	const eventsPerPublisher = 20

	// Start receiver first
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				receivedCount.Add(1)
			case <-done:
				return
			}
		}
	}()

	// Give receiver time to start
	time.Sleep(10 * time.Millisecond)

	// Publish events with small delays to ensure delivery
	var wg sync.WaitGroup
	for p := range numPublishers {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for i := range eventsPerPublisher {
				event := string(rune('A'+publisherID)) + string(rune('0'+i))
				eb.PublishAsync(event)
				published.Add(1)
				// Small delay to prevent overwhelming the buffer
				time.Sleep(time.Millisecond)
			}
		}(p)
	}

	// Wait for all publishers to finish
	wg.Wait()

	// With smaller numbers and delays, we should receive most events.
	// Poll for delivery to catch up instead of trusting a fixed sleep to
	// outlast processing on a loaded runner - allow for some drops but
	// expect at least 80% delivery.
	minExpected := int64(float64(numPublishers*eventsPerPublisher) * 0.8)
	require.Eventually(t, func() bool {
		return receivedCount.Load() >= minExpected
	}, pollCeiling, pollInterval, "expected at least %d events to be delivered", minExpected)

	// Stop receiver
	close(done)

	publishedTotal := published.Load()
	receivedTotal := receivedCount.Load()

	t.Logf("Published: %d", publishedTotal)
	t.Logf("Received: %d events", receivedTotal)

	// Ensure we actually published what we expected
	if publishedTotal != int64(numPublishers*eventsPerPublisher) {
		t.Errorf("Expected to publish %d events, but published %d", numPublishers*eventsPerPublisher, publishedTotal)
	}
}
