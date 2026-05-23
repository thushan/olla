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

// CircuitBreakerState provides visibility into per-endpoint circuit breaker status
type CircuitBreakerState struct {
	State                string `json:"state"` // "closed", "open", or "half-open"
	LastTripTimestamp    *string `json:"last_trip_ts,omitempty"` // ISO-8601 or null if never tripped
	ConsecutiveFailures  int64  `json:"consecutive_failures"`
	CooldownRemainingSec int    `json:"cooldown_remaining_s"`
}

type EndpointSummary struct {
	Name            string               `json:"name"`
	Type            string               `json:"type"`
	Status          string               `json:"status"`
	LastModelSync   string               `json:"last_model_sync,omitempty"`
	HealthCheck     string               `json:"health_check"`
	ResponseTime    string               `json:"response_time,omitempty"`
	SuccessRate     string               `json:"success_rate"`
	Issues          string               `json:"issues,omitempty"`
	Priority        int                  `json:"priority"`
	ModelCount      int                  `json:"model_count"`
	RequestCount    int64                `json:"request_count"`
	CircuitBreaker  *CircuitBreakerState `json:"circuit_breaker,omitempty"`
}

type EndpointStatusResponse struct {
	Timestamp     time.Time         `json:"timestamp"`
	Endpoints     []EndpointSummary `json:"endpoints"`
	TotalCount    int               `json:"total_count"`
	HealthyCount  int               `json:"healthy_count"`
	RoutableCount int               `json:"routable_count"`
}

const (
	healthyStatus = "healthy"
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
		if stats.TotalRequests > 0 {
			successRate := (float64(stats.SuccessfulRequests) * 100.0) / float64(stats.TotalRequests)
			summary.SuccessRate = format.Percentage(successRate)
		} else {
			summary.SuccessRate = "N/A"
		}
	} else {
		summary.SuccessRate = "N/A"
	}

	summary.Issues = a.getEndpointIssuesSummaryOptimised(endpoint, stats, hasStats)

	// Populate circuit breaker state if available (Olla-specific feature)
	summary.CircuitBreaker = a.getCircuitBreakerState(endpoint.Name)

	return summary
}

// getCircuitBreakerState retrieves circuit breaker state from the proxy service if available.
// Returns nil if the proxy service does not support circuit breaker inspection (Olla-specific).
func (a *Application) getCircuitBreakerState(endpointName string) *CircuitBreakerState {
	// Type-assert to Olla service to access circuit breaker state (Olla-specific feature).
	// If the proxy service is not Olla, return nil (no circuit breaker state available).
	ollaService, ok := a.proxyService.(interface {
		GetCircuitBreakerState(endpointName string) interface{}
	})
	if !ok {
		return nil
	}

	stateMap := ollaService.GetCircuitBreakerState(endpointName)
	if stateMap == nil {
		return nil
	}

	// Marshal the map into the CircuitBreakerState struct
	data, _ := json.Marshal(stateMap)
	var cbs CircuitBreakerState
	if err := json.Unmarshal(data, &cbs); err != nil {
		return nil
	}
	return &cbs
}

func (a *Application) getEndpointIssuesSummaryOptimised(endpoint *domain.Endpoint, stats ports.EndpointStats, hasStats bool) string {
	if endpoint.Status == domain.StatusHealthy && endpoint.ConsecutiveFailures == 0 {
		return ""
	}

	if endpoint.Status == domain.StatusOffline || endpoint.Status == domain.StatusUnhealthy {
		return "unavailable"
	}

	if endpoint.ConsecutiveFailures > 3 {
		return "unstable"
	}

	if hasStats && stats.TotalRequests > 10 {
		if stats.SuccessfulRequests*100 < stats.TotalRequests*90 {
			return "low success rate"
		}
	}

	return ""
}
