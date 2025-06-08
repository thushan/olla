package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
type ProxyStatusResponse struct {
	LoadBalancer       string `json:"load_balancer"`
	TotalRequests      int64  `json:"total_requests"`
	SuccessfulRequests int64  `json:"successful_requests"`
	FailedRequests     int64  `json:"failed_requests"`
	AverageLatency     int64  `json:"avg_latency_ms"`
	MinLatency         int64  `json:"min_latency_ms"`
	MaxLatency         int64  `json:"max_latency_ms"`
}

type SecurityStatsResponse struct {
	RateLimitViolations  int64 `json:"rate_limit_violations"`
	SizeLimitViolations  int64 `json:"size_limit_violations"`
	UniqueRateLimitedIPs int   `json:"unique_rate_limited_ips"`
}

type OllaStatusResponse struct {
	Endpoints          []EndpointStatusResponse `json:"endpoints"`
	Proxy              ProxyStatusResponse      `json:"proxy"`
	Security           SecurityStatsResponse    `json:"security"`
	TotalEndpoints     int                      `json:"total_endpoints"`
	HealthyEndpoints   int                      `json:"healthy_endpoints"`
	UnhealthyEndpoints int                      `json:"unhealthy_endpoints"`
	RoutableEndpoints  int                      `json:"routable_endpoints"`
}

// Update statusHandler to include endpoint and security stats
func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	all, err := a.repository.GetAll(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	healthy, err := a.repository.GetHealthy(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get healthy endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	routable, err := a.repository.GetRoutable(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get routable endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	statusResponse := OllaStatusResponse{
		TotalEndpoints:     len(all),
		HealthyEndpoints:   len(healthy),
		UnhealthyEndpoints: len(all) - len(routable),
		RoutableEndpoints:  len(routable),
	}

	// Build endpoint status (health check data)
	endpoints := make([]EndpointStatusResponse, len(all))
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
	statusResponse.Endpoints = endpoints

	// Get proxy stats from stats collector (maintains existing interface)
	proxyStats := a.statsCollector.GetProxyStats()
	statusResponse.Proxy = ProxyStatusResponse{
		LoadBalancer:       a.Config.Proxy.LoadBalancer,
		TotalRequests:      proxyStats.TotalRequests,
		SuccessfulRequests: proxyStats.SuccessfulRequests,
		FailedRequests:     proxyStats.FailedRequests,
		AverageLatency:     proxyStats.AverageLatency,
		MinLatency:         proxyStats.MinLatency,
		MaxLatency:         proxyStats.MaxLatency,
	}

	// Get security stats from stats collector
	securityStats := a.statsCollector.GetSecurityStats()
	statusResponse.Security = SecurityStatsResponse{
		RateLimitViolations:  securityStats.RateLimitViolations,
		SizeLimitViolations:  securityStats.SizeLimitViolations,
		UniqueRateLimitedIPs: securityStats.UniqueRateLimitedIPs,
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(statusResponse)
}
