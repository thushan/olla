package core

import (
	"sync/atomic"

	"github.com/thushan/olla/internal/core/ports"
)

// ProxyStats contains common proxy statistics
type ProxyStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	TotalLatency       int64
	MinLatency         int64
	MaxLatency         int64
}

// RecordSuccess records a successful proxy request
func (s *ProxyStats) RecordSuccess(latency int64) {
	atomic.AddInt64(&s.SuccessfulRequests, 1)
	atomic.AddInt64(&s.TotalLatency, latency)

	// Update min latency
	for {
		oldMin := atomic.LoadInt64(&s.MinLatency)
		if oldMin != 0 && oldMin <= latency {
			break
		}
		if atomic.CompareAndSwapInt64(&s.MinLatency, oldMin, latency) {
			break
		}
	}

	// Update max latency
	for {
		oldMax := atomic.LoadInt64(&s.MaxLatency)
		if oldMax >= latency {
			break
		}
		if atomic.CompareAndSwapInt64(&s.MaxLatency, oldMax, latency) {
			break
		}
	}
}

// RecordFailure records a failed proxy request
func (s *ProxyStats) RecordFailure() {
	atomic.AddInt64(&s.FailedRequests, 1)
}

// GetStats returns current statistics
func (s *ProxyStats) GetStats() ports.ProxyStats {
	total := atomic.LoadInt64(&s.TotalRequests)
	successful := atomic.LoadInt64(&s.SuccessfulRequests)
	failed := atomic.LoadInt64(&s.FailedRequests)
	totalLatency := atomic.LoadInt64(&s.TotalLatency)

	avgLatency := int64(0)
	if successful > 0 {
		avgLatency = totalLatency / successful
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
		MinLatency:         atomic.LoadInt64(&s.MinLatency),
		MaxLatency:         atomic.LoadInt64(&s.MaxLatency),
	}
}
