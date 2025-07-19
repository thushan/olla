package stats

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

const (
	MaxTrackedModels     = 100
	ModelStatsTTL        = 24 * time.Hour
	ModelCleanupInterval = 1 * time.Hour
	LatencyBucketSize    = 1000 // Store last N latencies for percentile calculation
)

// ModelCollector tracks model-specific statistics
type ModelCollector struct {
	models         *xsync.Map[string, *modelData]
	modelEndpoints *xsync.Map[string, *xsync.Map[string, *modelEndpointData]]

	lastCleanup int64
	cleanupMu   sync.Mutex
}

type modelData struct {
	uniqueClients      *xsync.Map[string, int64] // IP -> last seen timestamp
	totalRequests      *xsync.Counter
	successfulRequests *xsync.Counter
	failedRequests     *xsync.Counter
	totalBytes         *xsync.Counter
	totalLatency       *xsync.Counter

	// Routing effectiveness
	routingHits      *xsync.Counter
	routingMisses    *xsync.Counter
	routingFallbacks *xsync.Counter
	name             string
	latencies        []int64 // Circular buffer for percentile calculation
	latencyIndex     int
	lastRequested    int64 // Keep atomic for timestamp

	latencyMu sync.Mutex
}

type modelEndpointData struct {
	requestCount      *xsync.Counter
	successCount      *xsync.Counter
	totalLatency      *xsync.Counter
	endpointName      string
	modelName         string
	lastUsed          int64 // Keep atomic for timestamp
	consecutiveErrors int32 // Keep atomic for CAS operations
}

func NewModelCollector() *ModelCollector {
	return &ModelCollector{
		models:         xsync.NewMap[string, *modelData](),
		modelEndpoints: xsync.NewMap[string, *xsync.Map[string, *modelEndpointData]](),
		lastCleanup:    time.Now().UnixNano(),
	}
}

func (mc *ModelCollector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
	if model == "" {
		return
	}

	now := time.Now().UnixNano()
	latencyMs := latency.Milliseconds()

	// Update model stats
	data := mc.getOrInitModel(model, now)
	data.totalRequests.Inc()
	data.totalBytes.Add(bytes)
	data.totalLatency.Add(latencyMs)
	atomic.StoreInt64(&data.lastRequested, now)

	if status == StatusSuccess {
		data.successfulRequests.Inc()
		mc.recordLatency(data, latencyMs)
	} else {
		data.failedRequests.Inc()
	}

	// Update model-endpoint stats
	if endpoint != nil {
		mc.updateModelEndpointStats(model, endpoint, status, latencyMs, now)
	}

	mc.tryCleanup(now)
}

func (mc *ModelCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
	if model == "" || endpoint == nil {
		return
	}

	endpointMap, _ := mc.modelEndpoints.LoadOrStore(model, xsync.NewMap[string, *modelEndpointData]())
	if data, ok := endpointMap.Load(endpoint.Name); ok {
		atomic.AddInt32(&data.consecutiveErrors, 1)
	}
}

func (mc *ModelCollector) GetModelStats() map[string]ports.ModelStats {
	result := make(map[string]ports.ModelStats)

	mc.models.Range(func(name string, data *modelData) bool {
		stats := mc.calculateModelStats(data)
		result[name] = stats
		return true
	})

	return result
}

func (mc *ModelCollector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	result := make(map[string]map[string]ports.EndpointModelStats)

	mc.modelEndpoints.Range(func(modelName string, endpointMap *xsync.Map[string, *modelEndpointData]) bool {
		endpointStats := make(map[string]ports.EndpointModelStats)

		endpointMap.Range(func(endpointName string, data *modelEndpointData) bool {
			stats := mc.calculateEndpointModelStats(data)
			endpointStats[endpointName] = stats
			return true
		})

		result[modelName] = endpointStats
		return true
	})

	return result
}

func (mc *ModelCollector) getOrInitModel(model string, now int64) *modelData {
	data, _ := mc.models.LoadOrCompute(model, func() (*modelData, bool) {
		return &modelData{
			name:               model,
			latencies:          make([]int64, LatencyBucketSize),
			uniqueClients:      xsync.NewMap[string, int64](),
			lastRequested:      now,
			totalRequests:      xsync.NewCounter(),
			successfulRequests: xsync.NewCounter(),
			failedRequests:     xsync.NewCounter(),
			totalBytes:         xsync.NewCounter(),
			totalLatency:       xsync.NewCounter(),
			routingHits:        xsync.NewCounter(),
			routingMisses:      xsync.NewCounter(),
			routingFallbacks:   xsync.NewCounter(),
		}, false
	})
	return data
}

func (mc *ModelCollector) recordLatency(data *modelData, latencyMs int64) {
	data.latencyMu.Lock()
	defer data.latencyMu.Unlock()

	data.latencies[data.latencyIndex] = latencyMs
	data.latencyIndex = (data.latencyIndex + 1) % LatencyBucketSize
}

func (mc *ModelCollector) updateModelEndpointStats(model string, endpoint *domain.Endpoint, status string, latencyMs int64, now int64) {
	endpointMap, _ := mc.modelEndpoints.LoadOrStore(model, xsync.NewMap[string, *modelEndpointData]())

	data, _ := endpointMap.LoadOrCompute(endpoint.Name, func() (*modelEndpointData, bool) {
		return &modelEndpointData{
			endpointName: endpoint.Name,
			modelName:    model,
			requestCount: xsync.NewCounter(),
			successCount: xsync.NewCounter(),
			totalLatency: xsync.NewCounter(),
			lastUsed:     now,
		}, false
	})

	data.requestCount.Inc()
	data.totalLatency.Add(latencyMs)
	atomic.StoreInt64(&data.lastUsed, now)

	if status == StatusSuccess {
		data.successCount.Inc()
		atomic.StoreInt32(&data.consecutiveErrors, 0)
	} else {
		atomic.AddInt32(&data.consecutiveErrors, 1)
	}
}

