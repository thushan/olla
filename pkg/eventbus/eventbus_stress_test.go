package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for i := 0; i < eventsPerPublisher; i++ {
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

// TestEventBus_HighVolumePublishing tests with very high volume
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
	go func() {
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

	for i := 0; i < totalEvents; i++ {
		bus.PublishAsync(i)
	}

	publishDuration := time.Since(start)

	// Wait for processing
	time.Sleep(2 * time.Second)
	close(done)

	receivedTotal := received.Load()
	t.Logf("HIGH VOLUME - Published %d events in %v", totalEvents, publishDuration)
	t.Logf("HIGH VOLUME - Received: %d events (%.2f%%)", receivedTotal, float64(receivedTotal)/float64(totalEvents)*100)
	t.Logf("HIGH VOLUME - Publish rate: %.0f events/second", float64(totalEvents)/publishDuration.Seconds())

	// We expect significant drops with this volume, but should still process many
	if receivedTotal < 1000 {
		t.Errorf("Expected to receive at least 1000 events out of %d, got %d", totalEvents, receivedTotal)
	}
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

	for i := 0; i < numSubscribers; i++ {
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
	for i := 0; i < eventsToPublish; i++ {
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
