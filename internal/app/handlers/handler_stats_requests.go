package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/thushan/olla/internal/adapter/metrics"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
)

// RequestStatsResponse wraps recent request metrics for the API.
type RequestStatsResponse struct {
	Timestamp time.Time                    `json:"timestamp"`
	Requests  []ports.RequestMetricsEvent  `json:"requests"`
	Count     int                          `json:"count"`
}

// RequestSummaryResponse wraps aggregated stats for the API.
type RequestSummaryResponse struct {
	Timestamp time.Time                 `json:"timestamp"`
	Stats     *metrics.AggregatedStats  `json:"stats"`
}

// recentRequestsHandler returns the last N request metrics.
// Query params: ?limit=50 (default 50, max 1000)
func (a *Application) recentRequestsHandler(w http.ResponseWriter, r *http.Request) {
	if a.metricsCollector == nil {
		http.Error(w, "Metrics collector not initialised", http.StatusServiceUnavailable)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > 1000 {
				limit = 1000
			}
		}
	}

	recent := a.metricsCollector.GetRecentRequests(limit)
	response := RequestStatsResponse{
		Timestamp: time.Now(),
		Requests:  recent,
		Count:     len(recent),
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Error("Failed to encode request stats response", "error", err)
	}
}

// requestSummaryHandler returns aggregated statistics.
// Query params: ?since=5m (time window, default all)
func (a *Application) requestSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if a.metricsCollector == nil {
		http.Error(w, "Metrics collector not initialised", http.StatusServiceUnavailable)
		return
	}

	var since time.Time
	if v := r.URL.Query().Get("since"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			since = time.Now().Add(-d)
		}
	}

	stats := a.metricsCollector.GetAggregatedStats(since)
	response := RequestSummaryResponse{
		Timestamp: time.Now(),
		Stats:     stats,
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Error("Failed to encode request summary response", "error", err)
	}
}
