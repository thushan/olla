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

	"github.com/puzpuzpuz/xsync/v4"
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
	logger logger.StyledLogger

	uniqueRateLimitedIPs map[string]int64

	endpoints *xsync.Map[string, *endpointData]

	// Model statistics tracking
	modelCollector *ModelCollector

	// Translator statistics tracking
	translatorCollector *TranslatorCollector

	// Using xsync.Counter for better performance under high contention
	totalRequests      *xsync.Counter
	successfulRequests *xsync.Counter
	failedRequests     *xsync.Counter
	totalLatency       *xsync.Counter

	rateLimitViolations *xsync.Counter
	sizeLimitViolations *xsync.Counter
	lastCleanup         int64
	securityMu          sync.RWMutex

	cleanupMu sync.Mutex
}

type endpointData struct {
	totalRequests      *xsync.Counter
	successfulRequests *xsync.Counter
	failedRequests     *xsync.Counter
	totalBytes         *xsync.Counter
	totalLatency       *xsync.Counter
	name               string
	url                string
	activeConnections  int64 // Still using atomic for decrement operations
	minLatency         int64 // Keep atomic for CAS operations
	maxLatency         int64 // Keep atomic for CAS operations
	lastUsed           int64 // Keep atomic for timestamp
}

func NewCollector(logger logger.StyledLogger) *Collector {
	return NewCollectorWithConfig(logger, DefaultModelCollectorConfig())
}

// NewCollectorWithConfig creates a new Collector with custom ModelCollector configuration
func NewCollectorWithConfig(logger logger.StyledLogger, modelConfig *ModelCollectorConfig) *Collector {
	if modelConfig == nil {
		modelConfig = DefaultModelCollectorConfig()
	}

	return &Collector{
		uniqueRateLimitedIPs: make(map[string]int64),
		logger:               logger,
		endpoints:            xsync.NewMap[string, *endpointData](),
		lastCleanup:          time.Now().UnixNano(),
		modelCollector:       NewModelCollectorWithConfig(modelConfig),
		translatorCollector:  NewTranslatorCollector(),
		totalRequests:        xsync.NewCounter(),
		successfulRequests:   xsync.NewCounter(),
		failedRequests:       xsync.NewCounter(),
		totalLatency:         xsync.NewCounter(),
		rateLimitViolations:  xsync.NewCounter(),
		sizeLimitViolations:  xsync.NewCounter(),
	}
}

func (c *Collector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
	now := time.Now().UnixNano()
	latencyMs := latency.Milliseconds()

	c.totalRequests.Inc()

	if status == StatusSuccess {
		c.successfulRequests.Inc()
		// Update total latency only for successful requests
		// realised in TestCollector_RecordRequest
		c.totalLatency.Add(latencyMs)
	} else {
		c.failedRequests.Inc()
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
	total := c.totalRequests.Value()
	successful := c.successfulRequests.Value()
	failed := c.failedRequests.Value()
	totalLatency := c.totalLatency.Value()

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

	c.endpoints.Range(func(url string, data *endpointData) bool {
		total := data.totalRequests.Value()
		successful := data.successfulRequests.Value()
		failed := data.failedRequests.Value()
		totalLatency := data.totalLatency.Value()
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
			TotalBytes:         data.totalBytes.Value(),
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
		RateLimitViolations:  c.rateLimitViolations.Value(),
		SizeLimitViolations:  c.sizeLimitViolations.Value(),
		UniqueRateLimitedIPs: uniqueIPs,
	}
}

func (c *Collector) GetConnectionStats() map[string]int64 {
	stats := make(map[string]int64)

	c.endpoints.Range(func(url string, data *endpointData) bool {
		stats[url] = atomic.LoadInt64(&data.activeConnections)
		return true
	})

	return stats
}

func (c *Collector) RecordSecurityViolation(violation ports.SecurityViolation) {
	switch violation.ViolationType {
	case constants.ViolationRateLimit:
		c.rateLimitViolations.Inc()
		c.recordRateLimitedIP(violation.ClientID)
	case constants.ViolationSizeLimit:
		c.sizeLimitViolations.Inc()
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

	data.totalRequests.Inc()
	data.totalBytes.Add(bytes)
	atomic.StoreInt64(&data.lastUsed, now)

	if status == StatusSuccess {
		data.successfulRequests.Inc()
		data.totalLatency.Add(latencyMs)
		c.updateLatencyBounds(data, latencyMs)
	} else {
		data.failedRequests.Inc()
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
	key := endpoint.URLString
	data, _ := c.endpoints.LoadOrCompute(key, func() (newValue *endpointData, cancel bool) {
		return &endpointData{
			url:                key,
			name:               endpoint.Name,
			lastUsed:           now,
			minLatency:         -1,
			totalRequests:      xsync.NewCounter(),
			successfulRequests: xsync.NewCounter(),
			failedRequests:     xsync.NewCounter(),
			totalBytes:         xsync.NewCounter(),
			totalLatency:       xsync.NewCounter(),
		}, false
	})
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

	c.endpoints.Range(func(url string, data *endpointData) bool {
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
		c.endpoints.Range(func(url string, data *endpointData) bool {
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

// Model-specific tracking methods

func (c *Collector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
	// Record in both regular stats and model-specific stats
	c.RecordRequest(endpoint, status, latency, bytes)
	c.modelCollector.RecordModelRequest(model, endpoint, status, latency, bytes)
}

func (c *Collector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
	c.modelCollector.RecordModelError(model, endpoint, errorType)
}

func (c *Collector) GetModelStats() map[string]ports.ModelStats {
	return c.modelCollector.GetModelStats()
}

func (c *Collector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	return c.modelCollector.GetModelEndpointStats()
}

// Translator-specific tracking methods

func (c *Collector) RecordTranslatorRequest(event ports.TranslatorRequestEvent) {
	c.translatorCollector.Record(event)
}

func (c *Collector) GetTranslatorStats() map[string]ports.TranslatorStats {
	return c.translatorCollector.GetStats()
}
