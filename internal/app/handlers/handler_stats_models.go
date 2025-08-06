package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

const (
	queryValueTrue = "true"
)

// safeInt64ToUint64 safely converts int64 to uint64 for byte formatting
func safeInt64ToUint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

type ModelStatsResponse struct {
	Timestamp time.Time                     `json:"timestamp"`
	Endpoints map[string]EndpointModelStats `json:"endpoints"`
	Models    []ModelStats                  `json:"models"`
	Summary   ModelStatsSummary             `json:"summary"`
}

type ModelStats struct {
	EndpointBreakdown  map[string]EndpointModelStats `json:"endpoint_breakdown,omitempty"`
	Name               string                        `json:"name"`
	SuccessRate        string                        `json:"success_rate"`
	TotalTraffic       string                        `json:"total_traffic"`
	AverageLatency     string                        `json:"average_latency"`
	P95Latency         string                        `json:"p95_latency"`
	P99Latency         string                        `json:"p99_latency"`
	LastRequested      string                        `json:"last_requested"`
	TotalRequests      int64                         `json:"total_requests"`
	SuccessfulRequests int64                         `json:"successful_requests"`
	FailedRequests     int64                         `json:"failed_requests"`
	UniqueClients      int64                         `json:"unique_clients"`
	RoutingHits        int64                         `json:"routing_hits"`
	RoutingMisses      int64                         `json:"routing_misses"`
	RoutingFallbacks   int64                         `json:"routing_fallbacks"`
}

type EndpointModelStats struct {
	EndpointName      string  `json:"endpoint_name"`
	AverageLatency    string  `json:"average_latency"`
	LastUsed          string  `json:"last_used"`
	RequestCount      int64   `json:"request_count"`
	SuccessRate       float64 `json:"success_rate"`
	ConsecutiveErrors int     `json:"consecutive_errors"`
}

type ModelStatsSummary struct {
	OverallSuccessRate string `json:"overall_success_rate"`
	TotalTraffic       string `json:"total_traffic"`
	MostPopularModel   string `json:"most_popular_model"`
	LeastPopularModel  string `json:"least_popular_model"`
	TotalModels        int    `json:"total_models"`
	ActiveModels       int    `json:"active_models"`
	TotalRequests      int64  `json:"total_requests"`
}

// modelStatsAccumulator tracks accumulated statistics
type modelStatsAccumulator struct {
	mostPopular        string
	leastPopular       string
	totalRequests      int64
	successfulRequests int64
	totalBytes         int64
	maxRequests        int64
	minRequests        int64
}

