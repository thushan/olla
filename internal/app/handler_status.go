package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type EndpointStatusResponse struct {
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	Priority            int       `json:"priority"`
	Status              string    `json:"status"`
	LastChecked         time.Time `json:"last_checked"`
	LastLatency         string    `json:"last_latency"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	BackoffMultiplier   int       `json:"backoff_multiplier"`
	NextCheckTime       time.Time `json:"next_check_time"`
}

// statusHandler handles endpoint status requests
func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	all, err := a.repository.GetAll(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	healthy, err := a.repository.GetRoutable(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get healthy endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	routable, err := a.repository.GetRoutable(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get routable endpoints: %v", err), http.StatusInternalServerError)
		return
	}

	status := make(map[string]interface{})
	status["total_endpoints"] = len(all)
	status["healthy_endpoints"] = len(healthy)
	status["routable_endpoints"] = len(routable)
	status["unhealthy_endpoints"] = len(all) - len(routable)

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
	status["endpoints"] = endpoints

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}
