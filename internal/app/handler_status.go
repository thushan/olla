package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

var (
	statusHealthy  = "healthy"
	statusDegraded = "degraded"
	statusCritical = "critical"
	statusNormal   = "normal"
	statusElevated = "elevated"
	zeroTraffic    = "0 B"
	emptyString    = ""
)

type SystemSummary struct {
	Status             string `json:"status"`
	EndpointsUp        string `json:"endpoints_up"`
	SuccessRate        string `json:"success_rate"`
	AvgLatency         string `json:"avg_latency"`
	ActiveConnections  int64  `json:"active_connections"`
	SecurityViolations int64  `json:"security_violations"`
	TotalTraffic       string `json:"total_traffic"`
	TotalRequests      int64  `json:"total_requests"`
	TotalFailures      int64  `json:"total_failures"`
	UptimeHuman        string `json:"uptime"`
}

type EndpointResponse struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	Connections int64  `json:"connections"`
	Requests    int64  `json:"requests"`
	SuccessRate string `json:"success_rate"`
	AvgLatency  string `json:"avg_latency"`
	Traffic     string `json:"traffic"`
	LastCheck   string `json:"last_check"`
	NextCheck   string `json:"next_check"`
	Issues      string `json:"issues"`
}

type SecuritySummary struct {
	RateLimits int64  `json:"rate_limits"`
	SizeLimits int64  `json:"size_limits"`
	BlockedIPs int    `json:"blocked_ips"`
	Status     string `json:"status"`
}

type StatusResponse struct {
	System    SystemSummary      `json:"system"`
	Endpoints []EndpointResponse `json:"endpoints"`
	Security  SecuritySummary    `json:"security"`
	Timestamp time.Time          `json:"timestamp"`
}

var issuesPool = make([]string, 0, 4)

func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	all, healthy, routable, err := a.getEndpointCounts(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoint data: %v", err), http.StatusInternalServerError)
		return
	}

	endpointStatsMap := a.statsCollector.GetEndpointStats()
	proxyStats := a.statsCollector.GetProxyStats()
	securityStats := a.statsCollector.GetSecurityStats()
	connectionStats := a.statsCollector.GetConnectionStats()

	response := StatusResponse{
		Timestamp: now,
		Endpoints: make([]EndpointResponse, len(all)),
	}

	response.System = a.buildSystemSummary(all, healthy, routable, proxyStats, securityStats, connectionStats, endpointStatsMap)
	a.buildUnifiedEndpoints(all, endpointStatsMap, connectionStats, response.Endpoints)
	response.Security = a.buildSecuritySummary(securityStats)

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *Application) buildSystemSummary(all, healthy, routable []*domain.Endpoint, proxy ports.ProxyStats, security ports.SecurityStats, connections map[string]int64, endpointStats map[string]ports.EndpointStats) SystemSummary {
	var totalConnections, totalTraffic int64

	for url, conn := range connections {
		totalConnections += conn
		if stats, exists := endpointStats[url]; exists {
			totalTraffic += stats.TotalBytes
		}
	}

	// ratios
	healthyRatio := float64(len(healthy)) / float64(len(all))
	var systemSuccessRate float64
	if proxy.TotalRequests > 0 {
		systemSuccessRate = float64(proxy.SuccessfulRequests) / float64(proxy.TotalRequests) * 100.0
	}

	var status string
	if healthyRatio < 0.5 || systemSuccessRate < 90.0 {
		status = statusCritical
	} else if healthyRatio < 0.8 || systemSuccessRate < 95.0 {
		status = statusDegraded
	} else {
		status = statusHealthy
	}

	totalViolations := security.RateLimitViolations + security.SizeLimitViolations

	return SystemSummary{
		Status:             status,
		EndpointsUp:        format.EndpointsUp(len(healthy), len(all)),
		SuccessRate:        format.Percentage(systemSuccessRate),
		AvgLatency:         format.Latency(proxy.AverageLatency),
		ActiveConnections:  totalConnections,
		SecurityViolations: totalViolations,
		TotalTraffic:       format.Bytes(uint64(totalTraffic)),
		TotalRequests:      proxy.TotalRequests,
		TotalFailures:      proxy.FailedRequests,
		UptimeHuman:        format.Duration2(time.Since(a.StartTime)),
	}
}

func (a *Application) buildUnifiedEndpoints(all []*domain.Endpoint, statsMap map[string]ports.EndpointStats, connectionStats map[string]int64, endpoints []EndpointResponse) {
	for i, endpoint := range all {
		url := endpoint.GetURLString()
		stats, hasStats := statsMap[url]
		connections := connectionStats[url]

		var successRate float64
		if hasStats && stats.TotalRequests > 0 {
			successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100.0
		}

		traffic := zeroTraffic
		requests := int64(0)
		avgLatency := int64(0)
		if hasStats {
			traffic = format.Bytes(uint64(stats.TotalBytes))
			requests = stats.TotalRequests
			avgLatency = stats.AverageLatency
		}

		endpoints[i] = EndpointResponse{
			Name:        endpoint.Name,
			Status:      endpoint.Status.String(),
			Priority:    endpoint.Priority,
			Connections: connections,
			Requests:    requests,
			SuccessRate: format.Percentage(successRate),
			AvgLatency:  format.Latency(avgLatency),
			Traffic:     traffic,
			LastCheck:   format.TimeAgo(endpoint.LastChecked),
			NextCheck:   format.TimeUntil(endpoint.NextCheckTime),
			Issues:      a.getEndpointIssues(endpoint, stats, hasStats, successRate),
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Priority != endpoints[j].Priority {
			return endpoints[i].Priority > endpoints[j].Priority
		}
		return endpoints[i].Status == statusHealthy && endpoints[j].Status != statusHealthy
	})
}

func (a *Application) buildSecuritySummary(stats ports.SecurityStats) SecuritySummary {
	var status string
	totalViolations := stats.RateLimitViolations + stats.SizeLimitViolations

	if totalViolations > 100 || stats.UniqueRateLimitedIPs > 10 {
		status = statusElevated
	} else {
		status = statusNormal
	}

	return SecuritySummary{
		RateLimits: stats.RateLimitViolations,
		SizeLimits: stats.SizeLimitViolations,
		BlockedIPs: stats.UniqueRateLimitedIPs,
		Status:     status,
	}
}

func (a *Application) getEndpointIssues(endpoint *domain.Endpoint, stats ports.EndpointStats, hasStats bool, successRate float64) string {
	issuesPool = issuesPool[:0]

	if endpoint.ConsecutiveFailures > 3 {
		issuesPool = append(issuesPool, "consecutive failures")
	}

	if hasStats {
		if successRate < 90.0 && stats.TotalRequests > 10 {
			issuesPool = append(issuesPool, "low success rate")
		}
		if stats.AverageLatency > 5000 {
			issuesPool = append(issuesPool, "high latency")
		}
	}

	if endpoint.Status == domain.StatusOffline || endpoint.Status == domain.StatusUnhealthy {
		issuesPool = append(issuesPool, "unavailable")
	}

	if len(issuesPool) == 0 {
		return emptyString
	}

	return strings.Join(issuesPool, ", ")
}

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
