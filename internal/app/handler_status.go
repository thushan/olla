package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/pkg/format"
	"net/http"
	"sort"
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

type EndpointStatusResponse struct {
	LastChecked         time.Time `json:"last_checked"`
	NextCheckTime       time.Time `json:"next_check_time"`
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	Status              string    `json:"status"`
	LastLatency         string    `json:"last_latency"`
	Priority            int       `json:"priority"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	BackoffMultiplier   int       `json:"backoff_multiplier"`
}

type EndpointStatsResponse struct {
	Name               string    `json:"name"`
	URL                string    `json:"url"`
	ActiveConnections  int64     `json:"active_connections"`
	TotalRequests      int64     `json:"total_requests"`
	SuccessfulRequests int64     `json:"successful_requests"`
	FailedRequests     int64     `json:"failed_requests"`
	TotalBytes         int64     `json:"total_bytes"`
	TotalBytesHuman    string    `json:"total_bytes_human"`
	AverageLatency     int64     `json:"avg_latency_ms"`
	MinLatency         int64     `json:"min_latency_ms"`
	MaxLatency         int64     `json:"max_latency_ms"`
	LastUsed           time.Time `json:"last_used"`
	SuccessRate        float64   `json:"success_rate_percent"`
	RequestsPerMinute  float64   `json:"requests_per_minute"`
	BytesPerRequest    int64     `json:"avg_bytes_per_request"`
}

type ProxyStatusResponse struct {
	LoadBalancer       string  `json:"load_balancer"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	AverageLatency     int64   `json:"avg_latency_ms"`
	MinLatency         int64   `json:"min_latency_ms"`
	MaxLatency         int64   `json:"max_latency_ms"`
	SuccessRate        float64 `json:"success_rate_percent"`
	ErrorRate          float64 `json:"error_rate_percent"`
}

type SecurityStatsResponse struct {
	RateLimitViolations  int64 `json:"rate_limit_violations"`
	SizeLimitViolations  int64 `json:"size_limit_violations"`
	UniqueRateLimitedIPs int   `json:"unique_rate_limited_ips"`
	TotalViolations      int64 `json:"total_violations"`
}

type ConnectionStatsResponse struct {
	TotalActiveConnections int64            `json:"total_active_connections"`
	ConnectionsByEndpoint  map[string]int64 `json:"connections_by_endpoint"`
	HighestConnections     string           `json:"highest_connections_endpoint"`
	LowestConnections      string           `json:"lowest_connections_endpoint"`
}

type SummaryStatsResponse struct {
	TotalEndpoints     int     `json:"total_endpoints"`
	HealthyEndpoints   int     `json:"healthy_endpoints"`
	UnhealthyEndpoints int     `json:"unhealthy_endpoints"`
	RoutableEndpoints  int     `json:"routable_endpoints"`
	MostActiveEndpoint string  `json:"most_active_endpoint"`
	HighestLatency     string  `json:"highest_latency_endpoint"`
	BestPerformance    string  `json:"best_performance_endpoint"`
	WorstPerformance   string  `json:"worst_performance_endpoint"`
	SystemSuccessRate  float64 `json:"system_success_rate_percent"`
	SystemErrorRate    float64 `json:"system_error_rate_percent"`
	TotalTrafficHuman  string  `json:"total_traffic_human"`
	UptimeSeconds      int64   `json:"uptime_seconds"`
}

type OllaStatusResponse struct {
	Summary       SummaryStatsResponse     `json:"summary"`
	Endpoints     []EndpointStatusResponse `json:"endpoints"`
	EndpointStats []EndpointStatsResponse  `json:"endpoint_stats"`
	Proxy         ProxyStatusResponse      `json:"proxy"`
	Security      SecurityStatsResponse    `json:"security"`
	Connections   ConnectionStatsResponse  `json:"connections"`
	Timestamp     time.Time                `json:"timestamp"`
}

