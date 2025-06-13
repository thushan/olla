package stats

/*
				Olla Stats Collector - Centralised Stats Collection
	Collector centralises all the stats we track across Olla - requests, connections,
	security stuff, etc. Instead of each component doing its own thing, everything
	reports here so we can actually see what's happening system-wide.

	Thread-safe for high concurrency since this gets hit on every request & multiple.
	times. We also clean up old endpoint data automatically so we don't leak memory.

	NOTE: 	Cleanup values defined are conservative to avoid memory retention over
		  	long running processes. Most users will have 10-20 endpoints, so we keep
			the tracked endpoints to a maximum of 50.

	GOALS:
	- Keep it simple and efficient (reduce allocation overhead)
	- Track all relevant stats in one place
	- Provide easy access to stats for monitoring and debugging
	- Automatically clean up old data to prevent long memory retention
*/

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	StatusSuccess = "success"
	StatusFailure = "failure"

	// NOTE: These are not too high to avoid memory retention for long periods
	// Most folks would have 10-20 endpoints looking at Sherpa usage stats
	MaxTrackedEndpoints = 50
	EndpointTTL         = 1 * time.Hour
	CleanupInterval     = 5 * time.Minute
)

type Collector struct {
	uniqueRateLimitedIPs map[string]int64

	logger logger.StyledLogger

	endpoints sync.Map // map[string]*endpointData

	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalLatency       int64

	rateLimitViolations int64
	sizeLimitViolations int64
	lastCleanup         int64
	securityMu          sync.RWMutex

	cleanupMu sync.Mutex
}

type endpointData struct {
	name               string
	url                string
	activeConnections  int64
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalBytes         int64
	totalLatency       int64
	minLatency         int64
	maxLatency         int64
	lastUsed           int64
}

func NewCollector(logger logger.StyledLogger) *Collector {
	return &Collector{
		uniqueRateLimitedIPs: make(map[string]int64),
		logger:               logger,
		lastCleanup:          time.Now().UnixNano(),
	}
}

func (c *Collector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
	c.logger.Warn("STATS DEBUG: RecordRequest called", "endpoint", endpoint.GetURLString(), "status", status)
	now := time.Now().UnixNano()
	latencyMs := latency.Milliseconds()

	atomic.AddInt64(&c.totalRequests, 1)

	if status == StatusSuccess {
		atomic.AddInt64(&c.successfulRequests, 1)
		// Update total latency only for successful requests
		// realised in TestCollector_RecordRequest
		atomic.AddInt64(&c.totalLatency, latencyMs)
	} else {
		atomic.AddInt64(&c.failedRequests, 1)
	}

	// Only update endpoint-specific stats if endpoint is known
	if endpoint != nil {
		c.updateEndpointStats(endpoint, status, latencyMs, bytes, now)
	}
	c.tryCleanup(now)
}

func (c *Collector) RecordConnection(endpoint *domain.Endpoint, delta int) {
	now := time.Now().UnixNano()
	data := c.getOrInitEndpoint(endpoint, now)

	if delta > 0 {
		atomic.AddInt64(&data.activeConnections, int64(delta))
	} else if delta < 0 {
		for {
			current := atomic.LoadInt64(&data.activeConnections)
			newVal := current + int64(delta)
			if newVal < 0 {
				newVal = 0
			}
			if atomic.CompareAndSwapInt64(&data.activeConnections, current, newVal) {
				break
			}
		}
	}
}

func (c *Collector) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
	status := StatusFailure
	if success {
		status = StatusSuccess
	}

	c.logger.Debug("Discovery operation recorded",
		"endpoint", endpoint,
		"status", status,
		"latency_ms", latency.Milliseconds())
}

func (c *Collector) GetProxyStats() ports.ProxyStats {
	total := atomic.LoadInt64(&c.totalRequests)
	successful := atomic.LoadInt64(&c.successfulRequests)
	failed := atomic.LoadInt64(&c.failedRequests)
	totalLatency := atomic.LoadInt64(&c.totalLatency)

	var avgLatency int64
	if successful > 0 {
		avgLatency = totalLatency / successful
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
		MinLatency:         0, // Not implemented yet
		MaxLatency:         0, // Not implemented yet
	}
}

func (c *Collector) GetEndpointStats() map[string]ports.EndpointStats {
	stats := make(map[string]ports.EndpointStats)

	c.endpoints.Range(func(key, value interface{}) bool {
		url, ok1 := key.(string)
		data, ok2 := value.(*endpointData)
		if !ok1 || !ok2 {
			c.logger.Error("BUGCHECK: Failed to cast endpoint data, please file a bug report.", "url", key)
			return true
		}

		total := atomic.LoadInt64(&data.totalRequests)
		successful := atomic.LoadInt64(&data.successfulRequests)
		failed := atomic.LoadInt64(&data.failedRequests)
		totalLatency := atomic.LoadInt64(&data.totalLatency)
		avgLatency := int64(0)
		if successful > 0 {
			avgLatency = totalLatency / successful
		}

		successRate := 0.0
		if total > 0 {
			successRate = float64(successful) / float64(total) * 100
		}

		minLatency := atomic.LoadInt64(&data.minLatency)
		if minLatency == -1 {
			minLatency = 0
		}

		stats[url] = ports.EndpointStats{
			Name:               data.name,
			URL:                data.url,
			ActiveConnections:  atomic.LoadInt64(&data.activeConnections),
			TotalRequests:      total,
			SuccessfulRequests: successful,
			FailedRequests:     failed,
			TotalBytes:         atomic.LoadInt64(&data.totalBytes),
			AverageLatency:     avgLatency,
			MinLatency:         minLatency,
			MaxLatency:         atomic.LoadInt64(&data.maxLatency),
			LastUsedNano:       atomic.LoadInt64(&data.lastUsed),
			SuccessRate:        successRate,
		}
		return true
	})

	return stats
}

