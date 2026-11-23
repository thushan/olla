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
	"cmp"
	"slices"
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

	// MaxUniqueRateLimitedIPs caps the in-memory set of rate-limited client IPs.
	// Under an active flood attack the map would otherwise grow without bound.
	// At cap, new entries are refused until the periodic age-eviction frees space.
	MaxUniqueRateLimitedIPs = 10_000

	// ipCleanupInterval is how often the age-based eviction scan runs. Keeping
	// it off the per-violation hot path avoids an O(N) scan under high load.
	ipCleanupInterval = 5 * time.Minute
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

	// Global latency bounds across all endpoints, CAS-updated the same way as
	// endpointData's per-endpoint bounds (see updateLatencyBounds). minLatency
	// starts at -1 as an "unset" sentinel since 0 is a valid observed latency.
	minLatency atomic.Int64
	maxLatency atomic.Int64

	lastCleanup   atomic.Int64
	lastIPCleanup atomic.Int64 // tracks when the age-eviction scan last ran
	securityMu    sync.RWMutex

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
	activeConnections  atomic.Int64
	minLatency         atomic.Int64
	maxLatency         atomic.Int64
	lastUsed           atomic.Int64
}

func NewCollector(logger logger.StyledLogger) *Collector {
	return NewCollectorWithConfig(logger, DefaultModelCollectorConfig())
}

