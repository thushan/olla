package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/ports"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/pkg/format"
)

type EndpointSummary struct {
	Name               string                      `json:"name"`
	Type               string                      `json:"type"`
	Status             string                      `json:"status"`
	LastModelSync      string                      `json:"last_model_sync,omitempty"`
	HealthCheck        string                      `json:"health_check"`
	ResponseTime       string                      `json:"response_time,omitempty"`
	SuccessRate        string                      `json:"success_rate"`
	DegradationReason  string                      `json:"degradation_reason,omitempty"`
	Issues             string                      `json:"issues,omitempty"`
	Priority           int                         `json:"priority"`
	ModelCount         int                         `json:"model_count"`
	RequestCount       int64                       `json:"request_count"`
	SuccessRatePercent float64                     `json:"success_rate_percent,omitempty"`
	Degraded           bool                        `json:"degraded"`
	CircuitBreaker     *domain.CircuitBreakerState `json:"circuit_breaker,omitempty"`
}

type EndpointStatusResponse struct {
	Timestamp     time.Time         `json:"timestamp"`
	Endpoints     []EndpointSummary `json:"endpoints"`
	TotalCount    int               `json:"total_count"`
	HealthyCount  int               `json:"healthy_count"`
	RoutableCount int               `json:"routable_count"`
}

const (
	healthyStatus                          = "healthy"
	circuitOpenStatus                      = "circuit_open"
	circuitOpenDegradationReason           = "circuit breaker open"
	degradedSuccessRateThresholdPercent    = 80.0
	degradedSuccessRateMinimumRequestCount = int64(10)
)

func (a *Application) endpointsStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get everyone all at once, then deal with it ted.
	allEndpoints, healthyEndpoints, routableEndpoints, err := a.getEndpointCounts(ctx)
	if err != nil {
		http.Error(w, "Failed to get endpoint data", http.StatusInternalServerError)
		return
	}

	endpointStats := a.statsCollector.GetEndpointStats()
	modelMap, _ := a.modelRegistry.GetEndpointModelMap(ctx)
	summaries := make([]EndpointSummary, 0, len(allEndpoints))

	for _, endpoint := range allEndpoints {
		summary := a.buildEndpointSummaryOptimised(endpoint, endpointStats, modelMap)
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Priority != summaries[j].Priority {
			return summaries[i].Priority > summaries[j].Priority
		}
		return summaries[i].Status == healthyStatus && summaries[j].Status != healthyStatus
	})

	// create a response with minimal mallocs
	response := EndpointStatusResponse{
		Timestamp:     time.Now(),
		TotalCount:    len(allEndpoints),
		HealthyCount:  len(healthyEndpoints),
		RoutableCount: len(routableEndpoints),
		Endpoints:     summaries,
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (a *Application) buildEndpointSummaryOptimised(endpoint *domain.Endpoint, statsMap map[string]ports.EndpointStats, modelMap map[string]*domain.EndpointModels) EndpointSummary {
	url := endpoint.URLString
	stats, hasStats := statsMap[url]
	models := modelMap[url]

	summary := EndpointSummary{
		Name:     endpoint.Name,
		Type:     endpoint.Type,
		Status:   endpoint.Status.String(),
		Priority: endpoint.Priority,
	}

	if models != nil {
		summary.ModelCount = len(models.Models)
		if !models.LastUpdated.IsZero() {
			summary.LastModelSync = format.TimeAgo(models.LastUpdated)
		}
	}

	if !endpoint.LastChecked.IsZero() {
		summary.HealthCheck = format.TimeAgo(endpoint.LastChecked)
		if endpoint.LastLatency > 0 {
			summary.ResponseTime = format.Latency(endpoint.LastLatency.Milliseconds())
		}
	}

	if hasStats {
		summary.RequestCount = stats.TotalRequests
		if successRate, ok := endpointSuccessRatePercent(stats, hasStats); ok {
			summary.SuccessRatePercent = successRate
			summary.SuccessRate = format.Percentage(successRate)
		} else {
			summary.SuccessRate = "N/A"
		}
	} else {
		summary.SuccessRate = "N/A"
	}

	summary.Issues = a.getEndpointIssuesSummaryOptimised(endpoint, stats, hasStats)
	if a.endpointSuccessRateDegraded(stats, hasStats) {
		summary.Degraded = true
		summary.DegradationReason = "low success rate"
	}
	if circuitBreaker := a.getCircuitBreakerState(endpoint); circuitBreaker != nil {
		summary.CircuitBreaker = circuitBreaker
		if isCircuitBreakerOpen(circuitBreaker) {
			summary.Status = circuitOpenStatus
			summary.Degraded = true
			summary.DegradationReason = circuitOpenDegradationReason
			summary.Issues = appendEndpointIssue(summary.Issues, circuitOpenDegradationReason)
		}
	}

	return summary
}

func (a *Application) getCircuitBreakerState(endpoint *domain.Endpoint) *domain.CircuitBreakerState {
	if a.proxyService == nil {
		return nil
	}
	breakerStateProvider, ok := a.proxyService.(interface {
		GetCircuitBreakerState(endpoint *domain.Endpoint) domain.CircuitBreakerState
	})
	if !ok {
		return nil
	}

	state := breakerStateProvider.GetCircuitBreakerState(endpoint)
	return &state
}

func isCircuitBreakerOpen(state *domain.CircuitBreakerState) bool {
	return state != nil && state.State == "open"
}

func appendEndpointIssue(existing, issue string) string {
	if existing == "" {
		return issue
	}
	if existing == issue {
		return existing
	}
	return existing + "; " + issue
}

func (a *Application) getEndpointIssuesSummaryOptimised(endpoint *domain.Endpoint, stats ports.EndpointStats, hasStats bool) string {
	if endpoint.Status == domain.StatusOffline || endpoint.Status == domain.StatusUnhealthy {
		return "unavailable"
	}

	if endpoint.ConsecutiveFailures > 3 {
		return "unstable"
	}

	if a.endpointSuccessRateDegraded(stats, hasStats) {
		return "low success rate"
	}

	if endpoint.Status == domain.StatusHealthy && endpoint.ConsecutiveFailures == 0 {
		return ""
	}

	return ""
}

func endpointSuccessRatePercent(stats ports.EndpointStats, hasStats bool) (float64, bool) {
	if !hasStats || stats.TotalRequests == 0 {
		return 0, false
	}

	return float64(stats.SuccessfulRequests) * 100.0 / float64(stats.TotalRequests), true
}

func (a *Application) endpointSuccessRateDegraded(stats ports.EndpointStats, hasStats bool) bool {
	successRate, ok := endpointSuccessRatePercent(stats, hasStats)
	threshold, minRequests := a.endpointSuccessRateDegradationConfig()
	if !ok || stats.TotalRequests < minRequests {
		return false
	}

	return successRate < threshold
}

func (a *Application) endpointSuccessRateDegradationConfig() (float64, int64) {
	threshold := degradedSuccessRateThresholdPercent
	minRequests := degradedSuccessRateMinimumRequestCount
	if a.Config == nil {
		return threshold, minRequests
	}

	if configured := a.Config.Engineering.EndpointDegradedSuccessRateThreshold; configured > 0 {
		threshold = configured
	}
	if configured := a.Config.Engineering.EndpointDegradedMinimumRequests; configured > 0 {
		minRequests = configured
	}

	return threshold, minRequests
}