// statusHandler provides comprehensive status - optimised for frequent access
func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all required data upfront
	all, healthy, routable, err := a.getEndpointCounts(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoint data: %v", err), http.StatusInternalServerError)
		return
	}

	// Get stats from collector in one go
	endpointStatsMap := a.statsCollector.GetEndpointStats()
	proxyStats := a.statsCollector.GetProxyStats()
	securityStats := a.statsCollector.GetSecurityStats()
	connectionStats := a.statsCollector.GetConnectionStats()

	// Build response with pre-allocated slices
	response := OllaStatusResponse{
		Timestamp:     time.Now(),
		Endpoints:     make([]EndpointStatusResponse, len(all)),
		EndpointStats: make([]EndpointStatsResponse, 0, len(endpointStatsMap)),
	}

	// Build endpoint health status
	a.buildEndpointStatus(all, response.Endpoints)

	// Build endpoint stats with analysis
	analysis := a.analyseEndpointStats(endpointStatsMap, &response.EndpointStats)

	// Build remaining response sections
	response.Proxy = a.buildProxyStats(proxyStats)
	response.Security = a.buildSecurityStats(securityStats)
	response.Connections = a.buildConnectionStats(connectionStats)
	response.Summary = a.buildSummaryStats(len(all), len(healthy), len(routable), analysis, response.Proxy)

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// getEndpointCounts efficiently gets all endpoint data in one pass
func (a *Application) getEndpointCounts(ctx context.Context) (all, healthy, routable []*domain.Endpoint, err error) {
	if all, err = a.repository.GetAll(ctx); err != nil {
		return
	}
	if healthy, err = a.repository.GetHealthy(ctx); err != nil {
		return
	}
	if routable, err = a.repository.GetRoutable(ctx); err != nil {
		return
	}
	return
}

// buildEndpointStatus populates endpoint health data efficiently
func (a *Application) buildEndpointStatus(all []*domain.Endpoint, endpoints []EndpointStatusResponse) {
	for i, endpoint := range all {
		endpoints[i] = EndpointStatusResponse{
			Name:                endpoint.Name,
			URL:                 endpoint.GetURLString(),
			Priority:            endpoint.Priority,
			Status:              endpoint.Status.String(),
			LastChecked:         endpoint.LastChecked,
			LastLatency:         endpoint.LastLatency.String(),
			ConsecutiveFailures: endpoint.ConsecutiveFailures,
			BackoffMultiplier:   endpoint.BackoffMultiplier,
			NextCheckTime:       endpoint.NextCheckTime,
		}
	}
}

// endpointAnalysis holds computed metrics for summary generation
type endpointAnalysis struct {
	totalSystemBytes         int64
	mostActiveEndpoint       string
	highestLatencyEndpoint   string
	bestPerformanceEndpoint  string
	worstPerformanceEndpoint string
}

// analyseEndpointStats processes endpoint stats and builds analysis
func (a *Application) analyseEndpointStats(statsMap map[string]ports.EndpointStats, endpointStats *[]EndpointStatsResponse) *endpointAnalysis {
	analysis := &endpointAnalysis{}
	var maxRequests, maxLatency int64
	var bestSuccessRate, worstSuccessRate float64 = -1, 101

	// Pre-allocate with known capacity
	*endpointStats = make([]EndpointStatsResponse, 0, len(statsMap))

	for _, stats := range statsMap {
		lastUsed := time.Unix(0, stats.LastUsedNano)
		// Calculate derived metrics
		requestsPerMinute := a.calculateRequestsPerMinute(lastUsed, stats.TotalRequests)
		bytesPerRequest := a.calculateBytesPerRequest(stats.TotalBytes, stats.TotalRequests)

		stat := EndpointStatsResponse{
			Name:               stats.Name,
			URL:                stats.URL,
			ActiveConnections:  stats.ActiveConnections,
			TotalRequests:      stats.TotalRequests,
			SuccessfulRequests: stats.SuccessfulRequests,
			FailedRequests:     stats.FailedRequests,
			TotalBytes:         stats.TotalBytes,
			TotalBytesHuman:    format.Bytes(uint64(stats.TotalBytes)),
			AverageLatency:     stats.AverageLatency,
			MinLatency:         stats.MinLatency,
			MaxLatency:         stats.MaxLatency,
			LastUsed:           lastUsed,
			SuccessRate:        stats.SuccessRate,
			RequestsPerMinute:  requestsPerMinute,
			BytesPerRequest:    bytesPerRequest,
		}
		*endpointStats = append(*endpointStats, stat)

		// Update analysis
		analysis.totalSystemBytes += stats.TotalBytes
		a.updateAnalysisMetrics(analysis, &stats, maxRequests, maxLatency, bestSuccessRate, worstSuccessRate, &maxRequests, &maxLatency, &bestSuccessRate, &worstSuccessRate)
	}

	// Sort by total requests (most active first)
	sort.Slice(*endpointStats, func(i, j int) bool {
		return (*endpointStats)[i].TotalRequests > (*endpointStats)[j].TotalRequests
	})

	return analysis
}

// calculateRequestsPerMinute efficiently calculates RPM
func (a *Application) calculateRequestsPerMinute(lastUsed time.Time, totalRequests int64) float64 {
	if lastUsed.IsZero() || totalRequests == 0 {
		return 0
	}
	minutesSinceLastUsed := time.Since(lastUsed).Minutes()
	if minutesSinceLastUsed <= 0 {
		return 0
	}
	return float64(totalRequests) / minutesSinceLastUsed
}