func (c *Collector) GetSecurityStats() ports.SecurityStats {
	c.securityMu.RLock()
	uniqueIPs := len(c.uniqueRateLimitedIPs)
	c.securityMu.RUnlock()

	return ports.SecurityStats{
		RateLimitViolations:  atomic.LoadInt64(&c.rateLimitViolations),
		SizeLimitViolations:  atomic.LoadInt64(&c.sizeLimitViolations),
		UniqueRateLimitedIPs: uniqueIPs,
	}
}

func (c *Collector) GetConnectionStats() map[string]int64 {
	stats := make(map[string]int64)

	c.endpoints.Range(func(key, value interface{}) bool {
		url, ok1 := key.(string)
		data, ok2 := value.(*endpointData)

		if !ok1 || !ok2 {
			c.logger.Error("BUGCHECK: Failed to cast endpoint data, please file a bug report.", "url", key)
			return true
		}

		stats[url] = atomic.LoadInt64(&data.activeConnections)
		return true
	})

	return stats
}

func (c *Collector) RecordSecurityViolation(violation ports.SecurityViolation) {
	switch violation.ViolationType {
	case constants.ViolationRateLimit:
		atomic.AddInt64(&c.rateLimitViolations, 1)
		c.recordRateLimitedIP(violation.ClientID)
	case constants.ViolationSizeLimit:
		atomic.AddInt64(&c.sizeLimitViolations, 1)
	}
}

func (c *Collector) recordRateLimitedIP(clientIP string) {
	now := time.Now().UnixNano()
	cutoff := now - int64(time.Hour)

	c.securityMu.Lock()
	c.uniqueRateLimitedIPs[clientIP] = now
	for ip, ts := range c.uniqueRateLimitedIPs {
		if ts < cutoff {
			delete(c.uniqueRateLimitedIPs, ip)
		}
	}
	c.securityMu.Unlock()
}

func (c *Collector) updateEndpointStats(endpoint *domain.Endpoint, status string, latencyMs, bytes int64, now int64) {
	data := c.getOrInitEndpoint(endpoint, now)

	atomic.AddInt64(&data.totalRequests, 1)
	atomic.AddInt64(&data.totalBytes, bytes)
	atomic.StoreInt64(&data.lastUsed, now)

	if status == StatusSuccess {
		atomic.AddInt64(&data.successfulRequests, 1)
		atomic.AddInt64(&data.totalLatency, latencyMs)
		c.updateLatencyBounds(data, latencyMs)
	} else {
		atomic.AddInt64(&data.failedRequests, 1)
	}
}

func (c *Collector) updateLatencyBounds(data *endpointData, latencyMs int64) {
	for {
		minLatency := atomic.LoadInt64(&data.minLatency)
		if minLatency == -1 || latencyMs < minLatency {
			if atomic.CompareAndSwapInt64(&data.minLatency, minLatency, latencyMs) {
				break
			}
		} else {
			break
		}
	}
	for {
		maxLatency := atomic.LoadInt64(&data.maxLatency)
		if latencyMs > maxLatency {
			if atomic.CompareAndSwapInt64(&data.maxLatency, maxLatency, latencyMs) {
				break
			}
		} else {
			break
		}
	}
}

func (c *Collector) getOrInitEndpoint(endpoint *domain.Endpoint, now int64) *endpointData {
	key := endpoint.URL.String()
	val, _ := c.endpoints.LoadOrStore(key, &endpointData{
		url:        key,
		name:       endpoint.Name,
		lastUsed:   now,
		minLatency: -1,
	})
	data, ok := val.(*endpointData)
	if !ok {
		c.logger.Error("BUGCHECK: Failed to cast endpoint data, please file a bug report.", "url", key)
		return nil
	}
	return data
}

func (c *Collector) tryCleanup(now int64) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()

	if now-atomic.LoadInt64(&c.lastCleanup) < int64(CleanupInterval) {
		return
	}

	c.cleanup(now)
	atomic.StoreInt64(&c.lastCleanup, now)
}

func (c *Collector) cleanup(now int64) {
	cutoff := now - int64(EndpointTTL)
	var toRemove []string
	var count int

	c.endpoints.Range(func(k, v interface{}) bool {
		url, ok1 := k.(string)
		data, ok2 := v.(*endpointData)
		if !ok1 || !ok2 {
			c.logger.Error("BUGCHECK: Failed to cast endpoint data, please file a bug report.", "url", url)
			return true
		}
		count++
		if atomic.LoadInt64(&data.lastUsed) < cutoff {
			toRemove = append(toRemove, url)
		}
		return true
	})

	for _, url := range toRemove {
		c.endpoints.Delete(url)
	}

	if count-len(toRemove) > MaxTrackedEndpoints {
		type endpointAge struct {
			url  string
			time int64
		}
		var ages []endpointAge
		c.endpoints.Range(func(k, v interface{}) bool {
			url, ok1 := k.(string)
			data, ok2 := v.(*endpointData)
			if !ok1 || !ok2 {
				c.logger.Error("BUGCHECK: Failed to cast endpoint data, please file a bug report.", "url", url)
				return true
			}
			ages = append(ages, endpointAge{url, atomic.LoadInt64(&data.lastUsed)})
			return true
		})
		sort.Slice(ages, func(i, j int) bool {
			return ages[i].time < ages[j].time
		})
		remove := len(ages) - MaxTrackedEndpoints + 100
		for i := 0; i < remove && i < len(ages); i++ {
			c.endpoints.Delete(ages[i].url)
		}
		c.logger.Debug("Cleaned up old endpoint stats", "removed", remove, "remaining", len(ages)-remove)
	}
}
