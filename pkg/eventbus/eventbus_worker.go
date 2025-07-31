package eventbus

import (
	"context"
	"sync"
)

// WorkerPool manages a pool of workers for async event publishing
type WorkerPool[T any] struct {
	ctx       context.Context
	eventChan chan T
	bus       *EventBus[T]
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	workers   int
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
	for i := 0; i < workers; i++ {
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
		// Queue is full, drop the event to prevent blocking
	}
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

// Shutdown stops all workers
func (wp *WorkerPool[T]) Shutdown() {
	// Cancel context to signal workers to stop
	wp.cancel()
	// Close the channel to prevent new events
	close(wp.eventChan)
	// Wait for all workers to finish
	wp.wg.Wait()
}
