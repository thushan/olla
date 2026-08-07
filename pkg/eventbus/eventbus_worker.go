package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of workers for async event publishing
type WorkerPool[T any] struct {
	ctx       context.Context
	eventChan chan T
	bus       *EventBus[T]
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	workers   int
	dropped   atomic.Uint64
}

// NewWorkerPool creates a new worker pool for the event bus
func NewWorkerPool[T any](bus *EventBus[T], workers int, bufferSize int) *WorkerPool[T] {
	ctx, cancel := context.WithCancel(context.Background())
	wp := &WorkerPool[T]{
		eventChan: make(chan T, bufferSize),
		bus:       bus,
		workers:   workers,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start worker goroutines
	for range workers {
		wp.wg.Add(1)
		go wp.worker()
	}

	return wp
}

// PublishAsync queues an event for async publishing
func (wp *WorkerPool[T]) PublishAsync(event T) {
	// Check if we're shutting down
	select {
	case <-wp.ctx.Done():
		// Worker pool is shutting down, drop the event
		return
	default:
	}

	// Try non-blocking send
	select {
	case wp.eventChan <- event:
		// Successfully queued
	default:
		// Queue is full, drop the event to prevent blocking. This is a
		// stage-1 drop - the event never reached a subscriber at all - which
		// is a different failure mode to EventBusStats.TotalDropped (a
		// stage-3 drop, where a subscriber's own buffer was full). Operators
		// need both counters to tell "queue too small / workers too slow to
		// drain" apart from "a subscriber is too slow to consume".
		wp.dropped.Add(1)
	}
}

// Dropped returns the number of events discarded because the worker queue
// was full when PublishAsync was called.
func (wp *WorkerPool[T]) Dropped() uint64 {
	return wp.dropped.Load()
}

// worker processes events from the queue
func (wp *WorkerPool[T]) worker() {
	defer wp.wg.Done()
	for {
		select {
		case event, ok := <-wp.eventChan:
			if !ok {
				return // channel closed
			}
			wp.bus.Publish(event)
		case <-wp.ctx.Done():
			return
		}
	}
}

// Shutdown stops all workers.
func (wp *WorkerPool[T]) Shutdown() {
	// Cancel the context so workers exit their select loops.
	wp.cancel()
	// Wait for all workers to drain and exit before returning.
	wp.wg.Wait()
	// Do NOT close eventChan. Workers exit via ctx cancellation, not via a
	// range-over-channel, so the close is unnecessary. More importantly,
	// PublishAsync checks ctx.Done() and then sends in two separate selects —
	// a goroutine that passes the ctx check before Shutdown runs can still
	// attempt a send on a closed channel, causing a panic. Leaving the channel
	// open is safe: it is GC'd once the WorkerPool itself goes out of scope.
}
