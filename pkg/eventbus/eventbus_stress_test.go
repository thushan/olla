package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWorkerPool_ConcurrentPublishingStress runs comprehensive stress tests
// This test is skipped in CI (when -short flag is used)
func TestWorkerPool_ConcurrentPublishingStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}
	eb := New[string]()

	ctx := context.Background()
	ch, cleanup := eb.Subscribe(ctx)
	defer cleanup()
	defer eb.Shutdown()

	// Track published vs received
	var published atomic.Int64
	var receivedCount atomic.Int64
	received := make(map[string]bool)
	var mu sync.Mutex

	// Original scale for stress testing
	const numPublishers = 10
	const eventsPerPublisher = 100

	// Start receiver
	done := make(chan struct{})
	go func() {
		for {
			select {
			case event := <-ch:
				receivedCount.Add(1)
				mu.Lock()
				received[event] = true
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	// Publish events rapidly (no delays - maximum stress)
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

	// Wait for all publishers to finish
	wg.Wait()

	// Give time for events to be processed
	time.Sleep(500 * time.Millisecond)

	// Stop receiver
	close(done)

	// Wait a bit for receiver to finish
	time.Sleep(100 * time.Millisecond)

	publishedTotal := published.Load()
	receivedTotal := receivedCount.Load()

	// Safely read the map length
	mu.Lock()
	uniqueEvents := len(received)
	mu.Unlock()

	t.Logf("STRESS TEST - Published: %d", publishedTotal)
	t.Logf("STRESS TEST - Received: %d events", receivedTotal)
	t.Logf("STRESS TEST - Unique events: %d", uniqueEvents)

	// With stress test, we expect more drops but still reasonable delivery
	// Lower threshold since we're stress testing without delays
	minExpected := int64(float64(numPublishers*eventsPerPublisher) * 0.3)
	if receivedTotal < minExpected {
		t.Errorf("Expected at least %d events, got %d", minExpected, receivedTotal)
	}
}

// TestEventBus_HighVolumePublishing floods the bus with far more events than
// its worker queue (1000) and subscriber buffer (bus.bufferSize, default 100)
// can hold. PublishAsync is deliberately non-blocking - see EventBus.PublishAsync
// and WorkerPool.PublishAsync - so under an unthrottled storm the bus WILL drop
// events rather than block the publisher or the workers. There is no delivery
// floor this bus promises during a storm that outruns its buffers, so this test
// proves the storm doesn't block the publisher, doesn't wedge the bus, and that
// the bus is still delivering normally once the storm has drained - not that a
// specific count of events survived it.
func TestEventBus_HighVolumePublishing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high volume test in short mode")
	}
	bus := New[int]()
	defer bus.Shutdown()

	ctx := context.Background()
	ch, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Drain events in background
	var received atomic.Int64
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-ch:
				received.Add(1)
			case <-done:
				return
			}
		}
	}()

	// Publish 100,000 events as fast as possible
	const totalEvents = 100000
	start := time.Now()

	for i := range totalEvents {
		bus.PublishAsync(i)
	}

	publishDuration := time.Since(start)

	// The whole point of PublishAsync is that the publisher never blocks on a
	// backed-up bus - prove that held under real load.
	require.Less(t, publishDuration, 5*time.Second, "PublishAsync must not block the publisher under load")

	// The worker queue can hold at most 1000 events in flight, so whatever got
	// queued drains quickly once publishing stops. Wait for the received count
	// to stop climbing rather than guessing a fixed delay.
	require.Eventually(t, func() bool {
		before := received.Load()
		time.Sleep(100 * time.Millisecond)
		return received.Load() == before
	}, 5*time.Second, 10*time.Millisecond, "received count never stabilised after the storm - bus may be wedged")

	receivedTotal := received.Load()

	// Stop the background drainer and wait for it to actually exit before
	// probing directly on ch - otherwise the two readers race for the same
	// delivery and the probe can lose nondeterministically.
	close(done)
	<-stopped

	// Prove the bus survived the storm rather than silently wedging: publish one
	// more event synchronously and confirm the now-sole reader gets it.
	delivered := bus.Publish(-1)
	require.Equal(t, 1, delivered, "bus should still deliver to the live subscriber after the storm")
	select {
	case v := <-ch:
		require.Equal(t, -1, v)
	case <-time.After(time.Second):
		t.Fatal("bus did not deliver a post-storm event - looks wedged")
	}

	// Subscriber-level drops are tracked in Stats().TotalDropped; drops in the
	// worker queue itself (WorkerPool.PublishAsync's "queue full" branch) are
	// not counted, so this is a lower bound, not an exact reconciliation -
	// still enough to catch a corrupted or overflowing counter.
	stats := bus.Stats()
	require.LessOrEqual(t, receivedTotal+int64(stats.TotalDropped), int64(totalEvents)+1,
		"delivered + tracked drops should never exceed what was published")

	t.Logf("HIGH VOLUME - Published %d events in %v", totalEvents, publishDuration)
	t.Logf("HIGH VOLUME - Received: %d events (%.2f%%)", receivedTotal, float64(receivedTotal)/float64(totalEvents)*100)
	t.Logf("HIGH VOLUME - Tracked drops: %d", stats.TotalDropped)
	t.Logf("HIGH VOLUME - Publish rate: %.0f events/second", float64(totalEvents)/publishDuration.Seconds())
}

// TestEventBus_ConcurrentSubscribers tests many concurrent subscribers
func TestEventBus_ConcurrentSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent subscribers test in short mode")
	}
	bus := New[int]()
	defer bus.Shutdown()

	ctx := context.Background()
	const numSubscribers = 50
	const eventsToPublish = 1000

	// Create many subscribers
	var totalReceived atomic.Int64
	var wg sync.WaitGroup

	for i := range numSubscribers {
		ch, cleanup := bus.Subscribe(ctx)
		defer cleanup()

		wg.Add(1)
		go func(subID int) {
			defer wg.Done()
			count := 0
			for range ch {
				count++
				if count >= eventsToPublish/10 { // Exit after receiving some events
					break
				}
			}
			totalReceived.Add(int64(count))
		}(i)
	}

	// Publish events
	start := time.Now()
	for i := range eventsToPublish {
		delivered := bus.Publish(i)
		if delivered < numSubscribers/2 {
			t.Logf("Warning: Only delivered to %d/%d subscribers at event %d", delivered, numSubscribers, i)
		}
	}
	publishDuration := time.Since(start)

	// Signal subscribers to exit by shutting down
	bus.Shutdown()
	wg.Wait()

	avgReceived := float64(totalReceived.Load()) / float64(numSubscribers)
	t.Logf("MANY SUBSCRIBERS - Published %d events to %d subscribers in %v", eventsToPublish, numSubscribers, publishDuration)
	t.Logf("MANY SUBSCRIBERS - Average received per subscriber: %.0f", avgReceived)
	t.Logf("MANY SUBSCRIBERS - Total events delivered: %d", totalReceived.Load())

	if avgReceived < 10 {
		t.Errorf("Expected subscribers to receive more events on average, got %.0f", avgReceived)
	}
}
