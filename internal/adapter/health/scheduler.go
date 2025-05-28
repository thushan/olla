package health

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// HealthScheduler manages the timing of health checks using a heap-based priority queue
type HealthScheduler struct {
	heap   *checkHeap
	heapMu sync.Mutex
	stopCh chan struct{}
	jobCh  chan<- healthCheckJob
}

// NewHealthScheduler creates a new health check scheduler
func NewHealthScheduler(jobCh chan<- healthCheckJob) *HealthScheduler {
	heapInstance := &checkHeap{}
	heap.Init(heapInstance)

	return &HealthScheduler{
		heap:   heapInstance,
		jobCh:  jobCh,
		stopCh: make(chan struct{}),
	}
}

// Start begins the scheduler loop
func (hs *HealthScheduler) Start(ctx context.Context, repository domain.EndpointRepository) {
	endpoints, err := repository.GetAll(ctx)
	if err == nil {
		hs.heapMu.Lock()
		for _, endpoint := range endpoints {
			heap.Push(hs.heap, &scheduledCheck{
				endpoint: endpoint,
				dueTime:  endpoint.NextCheckTime,
				ctx:      ctx,
			})
		}
		hs.heapMu.Unlock()
	}

	go hs.schedulerLoop(ctx)
}

// Stop stops the scheduler
func (hs *HealthScheduler) Stop() {
	close(hs.stopCh)
}

// ScheduleCheck adds a health check to the schedule
func (hs *HealthScheduler) ScheduleCheck(endpoint *domain.Endpoint, dueTime time.Time, ctx context.Context) {
	hs.heapMu.Lock()
	defer hs.heapMu.Unlock()

	heap.Push(hs.heap, &scheduledCheck{
		endpoint: endpoint,
		dueTime:  dueTime,
		ctx:      ctx,
	})
}

// GetScheduledCount returns the number of scheduled checks
func (hs *HealthScheduler) GetScheduledCount() int {
	hs.heapMu.Lock()
	defer hs.heapMu.Unlock()
	return hs.heap.Len()
}

// schedulerLoop is the main scheduler goroutine that processes due health checks
func (hs *HealthScheduler) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond) // Check more frequently for heap
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hs.stopCh:
			return
		case now := <-ticker.C:
			hs.processDueChecks(now, ctx)
		}
	}
}

// processDueChecks processes all health checks that are due
func (hs *HealthScheduler) processDueChecks(now time.Time, ctx context.Context) {
	hs.heapMu.Lock()
	defer hs.heapMu.Unlock()

	// Process all due checks
	for hs.heap.Len() > 0 {
		next := (*hs.heap)[0]
		if now.Before(next.dueTime) {
			break // Next check isn't due yet
		}

		check := heap.Pop(hs.heap).(*scheduledCheck)

		job := healthCheckJob{
			endpoint: check.endpoint,
			ctx:      check.ctx,
		}

		select {
		case hs.jobCh <- job:
			// Queued successfully
		default:
			// Queue full, reschedule in 1 second
			check.dueTime = now.Add(time.Second)
			heap.Push(hs.heap, check)
		}
	}
}