// calculateBytesPerRequest efficiently calculates average bytes
func (a *Application) calculateBytesPerRequest(totalBytes, totalRequests int64) int64 {
	if totalRequests == 0 {
		return 0
	}
	return totalBytes / totalRequests
}

// updateAnalysisMetrics updates tracking metrics efficiently
func (a *Application) updateAnalysisMetrics(analysis *endpointAnalysis, stats *ports.EndpointStats, maxRequests, maxLatency int64, bestSuccessRate, worstSuccessRate float64, maxRequestsPtr, maxLatencyPtr *int64, bestRatePtr, worstRatePtr *float64) {
	if stats.TotalRequests > maxRequests {
		*maxRequestsPtr = stats.TotalRequests
		analysis.mostActiveEndpoint = stats.Name
	}
	if stats.MaxLatency > maxLatency {
		*maxLatencyPtr = stats.MaxLatency
		analysis.highestLatencyEndpoint = stats.Name
	}
	if stats.SuccessRate > bestSuccessRate {
		*bestRatePtr = stats.SuccessRate
		analysis.bestPerformanceEndpoint = stats.Name
	}
	if stats.SuccessRate < worstSuccessRate && stats.TotalRequests > 0 {
		*worstRatePtr = stats.SuccessRate
		analysis.worstPerformanceEndpoint = stats.Name
	}
}

// buildProxyStats creates proxy response efficiently
func (a *Application) buildProxyStats(stats ports.ProxyStats) ProxyStatusResponse {
	var successRate, errorRate float64
	if stats.TotalRequests > 0 {
		successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100.0
		errorRate = float64(stats.FailedRequests) / float64(stats.TotalRequests) * 100.0
	}

	return ProxyStatusResponse{
		LoadBalancer:       a.Config.Proxy.LoadBalancer,
		TotalRequests:      stats.TotalRequests,
		SuccessfulRequests: stats.SuccessfulRequests,
		FailedRequests:     stats.FailedRequests,
		AverageLatency:     stats.AverageLatency,
		MinLatency:         stats.MinLatency,
		MaxLatency:         stats.MaxLatency,
		SuccessRate:        successRate,
		ErrorRate:          errorRate,
	}
}

// buildSecurityStats creates security response efficiently
func (a *Application) buildSecurityStats(stats ports.SecurityStats) SecurityStatsResponse {
	return SecurityStatsResponse{
		RateLimitViolations:  stats.RateLimitViolations,
		SizeLimitViolations:  stats.SizeLimitViolations,
		UniqueRateLimitedIPs: stats.UniqueRateLimitedIPs,
		TotalViolations:      stats.RateLimitViolations + stats.SizeLimitViolations,
	}
}

// buildConnectionStats creates connection response efficiently
func (a *Application) buildConnectionStats(connectionStats map[string]int64) ConnectionStatsResponse {
	var totalConnections int64
	var highestEndpoint, lowestEndpoint string
	var maxConnections, minConnections int64 = 0, -1

	for endpoint, connections := range connectionStats {
		totalConnections += connections
		if connections > maxConnections {
			maxConnections = connections
			highestEndpoint = endpoint
		}
		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			lowestEndpoint = endpoint
		}
	}

	return ConnectionStatsResponse{
		TotalActiveConnections: totalConnections,
		ConnectionsByEndpoint:  connectionStats,
		HighestConnections:     highestEndpoint,
		LowestConnections:      lowestEndpoint,
	}
}

// buildSummaryStats creates summary response efficiently
func (a *Application) buildSummaryStats(totalCount, healthyCount, routableCount int, analysis *endpointAnalysis, proxy ProxyStatusResponse) SummaryStatsResponse {
	uptime := time.Since(a.StartTime).Seconds()

	return SummaryStatsResponse{
		TotalEndpoints:     totalCount,
		HealthyEndpoints:   healthyCount,
		UnhealthyEndpoints: totalCount - routableCount,
		RoutableEndpoints:  routableCount,
		MostActiveEndpoint: analysis.mostActiveEndpoint,
		HighestLatency:     analysis.highestLatencyEndpoint,
		BestPerformance:    analysis.bestPerformanceEndpoint,
		WorstPerformance:   analysis.worstPerformanceEndpoint,
		SystemSuccessRate:  proxy.SuccessRate,
		SystemErrorRate:    proxy.ErrorRate,
		TotalTrafficHuman:  format.Bytes(uint64(analysis.totalSystemBytes)),
		UptimeSeconds:      int64(uptime),
	}
}
