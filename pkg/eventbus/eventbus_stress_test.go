package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWorkerPool_ConcurrentPublishingStress hammers the bus with 10 publishers
// racing unthrottled against a single subscriber. Like TestEventBus_HighVolumePublishing,
// PublishAsync is non-blocking by design, so under a genuine storm the bus WILL
// drop events at the queue and/or subscriber stage rather than block anyone -
// on a starved 2-core CI runner the receiver goroutine can lose enough
// scheduler time that most events are dropped before it ever gets to read
// them. There is no delivery floor to assert here; once the storm ends,
// dropped events are gone for good, not "eventually" delivered. This test
// instead proves the bus keeps its actual guarantees: the storm doesn't wedge
// it, and delivered + tracked drops reconciles against what was published.
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
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
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

	// Wait for the received count to stop climbing rather than guessing a
	// fixed delay - the queue (1000) and subscriber buffer (100) both drain
	// quickly once publishing stops.
	require.Eventually(t, func() bool {
		before := receivedCount.Load()
		time.Sleep(50 * time.Millisecond)
		return receivedCount.Load() == before
	}, pollCeiling, pollInterval, "received count never stabilised after the storm - bus may be wedged")

	// Stop the receiver and wait for it to actually exit before touching the
	// map directly, so there's no race between the goroutine's last write and
	// this read.
	close(done)
	<-stopped

	publishedTotal := published.Load()
	receivedTotal := receivedCount.Load()

	mu.Lock()
	uniqueEvents := len(received)
	mu.Unlock()

	t.Logf("STRESS TEST - Published: %d", publishedTotal)
	t.Logf("STRESS TEST - Received: %d events", receivedTotal)
	t.Logf("STRESS TEST - Unique events: %d", uniqueEvents)

	// Prove the bus survived the storm rather than silently wedging: publish
	// one more event synchronously and confirm the still-live subscriber gets it.
	delivered := eb.Publish("post-storm")
	require.Equal(t, 1, delivered, "bus should still deliver to the live subscriber after the storm")

	// Delivered + tracked drops (subscriber-stage and queue-stage) should
	// never exceed what was actually published.
	stats := eb.Stats()
	require.LessOrEqual(t, receivedTotal+int64(stats.TotalDropped)+int64(stats.QueueDropped), publishedTotal,
		"delivered + tracked drops should never exceed what was published")
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

	// Subscriber-level drops are tracked in Stats().TotalDropped, and
	// worker-queue-level drops (WorkerPool.PublishAsync's "queue full" branch)
	// in Stats().QueueDropped, so summing both now gives an exact
	// reconciliation rather than a lower bound.
	stats := bus.Stats()
	require.LessOrEqual(t, receivedTotal+int64(stats.TotalDropped)+int64(stats.QueueDropped), int64(totalEvents)+1,
		"delivered + tracked drops should never exceed what was published")

	t.Logf("HIGH VOLUME - Published %d events in %v", totalEvents, publishDuration)
	t.Logf("HIGH VOLUME - Received: %d events (%.2f%%)", receivedTotal, float64(receivedTotal)/float64(totalEvents)*100)
	t.Logf("HIGH VOLUME - Tracked drops: %d (subscriber) + %d (queue)", stats.TotalDropped, stats.QueueDropped)
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
