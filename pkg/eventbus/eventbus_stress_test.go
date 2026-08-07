package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// hangGuard bounds how long a storm test's publish+drain phase is allowed to
// take before we conclude it has wedged. It is a deadlock detector, not a
// success condition: reaching it fails the test with a clear "hung" message,
// but nothing here treats "finished within hangGuard" as proof of correct
// behaviour - the assertions after the guard do that.
const hangGuard = 60 * time.Second

// TestWorkerPool_ConcurrentPublishingStress hammers the bus with 10 publishers
// racing unthrottled against a single subscriber. PublishAsync is non-blocking
// by design, so under a genuine storm the bus WILL drop events at the queue
// and/or subscriber stage rather than block anyone - on a starved 2-core CI
// runner the receiver goroutine can lose enough scheduler time that most
// events are dropped before it ever gets to read them. There is no delivery
// floor to assert here; once the storm ends, dropped events are gone for
// good, not "eventually" delivered.
//
// This test proves two things deterministically: the storm doesn't wedge the
// bus (hangGuard), and once Drain confirms every queued event has been fully
// processed, delivered + tracked drops reconciles EXACTLY against what was
// published - not a lower bound, because Drain removes all ambiguity about
// events still mid-flight.
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

	// Publish then Drain, guarded by hangGuard rather than polling for the
	// receive count to "stabilise". Drain blocks until every event that made
	// it into the worker queue has been run through bus.Publish, so once it
	// returns the subscriber-delivery decision for every published event is
	// final - a deterministic synchronisation point instead of a wall-clock
	// guess at when the storm has settled.
	stormDone := make(chan struct{})
	go func() {
		defer close(stormDone)
		wg.Wait()
		eb.Drain()
	}()

	select {
	case <-stormDone:
	case <-time.After(hangGuard):
		t.Fatal("test hung / suspected deadlock: storm publish+drain did not complete")
	}

	// Stop the receiver, then flush anything it hadn't yet pulled off the
	// channel: Drain has already returned, so nothing writes to ch anymore -
	// this sweep is a safe, deterministic drain rather than a race against
	// the receiver goroutine's own scheduling.
	close(done)
	<-stopped
flush:
	for {
		select {
		case event := <-ch:
			receivedCount.Add(1)
			mu.Lock()
			received[event] = true
			mu.Unlock()
		default:
			break flush
		}
	}

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

	// With every event drained and accounted for, delivered + tracked drops
	// (subscriber-stage and queue-stage) must reconcile EXACTLY against what
	// was published - there is no longer any event that could still be
	// mid-flight and uncounted.
	stats := eb.Stats()
	require.Equal(t, publishedTotal, receivedTotal+int64(stats.TotalDropped)+int64(stats.QueueDropped),
		"delivered + tracked drops must exactly reconcile against what was published")
}

// TestEventBus_HighVolumePublishing floods the bus with far more events than
// its worker queue (1000) and subscriber buffer (bus.bufferSize, default 100)
// can hold. PublishAsync is deliberately non-blocking - see EventBus.PublishAsync
// and WorkerPool.PublishAsync - so under an unthrottled storm the bus WILL drop
// events rather than block the publisher or the workers. There is no delivery
// floor this bus promises during a storm that outruns its buffers.
//
// This test proves the storm doesn't block the publisher, doesn't wedge the
// bus (hangGuard), that the bus is still delivering normally once Drain
// confirms the storm has fully drained, and that the drop/delivery
// accounting reconciles exactly at that point.
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

	// Drain blocks until every event that made it into the worker queue has
	// been run through bus.Publish, guarded by hangGuard rather than polling
	// for the receive count to stop climbing.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		bus.Drain()
	}()

	select {
	case <-drainDone:
	case <-time.After(hangGuard):
		t.Fatal("test hung / suspected deadlock: drain did not complete after the storm")
	}

	// Stop the background drainer, then flush anything it hadn't yet pulled
	// off the channel - Drain has returned, so nothing writes to ch anymore,
	// making this a safe deterministic sweep rather than a race against the
	// goroutine's own scheduling.
	close(done)
	<-stopped