// NewCollectorWithConfig creates a new Collector with custom ModelCollector configuration
func NewCollectorWithConfig(logger logger.StyledLogger, modelConfig *ModelCollectorConfig) *Collector {
	if modelConfig == nil {
		modelConfig = DefaultModelCollectorConfig()
	}

	uniqueIPs := make(map[string]int64)
	endpoints := xsync.NewMap[string, *endpointData]()
	// Sampled exactly where the base's composite literal sampled it: after the
	// map allocations, before the sub-collector constructors.
	now := time.Now().UnixNano()
	// atomic.Int64 cannot carry a non-zero value in a composite literal, so
	// the sentinel and the cleanup watermark are stored after the literal,
	// still before c is published to callers.
	c := &Collector{
		uniqueRateLimitedIPs: uniqueIPs,
		logger:               logger,
		endpoints:            endpoints,
		modelCollector:       NewModelCollectorWithConfig(modelConfig),
		translatorCollector:  NewTranslatorCollector(),
		totalRequests:        xsync.NewCounter(),
		successfulRequests:   xsync.NewCounter(),
		failedRequests:       xsync.NewCounter(),
		totalLatency:         xsync.NewCounter(),
		rateLimitViolations:  xsync.NewCounter(),
		sizeLimitViolations:  xsync.NewCounter(),
	}
	c.lastCleanup.Store(now)
	c.minLatency.Store(-1)
	return c
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
		c.updateGlobalLatencyBounds(latencyMs)
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
		data.activeConnections.Add(int64(delta))
	} else if delta < 0 {
		for {
			current := data.activeConnections.Load()
			newVal := max(current+int64(delta), 0)
			if data.activeConnections.CompareAndSwap(current, newVal) {
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

	minLatency := c.minLatency.Load()
	if minLatency == -1 {
		minLatency = 0
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
		MinLatency:         minLatency,
		MaxLatency:         c.maxLatency.Load(),
	}
}

// updateGlobalLatencyBounds is the global-across-all-endpoints equivalent of
// updateLatencyBounds, feeding GetProxyStats' MinLatency/MaxLatency.
func (c *Collector) updateGlobalLatencyBounds(latencyMs int64) {
	for {
		minLatency := c.minLatency.Load()
		if minLatency == -1 || latencyMs < minLatency {
			if c.minLatency.CompareAndSwap(minLatency, latencyMs) {
				break
			}
		} else {
			break
		}
	}
	for {
		maxLatency := c.maxLatency.Load()
		if latencyMs > maxLatency {
			if c.maxLatency.CompareAndSwap(maxLatency, latencyMs) {
				break
			}
		} else {
			break
		}
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

		minLatency := data.minLatency.Load()
		if minLatency == -1 {
			minLatency = 0
		}

		stats[url] = ports.EndpointStats{
			Name:               data.name,
			URL:                data.url,
			ActiveConnections:  data.activeConnections.Load(),
			TotalRequests:      total,
			SuccessfulRequests: successful,
			FailedRequests:     failed,
			TotalBytes:         data.totalBytes.Value(),
			AverageLatency:     avgLatency,
			MinLatency:         minLatency,
			MaxLatency:         data.maxLatency.Load(),
			LastUsedNano:       data.lastUsed.Load(),
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
		stats[url] = data.activeConnections.Load()
		return true
	})

	return stats
}

// GetConnectionCount returns the active connection count for a single endpoint,
// keyed the same way as GetConnectionStats (endpoint URL string). Returns 0 for
// an endpoint that has never recorded a connection, matching GetConnectionStats'
// implicit-zero behaviour for missing keys.
func (c *Collector) GetConnectionCount(endpoint string) int64 {
	data, ok := c.endpoints.Load(endpoint)
	if !ok {
		return 0
	}
	return data.activeConnections.Load()
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

	// Age-eviction scan runs at most once per ipCleanupInterval, not on every
	// violation. The pre-check outside the lock avoids contention when the
	// interval has not yet elapsed (the common case under a flood).
	lastClean := c.lastIPCleanup.Load()
	if now-lastClean >= int64(ipCleanupInterval) {
		c.cleanupOldRateLimitedIPs(now)
	}

	c.securityMu.Lock()
	// Refuse new entries once the hard cap is reached. At cap, the periodic
	// eviction will free space for the next cleanup cycle.
	if len(c.uniqueRateLimitedIPs) < MaxUniqueRateLimitedIPs {
		c.uniqueRateLimitedIPs[clientIP] = now
	}
	c.securityMu.Unlock()
}

// cleanupOldRateLimitedIPs removes IPs whose last-seen timestamp is older than
// one hour. Uses a double-checked lock to avoid redundant scans when multiple
// goroutines arrive at the interval boundary concurrently.
func (c *Collector) cleanupOldRateLimitedIPs(now int64) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()

	// Second check after acquiring the cleanup lock.
	if now-c.lastIPCleanup.Load() < int64(ipCleanupInterval) {
		return
	}

	cutoff := now - int64(time.Hour)

	c.securityMu.Lock()
	for ip, ts := range c.uniqueRateLimitedIPs {
		if ts < cutoff {
			delete(c.uniqueRateLimitedIPs, ip)
		}
	}
	c.securityMu.Unlock()

	c.lastIPCleanup.Store(now)
}

func (c *Collector) updateEndpointStats(endpoint *domain.Endpoint, status string, latencyMs, bytes int64, now int64) {
	data := c.getOrInitEndpoint(endpoint, now)

	data.totalRequests.Inc()
	data.totalBytes.Add(bytes)
	data.lastUsed.Store(now)

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
		minLatency := data.minLatency.Load()
		if minLatency == -1 || latencyMs < minLatency {
			if data.minLatency.CompareAndSwap(minLatency, latencyMs) {
				break
			}
		} else {
			break
		}
	}
	for {
		maxLatency := data.maxLatency.Load()
		if latencyMs > maxLatency {
			if data.maxLatency.CompareAndSwap(maxLatency, latencyMs) {
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
		d := &endpointData{
			url:                key,
			name:               endpoint.Name,
			totalRequests:      xsync.NewCounter(),
			successfulRequests: xsync.NewCounter(),
			failedRequests:     xsync.NewCounter(),
			totalBytes:         xsync.NewCounter(),
			totalLatency:       xsync.NewCounter(),
		}
		// Set inside the creation callback, before the entry is published:
		// a zero lastUsed would let TTL cleanup evict an endpoint whose first
		// update was connection-only, and minLatency needs the -1 sentinel
		// because 0 is a valid observed latency.
		d.lastUsed.Store(now)
		d.minLatency.Store(-1)
		return d, false
	})
	return data
}

func (c *Collector) tryCleanup(now int64) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()

	if now-c.lastCleanup.Load() < int64(CleanupInterval) {
		return
	}

	c.cleanup(now)
	c.lastCleanup.Store(now)
}

func (c *Collector) cleanup(now int64) {
	cutoff := now - int64(EndpointTTL)
	var toRemove []string
	var count int

	c.endpoints.Range(func(url string, data *endpointData) bool {
		count++
		if data.lastUsed.Load() < cutoff {
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
			ages = append(ages, endpointAge{url, data.lastUsed.Load()})
			return true
		})
		slices.SortFunc(ages, func(a, b endpointAge) int {
			return cmp.Compare(a.time, b.time)
		})
		remove := len(ages) - MaxTrackedEndpoints + 100
		for i := 0; i < remove && i < len(ages); i++ {
			c.endpoints.Delete(ages[i].url)
		}
		c.logger.Debug("Cleaned up old endpoint stats", "removed", remove, "remaining", len(ages)-remove)
	}
}

// Model-specific tracking methods

// RecordModelRequest records model-level stats only. Callers record the request
// itself through RecordRequest; doing it here as well counted every modelled
// request twice in the proxy and per-endpoint totals.
func (c *Collector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
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
