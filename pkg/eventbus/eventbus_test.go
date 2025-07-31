package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type TestEvent struct {
	ID      int
	Message string
}

type HealthEvent struct {
	EndpointName string
	Status       string
	Timestamp    time.Time
}

func TestEventBus_BasicPubSub(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	testEvent := TestEvent{ID: 1, Message: "test"}
	delivered := bus.Publish(testEvent)

	if delivered != 1 {
		t.Errorf("Expected 1 delivery, got %d", delivered)
	}

	select {
	case received := <-events:
		if received.ID != testEvent.ID || received.Message != testEvent.Message {
			t.Errorf("Event mismatch: expected %+v, got %+v", testEvent, received)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()
	numSubscribers := 5
	var subscribers []<-chan TestEvent
	var cleanups []func()

	for i := 0; i < numSubscribers; i++ {
		events, cleanup := bus.Subscribe(ctx)
		subscribers = append(subscribers, events)
		cleanups = append(cleanups, cleanup)
	}
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	testEvent := TestEvent{ID: 42, Message: "broadcast"}
	delivered := bus.Publish(testEvent)

	if delivered != numSubscribers {
		t.Errorf("Expected %d deliveries, got %d", numSubscribers, delivered)
	}

	for i, events := range subscribers {
		select {
		case received := <-events:
			if received.ID != testEvent.ID {
				t.Errorf("Subscriber %d: expected ID %d, got %d", i, testEvent.ID, received.ID)
			}
		case <-time.After(time.Second):
			t.Errorf("Subscriber %d: timeout waiting for event", i)
		}
	}
}

func TestEventBus_ContextCancellation(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	cancel()

	// Wait a bit to ensure unsubscribe has processed
	time.Sleep(50 * time.Millisecond)

	// Verify no more events are received (channel not closed to prevent panics)
	select {
	case event := <-events:
		t.Errorf("Should not receive events after context cancellation, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no events after unsubscribe
	}
}

func TestEventBus_BackpressureHandling(t *testing.T) {
	config := EventBusConfig{
		BufferSize:      2,         // Small buffer to test backpressure
		CleanupPeriod:   time.Hour, // Disable cleanup for this test
		InactiveTimeout: time.Hour,
	}
	bus := NewWithConfig[TestEvent](config)
	defer bus.Shutdown()

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Fill the buffer completely
	for i := 0; i < 2; i++ {
		delivered := bus.Publish(TestEvent{ID: i, Message: "fill buffer"})
		if delivered != 1 {
			t.Errorf("Event %d: expected 1 delivery, got %d", i, delivered)
		}
	}

	// This should be dropped due to full buffer
	delivered := bus.Publish(TestEvent{ID: 999, Message: "should be dropped"})
	if delivered != 0 {
		t.Errorf("Expected 0 deliveries (buffer full), got %d", delivered)
	}

	stats := bus.Stats()
	if stats.TotalDropped == 0 {
		t.Error("Expected at least 1 dropped message")
	}

	// Drain buffer to verify first events made it through
	for i := 0; i < 2; i++ {
		select {
		case event := <-events:
			if event.ID != i {
				t.Errorf("Expected event ID %d, got %d", i, event.ID)
			}
		case <-time.After(time.Second):
			t.Errorf("Timeout waiting for buffered event %d", i)
		}
	}
}
func TestEventBus_ConcurrentPublishSubscribe(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	const numPublishers = 10
	const numSubscribers = 5
	const eventsPerPublisher = 100

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var subscribers []<-chan TestEvent
	var cleanups []func()
	receivedCounts := make([]int64, numSubscribers)
	var subscriberWg sync.WaitGroup

	for i := 0; i < numSubscribers; i++ {
		events, cleanup := bus.Subscribe(ctx)
		subscribers = append(subscribers, events)
		cleanups = append(cleanups, cleanup)

		idx := i
		subscriberWg.Add(1)
		go func() {
			defer subscriberWg.Done()
			for {
				select {
				case <-events:
					atomic.AddInt64(&receivedCounts[idx], 1)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	var wg sync.WaitGroup
	totalPublished := int64(0)

	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				event := TestEvent{
					ID:      publisherID*1000 + j,
					Message: "concurrent test",
				}
				bus.Publish(event)
				atomic.AddInt64(&totalPublished, 1)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let subscribers process remaining events
	cancel()                           // Stop subscribers
	subscriberWg.Wait()

	expectedTotal := int64(numPublishers * eventsPerPublisher)
	if totalPublished != expectedTotal {
		t.Errorf("Expected %d total published, got %d", expectedTotal, totalPublished)
	}

	for i, count := range receivedCounts {
		if count == 0 {
			t.Errorf("Subscriber %d received no events", i)
		}
	}
}

func TestEventBus_PublishAsync(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	testEvent := TestEvent{ID: 1, Message: "async test"}

	// PublishAsync should not block
	start := time.Now()
	bus.PublishAsync(testEvent)
	duration := time.Since(start)

	if duration > 10*time.Millisecond {
		t.Errorf("PublishAsync took too long: %v", duration)
	}

	// Should still receive the event
	select {
	case received := <-events:
		if received.ID != testEvent.ID {
			t.Errorf("Event mismatch: expected %+v, got %+v", testEvent, received)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for async event")
	}
}

func TestEventBus_Shutdown(t *testing.T) {
	bus := New[TestEvent]()

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Drain any buffered events first
	select {
	case <-events:
		// Drain the buffered event
	case <-time.After(100 * time.Millisecond):
		// No event to drain, that's fine
	}

	// Shutdown the bus
	bus.Shutdown()

	// Verify shutdown state
	stats := bus.Stats()
	if !stats.IsShutdown {
		t.Error("Bus should report as shutdown")
	}

	// Attempts to publish should fail
	delivered := bus.Publish(TestEvent{ID: 2, Message: "after shutdown"})
	if delivered != 0 {
		t.Errorf("Expected 0 deliveries after shutdown, got %d", delivered)
	}

	// Subscribe to shutdown bus should return closed channel
	newEvents, newCleanup := bus.Subscribe(ctx)
	defer newCleanup()

	select {
	case _, ok := <-newEvents:
		if ok {
			t.Error("Channel from shutdown bus should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected closed channel immediately")
	}

	// Original subscriber channel should stop receiving events but won't be closed
	// (channels are left to GC to prevent send-on-closed-channel panics)
	select {
	case event := <-events:
		t.Errorf("Should not receive events after shutdown, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no more events after shutdown
	}
}

func TestEventBus_Stats(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	// Initial stats
	stats := bus.Stats()
	if stats.TotalSubscribers != 0 || stats.ActiveSubscribers != 0 {
		t.Errorf("Expected empty bus stats, got %+v", stats)
	}

	ctx := context.Background()

	// Add subscribers
	_, cleanup1 := bus.Subscribe(ctx)
	_, cleanup2 := bus.Subscribe(ctx)
	defer cleanup1()
	defer cleanup2()

	stats = bus.Stats()
	if stats.TotalSubscribers != 2 || stats.ActiveSubscribers != 2 {
		t.Errorf("Expected 2 subscribers, got %+v", stats)
	}

	// Remove one subscriber
	cleanup1()
	time.Sleep(10 * time.Millisecond) // Give time for cleanup

	stats = bus.Stats()
	if stats.TotalSubscribers != 1 || stats.ActiveSubscribers != 1 {
		t.Errorf("Expected 1 subscriber after cleanup, got %+v", stats)
	}
}

func TestEventBus_CleanupInactiveSubscribers(t *testing.T) {
	config := EventBusConfig{
		BufferSize:      10,
		CleanupPeriod:   50 * time.Millisecond,
		InactiveTimeout: 100 * time.Millisecond,
	}
	bus := NewWithConfig[TestEvent](config)
	defer bus.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Verify subscriber exists
	stats := bus.Stats()
	if stats.TotalSubscribers != 1 {
		t.Errorf("Expected 1 subscriber, got %d", stats.TotalSubscribers)
	}

	// Cancel context to make subscriber inactive
	cancel()

	// Wait for cleanup cycle
	time.Sleep(200 * time.Millisecond)

	// Subscriber should be cleaned up
	stats = bus.Stats()
	if stats.TotalSubscribers != 0 {
		t.Errorf("Expected subscriber to be cleaned up, got %d subscribers", stats.TotalSubscribers)
	}

	// Channel won't be closed (to prevent panics), but should not receive events
	select {
	case event := <-events:
		t.Errorf("Should not receive events after cleanup, got: %+v", event)
	default:
		// Expected - no events available
	}
}

// Benchmark tests
func BenchmarkEventBus_Publish(b *testing.B) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()
	_, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	event := TestEvent{ID: 1, Message: "benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(event)
	}
}

func BenchmarkEventBus_PublishMultipleSubscribers(b *testing.B) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()

	// Create 10 subscribers
	var cleanups []func()
	for i := 0; i < 10; i++ {
		_, cleanup := bus.Subscribe(ctx)
		cleanups = append(cleanups, cleanup)
	}
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	event := TestEvent{ID: 1, Message: "benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(event)
	}
}

func BenchmarkEventBus_ConcurrentPublish(b *testing.B) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()
	_, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	event := TestEvent{ID: 1, Message: "concurrent benchmark"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bus.Publish(event)
		}
	})
}

func TestEventBus_PublishAsyncOrderBasic(t *testing.T) {
	bus := New[TestEvent]()
	defer bus.Shutdown()

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Send a series of async events
	for i := 0; i < 5; i++ {
		bus.PublishAsync(TestEvent{ID: i})
	}

	received := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		select {
		case ev := <-events:
			received = append(received, ev.ID)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("Timeout waiting for async event %d", i)
		}
	}

	if len(received) != 5 {
		t.Errorf("Expected 5 async events, got %d", len(received))
	}
}

func TestEventBus_TypeSafety(t *testing.T) {
	// Test that different event types work correctly
	// modified to use Sherpa/Olla events
	healthBus := New[HealthEvent]()
	testBus := New[TestEvent]()
	defer healthBus.Shutdown()
	defer testBus.Shutdown()

	ctx := context.Background()

	healthEvents, healthCleanup := healthBus.Subscribe(ctx)
	testEvents, testCleanup := testBus.Subscribe(ctx)
	defer healthCleanup()
	defer testCleanup()

	// Publish different event types
	healthEvent := HealthEvent{
		EndpointName: "test-endpoint",
		Status:       "healthy",
		Timestamp:    time.Now(),
	}
	testEvent := TestEvent{ID: 1, Message: "test"}

	healthBus.Publish(healthEvent)
	testBus.Publish(testEvent)

	// Verify type safety
	select {
	case received := <-healthEvents:
		if received.EndpointName != healthEvent.EndpointName {
			t.Errorf("Health event mismatch: expected %s, got %s",
				healthEvent.EndpointName, received.EndpointName)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for health event")
	}

	select {
	case received := <-testEvents:
		if received.ID != testEvent.ID {
			t.Errorf("Test event mismatch: expected %d, got %d",
				testEvent.ID, received.ID)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for test event")
	}
}