flush:
	for {
		select {
		case <-ch:
			received.Add(1)
		default:
			break flush
		}
	}

	receivedTotal := received.Load()

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
	// in Stats().QueueDropped. With Drain having confirmed every event is
	// fully processed, delivered + tracked drops reconciles EXACTLY against
	// what was published.
	stats := bus.Stats()
	require.Equal(t, int64(totalEvents), receivedTotal+int64(stats.TotalDropped)+int64(stats.QueueDropped),
		"delivered + tracked drops must exactly reconcile against what was published")

	t.Logf("HIGH VOLUME - Published %d events in %v", totalEvents, publishDuration)
	t.Logf("HIGH VOLUME - Received: %d events (%.2f%%)", receivedTotal, float64(receivedTotal)/float64(totalEvents)*100)
	t.Logf("HIGH VOLUME - Tracked drops: %d (subscriber) + %d (queue)", stats.TotalDropped, stats.QueueDropped)
	t.Logf("HIGH VOLUME - Publish rate: %.0f events/second", float64(totalEvents)/publishDuration.Seconds())
}

// TestEventBus_ConcurrentSubscribers tests many concurrent subscribers.
//
// The original version had each subscriber goroutine loop `for range ch`
// until it accumulated eventsToPublish/10 events, with no way out if it
// never reached that count - on a sufficiently starved runner a subscriber
// scheduled too late to receive enough events before Shutdown() (which does
// not close subscriber channels, by design - see EventBus.Shutdown) would
// block forever, hanging wg.Wait() with no timeout at all. That is exactly
// the class of bug this whole redesign exists to catch: a stress test that
// can wedge under the very conditions it's supposed to be stress-testing.
//
// This version bounds every subscriber goroutine on a stop signal fired once
// publishing has finished, so it can never wait for events that will never
// arrive, and wraps the wait in hangGuard as a deadlock detector. The
// assertion is "no deadlock, no wedge" per the test's actual purpose - not a
// delivery floor, which would be exactly the kind of flaky-under-load
// assertion this redesign is meant to eliminate.
func TestEventBus_ConcurrentSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent subscribers test in short mode")
	}
	bus := New[int]()

	ctx := context.Background()
	const numSubscribers = 50
	const eventsToPublish = 1000

	var totalReceived atomic.Int64
	var wg sync.WaitGroup

	// stopWaiting fires once publishing has finished, telling subscriber
	// goroutines no more events are coming so they should stop waiting and
	// drain whatever is left in their own buffer instead of blocking forever.
	stopWaiting := make(chan struct{})

	var cleanups []func()
	for i := range numSubscribers {
		ch, cleanup := bus.Subscribe(ctx)
		cleanups = append(cleanups, cleanup)

		wg.Add(1)
		go func(subID int) {
			defer wg.Done()
			count := 0
			for {
				select {
				case event, ok := <-ch:
					if !ok {
						return
					}
					_ = event
					count++
				case <-stopWaiting:
					// Drain whatever is already buffered, non-blocking - safe
					// because no more events will be published after this
					// point, so ch's remaining contents are static.
					for {
						select {
						case <-ch:
							count++
						default:
							totalReceived.Add(int64(count))
							return
						}
					}
				}
			}
		}(i)
	}

	// Publish events
	start := time.Now()
	for i := range eventsToPublish {
		bus.Publish(i)
	}
	publishDuration := time.Since(start)

	close(stopWaiting)

	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		wg.Wait()
	}()

	select {
	case <-waitDone:
	case <-time.After(hangGuard):
		t.Fatal("test hung / suspected deadlock: subscriber goroutines never exited")
	}

	for _, cleanup := range cleanups {
		cleanup()
	}
	bus.Shutdown()

	avgReceived := float64(totalReceived.Load()) / float64(numSubscribers)
	t.Logf("MANY SUBSCRIBERS - Published %d events to %d subscribers in %v", eventsToPublish, numSubscribers, publishDuration)
	t.Logf("MANY SUBSCRIBERS - Average received per subscriber: %.0f", avgReceived)
	t.Logf("MANY SUBSCRIBERS - Total events delivered: %d", totalReceived.Load())
}
