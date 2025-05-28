package health

import (
	"context"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

const (
	DefaultHealthCheckerWorkerCount = 10
	BaseHealthCheckerQueueSize      = 100
	QueueScaleFactor                = 2 // Queue size = endpoints * factor

	DefaultHealthCheckerTimeout = 5 * time.Second
	SlowResponseThreshold       = 10 * time.Second
	VerySlowResponseThreshold   = 30 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 3
	DefaultCircuitBreakerTimeout   = 30 * time.Second

	MaxBackoffMultiplier = 12
	BaseBackoffSeconds   = 2

	CleanupInterval = 5 * time.Minute
)

// scheduledCheck represents a health check scheduled for a specific time
type scheduledCheck struct {
	endpoint *domain.Endpoint
	dueTime  time.Time
	ctx      context.Context
}

// healthCheckJob represents a health check job to be processed by workers
type healthCheckJob struct {
	endpoint *domain.Endpoint
	ctx      context.Context
}

// checkHeap implements heap.Interface for efficient scheduling
type checkHeap []*scheduledCheck

func (h checkHeap) Len() int           { return len(h) }
func (h checkHeap) Less(i, j int) bool { return h[i].dueTime.Before(h[j].dueTime) }
func (h checkHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *checkHeap) Push(x interface{}) {
	*h = append(*h, x.(*scheduledCheck))
}

func (h *checkHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}
