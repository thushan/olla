package eventbus

/*
 * EventBus - A high-performance, lock-free pub/sub-system for Go
 * Copyright (c) 2016-2025 Thushan Fernando, Jason Wright and contributors
 *
 * Ported from Scout (2023) and updated to xsync v4 for better performance and safety (2025)
 */
import (
	"context"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

// EventBus provides lock-free pub/sub with automatic cleanup and backpressure handling
type EventBus[T any] struct {
	subscribers   *xsync.Map[string, *subscriber[T]]
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	workerPool    *WorkerPool[T]
	subscriberSeq atomic.Uint64
	bufferSize    int
	cleanupPeriod time.Duration
	isShutdown    atomic.Bool
}

type subscriber[T any] struct {
	ch         chan T
	id         string
	lastActive atomic.Int64
	dropped    atomic.Uint64
	isActive   atomic.Bool
}

// EventBusConfig allows customisation of buffer sizes and cleanup behaviour
type EventBusConfig struct {
	BufferSize      int
	CleanupPeriod   time.Duration
	InactiveTimeout time.Duration
}

var DefaultConfig = EventBusConfig{
	BufferSize:      100,
	CleanupPeriod:   5 * time.Minute,
	InactiveTimeout: 10 * time.Minute,
}

// New creates a new EventBus with default configuration
func New[T any]() *EventBus[T] {
	return NewWithConfig[T](DefaultConfig)
}

// NewWithConfig creates a new EventBus with custom configuration
func NewWithConfig[T any](config EventBusConfig) *EventBus[T] {
	eb := &EventBus[T]{
		subscribers:   xsync.NewMap[string, *subscriber[T]](),
		bufferSize:    config.BufferSize,
		cleanupPeriod: config.CleanupPeriod,
		stopCleanup:   make(chan struct{}),
	}

	// Create worker pool for async publishing (4 workers, 1000 buffer)
	eb.workerPool = NewWorkerPool(eb, 4, 1000)

	if config.CleanupPeriod > 0 {
		eb.cleanupTicker = time.NewTicker(config.CleanupPeriod)
		go eb.cleanupLoop(config.InactiveTimeout)
	}

	return eb
}

// Subscribe returns a channel that receives events and a cleanup function
func (eb *EventBus[T]) Subscribe(ctx context.Context) (<-chan T, func()) {
	if eb.isShutdown.Load() {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	id := eb.generateSubscriberID()
	ch := make(chan T, eb.bufferSize)

	sub := &subscriber[T]{
		id: id,
		ch: ch,
	}
	sub.lastActive.Store(time.Now().UnixNano())
	sub.isActive.Store(true)

	eb.subscribers.Store(id, sub)

	// Context cancellation handler ensures proper cleanup
	go func() {
		<-ctx.Done()
		eb.unsubscribe(id)
	}()

	cleanup := func() {
		eb.unsubscribe(id)
	}

	return ch, cleanup
}

// Publish sends an event to all active subscribers
func (eb *EventBus[T]) Publish(event T) int {
	if eb.isShutdown.Load() {
		return 0
	}

	delivered := 0
	now := time.Now().UnixNano()

	eb.subscribers.Range(func(id string, sub *subscriber[T]) bool {
		if !sub.isActive.Load() {
			return true
		}

		// Double-check active status before sending to avoid race
		if sub.isActive.Load() {
			select {
			case sub.ch <- event:
				sub.lastActive.Store(now)
				delivered++
			default:
				sub.dropped.Add(1)
			}
		}
		return true
	})

	return delivered
}

// PublishAsync sends an event without blocking
func (eb *EventBus[T]) PublishAsync(event T) {
	if eb.isShutdown.Load() {
		return
	}
	if eb.workerPool != nil {
		eb.workerPool.PublishAsync(event)
	}
}

// Shutdown gracefully stops the event bus
func (eb *EventBus[T]) Shutdown() {
	if !eb.isShutdown.CompareAndSwap(false, true) {
		return
	}

	// Shutdown worker pool first
	if eb.workerPool != nil {
		eb.workerPool.Shutdown()
	}

	if eb.cleanupTicker != nil {
		eb.cleanupTicker.Stop()
		close(eb.stopCleanup)
	}

	// Mark all subscribers as inactive first
	eb.subscribers.Range(func(id string, sub *subscriber[T]) bool {
		sub.isActive.Store(false)
		return true
	})

	// Clear subscribers map - channels will be GC'd when no longer referenced
	// We don't close channels to avoid send-on-closed-channel panics
	eb.subscribers.Clear()
}

// Stats returns overall event bus statistics
func (eb *EventBus[T]) Stats() EventBusStats {
	stats := EventBusStats{
		IsShutdown: eb.isShutdown.Load(),
	}
	if stats.IsShutdown {
		return stats
	}

	eb.subscribers.Range(func(id string, sub *subscriber[T]) bool {
		stats.TotalSubscribers++
		if sub.isActive.Load() {
			stats.ActiveSubscribers++
		}
		stats.TotalDropped += sub.dropped.Load()
		return true
	})

	return stats
}

// EventBusStats provides aggregate metrics
type EventBusStats struct {
	TotalSubscribers  int
	ActiveSubscribers int
	TotalDropped      uint64
	IsShutdown        bool
}

// generateSubscriberID creates a unique subscriber ID
func (eb *EventBus[T]) generateSubscriberID() string {
	seq := eb.subscriberSeq.Add(1)
	return "sub_" + strconv.FormatUint(seq, 10)
}

// unsubscribe removes a subscriber safely
func (eb *EventBus[T]) unsubscribe(id string) {
	if sub, exists := eb.subscribers.Load(id); exists {
		// Mark as inactive first to prevent new sends
		sub.isActive.Store(false)
		// Remove from map so no new operations can find it
		eb.subscribers.Delete(id)
		// Don't close the channel - let GC handle it when no references remain
		// This prevents panic from concurrent sends
	}
}

// cleanupLoop removes inactive subscribers every so often
func (eb *EventBus[T]) cleanupLoop(inactiveTimeout time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("eventbus cleanupLoop panic recovered: %v", r)
		}
	}()

	for {
		select {
		case <-eb.stopCleanup:
			return
		case <-eb.cleanupTicker.C:
			eb.cleanupInactiveSubscribers(inactiveTimeout)
		}
	}
}

// cleanupInactiveSubscribers purges stale entries
func (eb *EventBus[T]) cleanupInactiveSubscribers(timeout time.Duration) {
	cutoff := time.Now().Add(-timeout).UnixNano()
	var toRemove []string

	eb.subscribers.Range(func(id string, sub *subscriber[T]) bool {
		if !sub.isActive.Load() || sub.lastActive.Load() < cutoff {
			toRemove = append(toRemove, id)
		}
		return true
	})

	for _, id := range toRemove {
		eb.unsubscribe(id)
	}
}
