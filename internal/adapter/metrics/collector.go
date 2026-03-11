package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

const (
	// DefaultRingBufferSize stores the last N requests for querying
	DefaultRingBufferSize = 4096

	// DefaultChannelSize is the buffer size for the async metrics channel
	DefaultChannelSize = 256
)

// RequestCollector receives RequestMetrics asynchronously via a channel and stores them
// in a ring buffer for querying. Aggregated stats are computed on demand.
//
// Thread-safe and non-blocking on the producer side — the proxy hot path
// only does a non-blocking channel send.
type RequestCollector struct {
	ring    []RequestMetrics
	ringMu  sync.RWMutex
	ringPos int
	ringLen int
	ringCap int

	ch   chan RequestMetrics
	done chan struct{}
}

// NewRequestCollector creates a new metrics collector with default settings.
func NewRequestCollector() *RequestCollector {
	return NewRequestCollectorWithConfig(DefaultRingBufferSize, DefaultChannelSize)
}

// NewRequestCollectorWithConfig creates a new metrics collector with custom sizes.
func NewRequestCollectorWithConfig(ringSize, channelSize int) *RequestCollector {
	c := &RequestCollector{
		ring:    make([]RequestMetrics, ringSize),
		ringCap: ringSize,
		ch:      make(chan RequestMetrics, channelSize),
		done:    make(chan struct{}),
	}
	go c.consumeLoop()
	return c
}

// RecordRequestMetrics implements ports.RequestMetricsRecorder.
// Non-blocking: if the channel is full, the event is dropped silently
// to avoid backpressure on the proxy hot path.
func (c *RequestCollector) RecordRequestMetrics(event ports.RequestMetricsEvent) {
	select {
	case c.ch <- event:
	default:
		// Channel full — drop the metric rather than block the proxy
	}
}

// consumeLoop runs in a dedicated goroutine, draining the channel into the ring buffer.
func (c *RequestCollector) consumeLoop() {
	for {
		select {
		case m := <-c.ch:
			c.ringMu.Lock()
			c.ring[c.ringPos] = m
			c.ringPos = (c.ringPos + 1) % c.ringCap
			if c.ringLen < c.ringCap {
				c.ringLen++
			}
			c.ringMu.Unlock()
		case <-c.done:
			return
		}
	}
}

// Shutdown stops the consumer goroutine.
func (c *RequestCollector) Shutdown() {
	close(c.done)
}

// GetRecentRequests returns the last N request metrics, most recent first.
func (c *RequestCollector) GetRecentRequests(limit int) []RequestMetrics {
	c.ringMu.RLock()
	defer c.ringMu.RUnlock()

	if limit <= 0 || limit > c.ringLen {
		limit = c.ringLen
	}
	if limit == 0 {
		return nil
	}

	result := make([]RequestMetrics, limit)
	pos := c.ringPos
	for i := 0; i < limit; i++ {
		pos--
		if pos < 0 {
			pos = c.ringCap - 1
		}
		result[i] = c.ring[pos]
	}
	return result
}

// GetAggregatedStats computes summary statistics from the ring buffer.
// Optionally filtered by time window (zero time = no filter).
func (c *RequestCollector) GetAggregatedStats(since time.Time) *AggregatedStats {
	c.ringMu.RLock()
	defer c.ringMu.RUnlock()

	stats := &AggregatedStats{
		ByModel:    make(map[string]*ModelAggregatedStats),
		ByEndpoint: make(map[string]*EndpointAggregatedStats),
		WindowEnd:  time.Now(),
	}

	if c.ringLen == 0 {
		stats.WindowStart = stats.WindowEnd
		return stats
	}

	var ttftValues []int64
	var durationValues []int64
	var tpsSum float64
	var tpsCount int64

	// Iterate ring buffer
	pos := c.ringPos
	for i := 0; i < c.ringLen; i++ {
		pos--
		if pos < 0 {
			pos = c.ringCap - 1
		}
		m := c.ring[pos]

		// Time filter
		if !since.IsZero() && m.StartTime.Before(since) {
			continue
		}

		// Track window bounds
		if stats.WindowStart.IsZero() || m.StartTime.Before(stats.WindowStart) {
			stats.WindowStart = m.StartTime
		}

		stats.TotalRequests++
		if m.Success {
			stats.SuccessfulRequests++
		} else {
			stats.FailedRequests++
		}
		if m.IsStreaming {
			stats.StreamingRequests++
		}

		stats.TotalInputTokens += int64(m.InputTokens)
		stats.TotalOutputTokens += int64(m.OutputTokens)

		if m.TTFTMs > 0 {
			ttftValues = append(ttftValues, m.TTFTMs)
		}
		durationValues = append(durationValues, m.TotalDurationMs)

		if m.TokensPerSecond > 0 {
			tpsSum += float64(m.TokensPerSecond)
			tpsCount++
		}

		// Per-model stats
		if m.Model != "" {
			ms, ok := stats.ByModel[m.Model]
			if !ok {
				ms = &ModelAggregatedStats{}
				stats.ByModel[m.Model] = ms
			}
			ms.TotalRequests++
			ms.TotalInputTokens += int64(m.InputTokens)
			ms.TotalOutputTokens += int64(m.OutputTokens)
			ms.AvgTTFTMs += m.TTFTMs
			ms.AvgDurationMs += m.TotalDurationMs
			if m.TokensPerSecond > 0 {
				ms.AvgTokensPerSec += float64(m.TokensPerSecond)
			}
		}

		// Per-endpoint stats
		if m.EndpointName != "" {
			es, ok := stats.ByEndpoint[m.EndpointName]
			if !ok {
				es = &EndpointAggregatedStats{}
				stats.ByEndpoint[m.EndpointName] = es
			}
			es.TotalRequests++
			es.TotalInputTokens += int64(m.InputTokens)
			es.TotalOutputTokens += int64(m.OutputTokens)
			es.AvgTTFTMs += m.TTFTMs
			es.AvgDurationMs += m.TotalDurationMs
			if m.TokensPerSecond > 0 {
				es.AvgTokensPerSec += float64(m.TokensPerSecond)
			}
		}
	}

	// Compute averages
	if stats.TotalRequests > 0 {
		stats.TTFTAvgMs = avg(ttftValues)
		stats.TTFTP50Ms = percentile(ttftValues, 50)
		stats.TTFTP95Ms = percentile(ttftValues, 95)
		stats.TTFTP99Ms = percentile(ttftValues, 99)

		stats.DurationAvgMs = avg(durationValues)
		stats.DurationP50Ms = percentile(durationValues, 50)
		stats.DurationP95Ms = percentile(durationValues, 95)
		stats.DurationP99Ms = percentile(durationValues, 99)
	}

	if tpsCount > 0 {
		stats.AvgTokensPerSec = tpsSum / float64(tpsCount)
	}

	// Convert model/endpoint sums to averages
	for _, ms := range stats.ByModel {
		if ms.TotalRequests > 0 {
			ms.AvgTTFTMs /= ms.TotalRequests
			ms.AvgDurationMs /= ms.TotalRequests
			ms.AvgTokensPerSec /= float64(ms.TotalRequests)
		}
	}
	for _, es := range stats.ByEndpoint {
		if es.TotalRequests > 0 {
			es.AvgTTFTMs /= es.TotalRequests
			es.AvgDurationMs /= es.TotalRequests
			es.AvgTokensPerSec /= float64(es.TotalRequests)
		}
	}

	return stats
}

func avg(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var sum int64
	for _, v := range values {
		sum += v
	}
	return sum / int64(len(values))
}

func percentile(values []int64, pct int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := (pct * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
