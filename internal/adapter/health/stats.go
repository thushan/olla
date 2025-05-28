package health

import (
	"context"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
)

// StatsCollector collects and provides statistics about health checking operations
type StatsCollector struct {
	mu             sync.RWMutex
	running        bool
	workerPool     *WorkerPool
	scheduler      *HealthScheduler
	circuitBreaker *CircuitBreaker
	statusTracker  *StatusTransitionTracker
	repository     domain.EndpointRepository
}

func NewStatsCollector(
	workerPool *WorkerPool,
	scheduler *HealthScheduler,
	circuitBreaker *CircuitBreaker,
	statusTracker *StatusTransitionTracker,
	repository domain.EndpointRepository,
) *StatsCollector {
	return &StatsCollector{
		workerPool:     workerPool,
		scheduler:      scheduler,
		circuitBreaker: circuitBreaker,
		statusTracker:  statusTracker,
		repository:     repository,
	}
}

func (sc *StatsCollector) SetRunning(running bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.running = running
}

// GetSchedulerStats returns comprehensive statistics about the health checker
func (sc *StatsCollector) GetSchedulerStats() map[string]interface{} {
	sc.mu.RLock()
	running := sc.running
	sc.mu.RUnlock()

	if !running {
		return map[string]interface{}{
			"running": false,
		}
	}

	queueSize, queueCap, queueUsage := sc.workerPool.GetQueueStats()
	heapSize := sc.scheduler.GetScheduledCount()

	return map[string]interface{}{
		"running":          running,
		"worker_count":     sc.getWorkerCount(),
		"queue_size":       queueSize,
		"queue_cap":        queueCap,
		"queue_usage":      queueUsage,
		"scheduled_checks": heapSize,
	}
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (sc *StatsCollector) GetCircuitBreakerStats() map[string]interface{} {
	activeEndpoints := sc.circuitBreaker.GetActiveEndpoints()

	openCircuits := 0
	for _, endpoint := range activeEndpoints {
		if sc.circuitBreaker.IsOpen(endpoint) {
			openCircuits++
		}
	}

	return map[string]interface{}{
		"total_endpoints":  len(activeEndpoints),
		"open_circuits":    openCircuits,
		"active_endpoints": activeEndpoints,
	}
}

// GetStatusTrackerStats returns status transition tracker statistics
func (sc *StatsCollector) GetStatusTrackerStats() map[string]interface{} {
	activeEndpoints := sc.statusTracker.GetActiveEndpoints()

	return map[string]interface{}{
		"tracked_endpoints": len(activeEndpoints),
		"active_endpoints":  activeEndpoints,
	}
}

// GetRepositoryStats returns repository cache statistics
func (sc *StatsCollector) GetRepositoryStats() map[string]interface{} {
	return sc.repository.GetCacheStats()
}

// GetEndpointCounts returns counts of endpoints by status
func (sc *StatsCollector) GetEndpointCounts(ctx context.Context) map[string]interface{} {
	all, err := sc.repository.GetAll(ctx)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	healthy, err := sc.repository.GetHealthy(ctx)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	routable, err := sc.repository.GetRoutable(ctx)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	// Count by status
	statusCounts := make(map[string]int)
	for _, endpoint := range all {
		statusCounts[endpoint.Status.String()]++
	}

	return map[string]interface{}{
		"total_endpoints":     len(all),
		"healthy_endpoints":   len(healthy),
		"routable_endpoints":  len(routable),
		"unhealthy_endpoints": len(all) - len(routable),
		"status_breakdown":    statusCounts,
	}
}

// GetComprehensiveStats returns all statistics in one call
func (sc *StatsCollector) GetComprehensiveStats(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"scheduler":       sc.GetSchedulerStats(),
		"circuit_breaker": sc.GetCircuitBreakerStats(),
		"status_tracker":  sc.GetStatusTrackerStats(),
		"repository":      sc.GetRepositoryStats(),
		"endpoints":       sc.GetEndpointCounts(ctx),
	}
}

// getWorkerCount returns the worker count from the worker pool
func (sc *StatsCollector) getWorkerCount() int {
	// This would need to be exposed by WorkerPool if needed
	// For now, we'll need to track this separately or expose it from WorkerPool
	return DefaultHealthCheckerWorkerCount // fallback
}