func (a *Application) modelStatsHandler(w http.ResponseWriter, r *http.Request) {
	statsCollector := a.statsCollector
	if statsCollector == nil {
		http.Error(w, "Stats collector not initialized", http.StatusServiceUnavailable)
		return
	}

	modelStats := statsCollector.GetModelStats()
	modelEndpointStats := statsCollector.GetModelEndpointStats()

	includeEndpoints := r.URL.Query().Get("include_endpoints") == queryValueTrue
	includeSummary := r.URL.Query().Get("include_summary") == queryValueTrue

	models, accumulator := a.buildModelStats(modelStats, modelEndpointStats, includeEndpoints)
	summary := a.buildSummary(models, modelStats, accumulator)
	endpoints := a.buildEndpointSummary(modelEndpointStats, includeSummary)

	response := ModelStatsResponse{
		Timestamp: time.Now(),
		Models:    models,
		Endpoints: endpoints,
		Summary:   summary,
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (a *Application) buildModelStats(modelStats map[string]ports.ModelStats,
	modelEndpointStats map[string]map[string]ports.EndpointModelStats,
	includeEndpoints bool) ([]ModelStats, *modelStatsAccumulator) {

	models := make([]ModelStats, 0, len(modelStats))
	acc := &modelStatsAccumulator{
		minRequests: int64(^uint64(0) >> 1),
	}

	for name, stats := range modelStats {
		model := a.convertModelStats(name, stats, modelEndpointStats, includeEndpoints)
		models = append(models, model)

		// Update accumulator
		acc.totalRequests += stats.TotalRequests
		acc.successfulRequests += stats.SuccessfulRequests
		acc.totalBytes += stats.TotalBytes

		if stats.TotalRequests > acc.maxRequests {
			acc.maxRequests = stats.TotalRequests
			acc.mostPopular = name
		}
		if stats.TotalRequests < acc.minRequests && stats.TotalRequests > 0 {
			acc.minRequests = stats.TotalRequests
			acc.leastPopular = name
		}
	}

	// Sort models by request count (most popular first)
	sort.Slice(models, func(i, j int) bool {
		return models[i].TotalRequests > models[j].TotalRequests
	})

	return models, acc
}

func (a *Application) convertModelStats(name string, stats ports.ModelStats,
	modelEndpointStats map[string]map[string]ports.EndpointModelStats,
	includeEndpoints bool) ModelStats {

	successRate := float64(0)
	if stats.TotalRequests > 0 {
		successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100
	}

	var endpointBreakdown map[string]EndpointModelStats
	if includeEndpoints {
		endpointBreakdown = a.buildEndpointBreakdown(name, modelEndpointStats)
	}

	return ModelStats{
		Name:               name,
		TotalRequests:      stats.TotalRequests,
		SuccessfulRequests: stats.SuccessfulRequests,
		FailedRequests:     stats.FailedRequests,
		SuccessRate:        format.Percentage(successRate),
		TotalTraffic:       format.Bytes(safeInt64ToUint64(stats.TotalBytes)),
		AverageLatency:     format.Latency(stats.AverageLatency),
		P95Latency:         format.Latency(stats.P95Latency),
		P99Latency:         format.Latency(stats.P99Latency),
		UniqueClients:      stats.UniqueClients,
		LastRequested:      format.TimeAgo(stats.LastRequested),
		RoutingHits:        stats.RoutingHits,
		RoutingMisses:      stats.RoutingMisses,
		RoutingFallbacks:   stats.RoutingFallbacks,
		EndpointBreakdown:  endpointBreakdown,
	}
}

func (a *Application) buildEndpointBreakdown(modelName string,
	modelEndpointStats map[string]map[string]ports.EndpointModelStats) map[string]EndpointModelStats {

	endpoints, ok := modelEndpointStats[modelName]
	if !ok {
		return nil
	}

	breakdown := make(map[string]EndpointModelStats)
	for epName, epStats := range endpoints {
		breakdown[epName] = EndpointModelStats{
			EndpointName:      epStats.EndpointName,
			RequestCount:      epStats.RequestCount,
			SuccessRate:       epStats.SuccessRate,
			AverageLatency:    format.Latency(epStats.AverageLatency),
			LastUsed:          format.TimeAgo(epStats.LastUsed),
			ConsecutiveErrors: epStats.ConsecutiveErrors,
		}
	}
	return breakdown
}

func (a *Application) buildSummary(models []ModelStats, modelStats map[string]ports.ModelStats,
	acc *modelStatsAccumulator) ModelStatsSummary {

	// Calculate active models (requested in last hour)
	activeModels := 0
	hourAgo := time.Now().Add(-time.Hour)
	for _, stats := range modelStats {
		if stats.LastRequested.After(hourAgo) {
			activeModels++
		}
	}

	overallSuccessRate := float64(0)
	if acc.totalRequests > 0 {
		overallSuccessRate = float64(acc.successfulRequests) / float64(acc.totalRequests) * 100
	}

	return ModelStatsSummary{
		TotalModels:        len(models),
		ActiveModels:       activeModels,
		TotalRequests:      acc.totalRequests,
		OverallSuccessRate: format.Percentage(overallSuccessRate),
		TotalTraffic:       format.Bytes(safeInt64ToUint64(acc.totalBytes)),
		MostPopularModel:   acc.mostPopular,
		LeastPopularModel:  acc.leastPopular,
	}
}

func (a *Application) buildEndpointSummary(modelEndpointStats map[string]map[string]ports.EndpointModelStats,
	includeSummary bool) map[string]EndpointModelStats {

	if !includeSummary {
		return make(map[string]EndpointModelStats)
	}

	// Aggregate endpoint stats across all models
	type endpointAggregate struct {
		lastUsed time.Time
		requests int64
		success  int64
		latency  int64
	}

	aggregates := make(map[string]*endpointAggregate)

	for _, modelEndpoints := range modelEndpointStats {
		for epName, epStats := range modelEndpoints {
			agg, exists := aggregates[epName]
			if !exists {
				agg = &endpointAggregate{}
				aggregates[epName] = agg
			}
			agg.requests += epStats.RequestCount
			agg.success += int64(epStats.SuccessRate * float64(epStats.RequestCount) / 100)
			agg.latency += epStats.AverageLatency * epStats.RequestCount
			if epStats.LastUsed.After(agg.lastUsed) {
				agg.lastUsed = epStats.LastUsed
			}
		}
	}

	endpoints := make(map[string]EndpointModelStats)
	for epName, agg := range aggregates {
		successRate := float64(0)
		avgLatency := int64(0)
		if agg.requests > 0 {
			successRate = float64(agg.success) / float64(agg.requests) * 100
			avgLatency = agg.latency / agg.requests
		}
		endpoints[epName] = EndpointModelStats{
			EndpointName:   epName,
			RequestCount:   agg.requests,
			SuccessRate:    successRate,
			AverageLatency: format.Latency(avgLatency),
			LastUsed:       format.TimeAgo(agg.lastUsed),
		}
	}

	return endpoints
}
