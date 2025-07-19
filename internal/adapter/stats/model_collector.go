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

// ModelCollector tracks model-specific statistics
type ModelCollector struct {
	models         *xsync.Map[string, *modelData]
	modelEndpoints *xsync.Map[string, *xsync.Map[string, *modelEndpointData]]

	config *ModelCollectorConfig

	lastCleanup int64
	cleanupMu   sync.Mutex
}

type modelData struct {

	// Percentile tracking
	percentileTracker  PercentileTracker
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

	lastRequested int64 // Keep atomic for timestamp

	latencyMu sync.Mutex // Used for thread-safe percentile tracker access
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

// NewModelCollectorWithConfig creates a new ModelCollector with custom configuration
func NewModelCollectorWithConfig(config *ModelCollectorConfig) *ModelCollector {
	if config == nil {
		config = DefaultModelCollectorConfig()
	}
	config.Validate()

	return &ModelCollector{
		models:         xsync.NewMap[string, *modelData](),
		modelEndpoints: xsync.NewMap[string, *xsync.Map[string, *modelEndpointData]](),
		lastCleanup:    time.Now().UnixNano(),
		config:         config,
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

	// Update model-endpoint stats only if detailed stats are enabled
	if mc.config.EnableDetailedStats && endpoint != nil {
		mc.updateModelEndpointStats(model, endpoint, status, latencyMs, now)
	}

	mc.tryCleanup(now)
}

func (mc *ModelCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
	if !mc.config.EnableDetailedStats || model == "" || endpoint == nil {
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

	// Return empty map if detailed stats are disabled
	if !mc.config.EnableDetailedStats {
		return result
	}

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
		var tracker PercentileTracker

		// Create appropriate percentile tracker based on config
		switch mc.config.PercentileTrackerType {
		case "simple":
			tracker = NewSimpleStatsTracker()
		default: // "reservoir"
			tracker = NewReservoirSampler(mc.config.PercentileSampleSize)
		}

		return &modelData{
			name:               model,
			percentileTracker:  tracker,
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

	if data.percentileTracker != nil {
		data.percentileTracker.Add(latencyMs)
	}
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

	if data.percentileTracker != nil {
		// Use the efficient percentile tracker
		_, p95, p99 = data.percentileTracker.GetPercentiles()
		return p95, p99
	}

	// No tracker available
	return 0, 0
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
	cleanupInterval := mc.config.ModelCleanupInterval.Nanoseconds()
	if now-lastCleanup < cleanupInterval {
		return
	}

	mc.cleanupMu.Lock()
	defer mc.cleanupMu.Unlock()

	// Double-check after acquiring lock
	if now-atomic.LoadInt64(&mc.lastCleanup) < cleanupInterval {
		return
	}

	mc.cleanupOldData(now)
	atomic.StoreInt64(&mc.lastCleanup, now)
}

func (mc *ModelCollector) cleanupOldData(now int64) {
	modelCutoff := now - mc.config.ModelStatsTTL.Nanoseconds()
	clientIPCutoff := now - mc.config.ClientIPRetentionTime.Nanoseconds()

	// Clean up old models
	var modelsToDelete []string
	mc.models.Range(func(name string, data *modelData) bool {
		if atomic.LoadInt64(&data.lastRequested) < modelCutoff {
			modelsToDelete = append(modelsToDelete, name)
		} else {
			// Clean up old client IPs more aggressively
			mc.cleanupClientIPs(data, clientIPCutoff)
		}
		return true
	})

	for _, name := range modelsToDelete {
		mc.models.Delete(name)
		mc.modelEndpoints.Delete(name)
	}

	// Keep only top N models if we exceed the limit
	if mc.models.Size() > mc.config.MaxTrackedModels {
		mc.pruneExcessModels()
	}
}

// cleanupClientIPs removes old client IPs and enforces the max clients per model limit
func (mc *ModelCollector) cleanupClientIPs(data *modelData, cutoff int64) {
	var ipsToDelete []string
	var allIPs []struct {
		ip   string
		time int64
	}

	data.uniqueClients.Range(func(ip string, lastSeen int64) bool {
		if lastSeen < cutoff {
			ipsToDelete = append(ipsToDelete, ip)
		} else {
			allIPs = append(allIPs, struct {
				ip   string
				time int64
			}{ip, lastSeen})
		}
		return true
	})

	// Delete old IPs
	for _, ip := range ipsToDelete {
		data.uniqueClients.Delete(ip)
	}

	// If we still have too many IPs, remove the oldest ones
	if len(allIPs) > mc.config.MaxUniqueClientsPerModel {
		// Sort by time, oldest first
		sort.Slice(allIPs, func(i, j int) bool {
			return allIPs[i].time < allIPs[j].time
		})

		// Delete the oldest IPs
		toRemove := len(allIPs) - mc.config.MaxUniqueClientsPerModel
		for i := 0; i < toRemove; i++ {
			data.uniqueClients.Delete(allIPs[i].ip)
		}
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
	for i := mc.config.MaxTrackedModels; i < len(models); i++ {
		mc.models.Delete(models[i].name)
		mc.modelEndpoints.Delete(models[i].name)
	}
}
