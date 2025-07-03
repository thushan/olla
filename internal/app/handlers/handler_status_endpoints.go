package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/thushan/olla/internal/core/ports"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/pkg/format"
)

const (
	maxEndpointsCapacity = 32 // sized for a typical deployment
)

type EndpointSummary struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	LastModelSync string `json:"last_model_sync,omitempty"`
	HealthCheck   string `json:"health_check"`
	ResponseTime  string `json:"response_time,omitempty"`
	SuccessRate   string `json:"success_rate"`
	Issues        string `json:"issues,omitempty"`
	Priority      int    `json:"priority"`
	ModelCount    int    `json:"model_count"`
	RequestCount  int64  `json:"request_count"`
}

type EndpointStatusResponse struct {
	Timestamp     time.Time         `json:"timestamp"`
	Endpoints     []EndpointSummary `json:"endpoints"`
	TotalCount    int               `json:"total_count"`
	HealthyCount  int               `json:"healthy_count"`
	RoutableCount int               `json:"routable_count"`
}

// we try and preallocate slices and buffers to avoid allocations
// especially in high-load scenarios where this endpoint is hit frequently
var (
	endpointSummaryPool = make([]EndpointSummary, 0, maxEndpointsCapacity)
	stringBuilderPool   = make([]byte, 0, 64) // For building issue strings
)

const (
	poolHealthHealthy = "healthy"
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

	endpointSummaryPool = endpointSummaryPool[:0]
	if cap(endpointSummaryPool) < len(allEndpoints) {
		endpointSummaryPool = make([]EndpointSummary, 0, len(allEndpoints))
	}

	for _, endpoint := range allEndpoints {
		summary := a.buildEndpointSummaryOptimised(endpoint, endpointStats, modelMap)
		endpointSummaryPool = append(endpointSummaryPool, summary)
	}

	sort.Slice(endpointSummaryPool, func(i, j int) bool {
		if endpointSummaryPool[i].Priority != endpointSummaryPool[j].Priority {
			return endpointSummaryPool[i].Priority > endpointSummaryPool[j].Priority
		}
		return endpointSummaryPool[i].Status == poolHealthHealthy && endpointSummaryPool[j].Status != poolHealthHealthy
	})

	// create a response with minimal allocs
	response := EndpointStatusResponse{
		Timestamp:     time.Now(),
		TotalCount:    len(allEndpoints),
		HealthyCount:  len(healthyEndpoints),
		RoutableCount: len(routableEndpoints),
		Endpoints:     endpointSummaryPool,
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (a *Application) buildEndpointSummaryOptimised(endpoint *domain.Endpoint, statsMap map[string]ports.EndpointStats, modelMap map[string]*domain.EndpointModels) EndpointSummary {
	url := endpoint.URLString
	stats, hasStats := statsMap[url]
	models := modelMap[url]

	summary := EndpointSummary{
		Name:     endpoint.Name,
		URL:      url,
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

	return summary
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