func (mc *ModelCollector) calculateModelStats(data *modelData) ports.ModelStats {
	total := data.totalRequests.Value()
	successful := data.successfulRequests.Value()
	failed := data.failedRequests.Value()
	totalLatency := data.totalLatency.Value()

	var avgLatency int64
	if successful > 0 {
		avgLatency = totalLatency / successful
	}

	// Calculate percentiles
	p95, p99 := mc.calculatePercentiles(data)

	// Count unique clients
	var uniqueClients int64
	data.uniqueClients.Range(func(ip string, lastSeen int64) bool {
		uniqueClients++
		return true
	})

	return ports.ModelStats{
		Name:               data.name,
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		TotalBytes:         data.totalBytes.Value(),
		AverageLatency:     avgLatency,
		P95Latency:         p95,
		P99Latency:         p99,
		UniqueClients:      uniqueClients,
		LastRequested:      time.Unix(0, atomic.LoadInt64(&data.lastRequested)),
		RoutingHits:        data.routingHits.Value(),
		RoutingMisses:      data.routingMisses.Value(),
		RoutingFallbacks:   data.routingFallbacks.Value(),
	}
}

func (mc *ModelCollector) calculatePercentiles(data *modelData) (p95, p99 int64) {
	data.latencyMu.Lock()
	defer data.latencyMu.Unlock()

	// Collect non-zero latencies
	var latencies []int64
	for _, l := range data.latencies {
		if l > 0 {
			latencies = append(latencies, l)
		}
	}

	if len(latencies) == 0 {
		return 0, 0
	}

	// Sort for percentile calculation
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p95Index := int(float64(len(latencies)) * 0.95)
	p99Index := int(float64(len(latencies)) * 0.99)

	if p95Index < len(latencies) {
		p95 = latencies[p95Index]
	}
	if p99Index < len(latencies) {
		p99 = latencies[p99Index]
	}

	return p95, p99
}

func (mc *ModelCollector) calculateEndpointModelStats(data *modelEndpointData) ports.EndpointModelStats {
	requestCount := data.requestCount.Value()
	successCount := data.successCount.Value()

	var successRate float64
	if requestCount > 0 {
		successRate = float64(successCount) / float64(requestCount) * 100
	}

	var avgLatency int64
	if successCount > 0 {
		avgLatency = data.totalLatency.Value() / successCount
	}

	return ports.EndpointModelStats{
		EndpointName:      data.endpointName,
		ModelName:         data.modelName,
		RequestCount:      requestCount,
		SuccessRate:       successRate,
		AverageLatency:    avgLatency,
		LastUsed:          time.Unix(0, atomic.LoadInt64(&data.lastUsed)),
		ConsecutiveErrors: int(atomic.LoadInt32(&data.consecutiveErrors)),
	}
}

func (mc *ModelCollector) tryCleanup(now int64) {
	lastCleanup := atomic.LoadInt64(&mc.lastCleanup)
	if now-lastCleanup < ModelCleanupInterval.Nanoseconds() {
		return
	}

	mc.cleanupMu.Lock()
	defer mc.cleanupMu.Unlock()

	// Double-check after acquiring lock
	if now-atomic.LoadInt64(&mc.lastCleanup) < ModelCleanupInterval.Nanoseconds() {
		return
	}

	mc.cleanupOldData(now)
	atomic.StoreInt64(&mc.lastCleanup, now)
}

func (mc *ModelCollector) cleanupOldData(now int64) {
	cutoff := now - ModelStatsTTL.Nanoseconds()

	// Clean up old models
	var modelsToDelete []string
	mc.models.Range(func(name string, data *modelData) bool {
		if atomic.LoadInt64(&data.lastRequested) < cutoff {
			modelsToDelete = append(modelsToDelete, name)
		} else {
			// Clean up old client IPs
			var ipsToDelete []string
			data.uniqueClients.Range(func(ip string, lastSeen int64) bool {
				if lastSeen < cutoff {
					ipsToDelete = append(ipsToDelete, ip)
				}
				return true
			})
			for _, ip := range ipsToDelete {
				data.uniqueClients.Delete(ip)
			}
		}
		return true
	})

	for _, name := range modelsToDelete {
		mc.models.Delete(name)
		mc.modelEndpoints.Delete(name)
	}

	// Keep only top N models if we exceed the limit
	if mc.models.Size() > MaxTrackedModels {
		mc.pruneExcessModels()
	}
}

func (mc *ModelCollector) pruneExcessModels() {
	type modelActivity struct {
		name          string
		lastRequested int64
	}

	var models []modelActivity
	mc.models.Range(func(name string, data *modelData) bool {
		models = append(models, modelActivity{
			name:          name,
			lastRequested: atomic.LoadInt64(&data.lastRequested),
		})
		return true
	})

	// Sort by last requested time, newest first
	sort.Slice(models, func(i, j int) bool {
		return models[i].lastRequested > models[j].lastRequested
	})

	// Delete the oldest models
	for i := MaxTrackedModels; i < len(models); i++ {
		mc.models.Delete(models[i].name)
		mc.modelEndpoints.Delete(models[i].name)
	}
}
