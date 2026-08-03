package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Asserts the additive dashboard fields render on /internal/status/endpoints
// with the agreed types and zero-traffic conventions: min/max latency are
// plain int64 zeros, avg_latency_ms is omitted when there is no traffic, and
// absolute timestamps are RFC3339 alongside the relative strings.
func TestEndpointsStatusHandler_AdditiveFields(t *testing.T) {
	const trafficURL = "http://localhost:11434"
	const idleURL = "http://localhost:8080"

	endpoints := []*domain.Endpoint{
		{
			Name:          "busy",
			Type:          "ollama",
			URLString:     trafficURL,
			Status:        domain.StatusHealthy,
			Priority:      1,
			LastChecked:   time.Now().Add(-15 * time.Second),
			NextCheckTime: time.Now().Add(45 * time.Second),
		},
		{
			Name:          "idle",
			Type:          "openai",
			URLString:     idleURL,
			Status:        domain.StatusHealthy,
			Priority:      2,
			LastChecked:   time.Now().Add(-15 * time.Second),
			NextCheckTime: time.Now().Add(45 * time.Second),
		},
	}

	stats := &mockStatusStatsCollector{
		endpointStats: map[string]ports.EndpointStats{
			trafficURL: {
				TotalRequests:      10,
				SuccessfulRequests: 9,
				MinLatency:         30,
				MaxLatency:         400,
				AverageLatency:     150,
			},
		},
		connectionStats: map[string]int64{
			trafficURL: 2,
			idleURL:    0,
		},
	}

	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: stats,
		modelRegistry:  &mockStatusModelRegistry{},
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()
	app.endpointsStatusHandler(w, req)

	var resp EndpointStatusResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Endpoints, 2)

	byName := map[string]EndpointSummary{}
	for _, e := range resp.Endpoints {
		byName[e.Name] = e
	}

	busy := byName["busy"]
	assert.Equal(t, int64(30), busy.MinLatencyMs)
	assert.Equal(t, int64(400), busy.MaxLatencyMs)
	require.NotNil(t, busy.AvgLatencyMs, "avg_latency_ms must be present for traffic endpoint")
	assert.Equal(t, int64(150), *busy.AvgLatencyMs)
	assert.Equal(t, int64(2), busy.ActiveConnections)
	assert.Equal(t, "http://localhost:11434", busy.URL)
	assert.NotNil(t, busy.HealthCheckAt)
	assert.NotNil(t, busy.NextCheckAt)

	idle := byName["idle"]
	assert.Equal(t, int64(0), idle.MinLatencyMs, "min_latency_ms zero-value for no traffic (A3 convention)")
	assert.Equal(t, int64(0), idle.MaxLatencyMs, "max_latency_ms zero-value for no traffic (A3 convention)")
	assert.Nil(t, idle.AvgLatencyMs, "avg_latency_ms must be omitted for no traffic")
	assert.Equal(t, int64(0), idle.ActiveConnections)
}

// The sanitised URL field must not echo userinfo or query strings even when
// the configured endpoint URL contained them (defence in depth alongside the
// config-time rejection in discovery.validateEndpointConfig).
func TestEndpointsStatusHandler_URLSanitised(t *testing.T) {
	const dirtyURL = "https://alice:s3cr3t@ollama.local:11434/v1?token=abc"
	endpoints := []*domain.Endpoint{
		{
			Name: "ollama-creds", Type: "ollama", URLString: dirtyURL,
			Status: domain.StatusHealthy, Priority: 1,
		},
	}
	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  &mockStatusModelRegistry{},
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()
	app.endpointsStatusHandler(w, req)

	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	endpointsArr, _ := raw["endpoints"].([]interface{})
	require.Len(t, endpointsArr, 1)
	first, _ := endpointsArr[0].(map[string]interface{})
	urlField, _ := first["url"].(string)
	assert.Equal(t, "https://ollama.local:11434/v1", urlField)
	assert.NotContains(t, urlField, "alice")
	assert.NotContains(t, urlField, "s3cr3t")
	assert.NotContains(t, urlField, "token")
}

// start_time on SystemSummary is RFC3339 and non-omitted.
func TestStatusHandler_StartTimeAbsolute(t *testing.T) {
	start := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	endpoints := []*domain.Endpoint{
		{Name: "e1", Type: "ollama", URLString: "http://localhost:11434", Status: domain.StatusHealthy, Priority: 1},
	}
	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      start,
		Config:         &config.Config{Proxy: config.ProxyConfig{Engine: "olla", Profile: "balanced", LoadBalancer: "round_robin"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w := httptest.NewRecorder()
	app.statusHandler(w, req)

	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	sys, _ := raw["system"].(map[string]interface{})
	require.NotNil(t, sys)
	startTimeStr, _ := sys["start_time"].(string)
	assert.Equal(t, start.Format(time.RFC3339), startTimeStr)
	_, err := time.Parse(time.RFC3339, startTimeStr)
	assert.NoError(t, err)
}

// buildSystemStatusApp is a minimal Application wired only for the
// /internal/status system summary, parameterised by endpoint health and
// proxy traffic so the status-semantics tests below can sweep combinations
// without rebuilding the whole seed each time.
func buildSystemStatusApp(t *testing.T, endpoints []*domain.Endpoint, proxyStats ports.ProxyStats) *Application {
	t.Helper()
	return &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{proxyStats: proxyStats},
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      time.Now(),
		Config:         &config.Config{Proxy: config.ProxyConfig{Engine: "olla", Profile: "balanced", LoadBalancer: "round_robin"}},
	}
}

func allHealthyEndpoints() []*domain.Endpoint {
	return []*domain.Endpoint{
		{Name: "a", Type: "ollama", URLString: "http://a:11434", Status: domain.StatusHealthy, Priority: 1},
		{Name: "b", Type: "ollama", URLString: "http://b:11434", Status: domain.StatusHealthy, Priority: 1},
	}
}

// Regression: a fresh boot with every endpoint healthy and zero proxy
// requests previously reported status "critical" (and success_rate "0%")
// because the legacy < 90.0 success-rate clause fired on the no-traffic
// zero. Status must now derive from endpoint health alone, success_rate
// must be the explicit no-data value, and has_traffic must be false.
func TestStatusHandler_FreshBootAllHealthy_NotCritical(t *testing.T) {
	app := buildSystemStatusApp(t, allHealthyEndpoints(), ports.ProxyStats{})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w := httptest.NewRecorder()
	app.statusHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	sys, _ := raw["system"].(map[string]interface{})
	require.NotNil(t, sys)

	assert.Equal(t, statusHealthy, sys["status"], "fresh boot with all endpoints healthy must not report critical")
	assert.Equal(t, "N/A", sys["success_rate"], "success_rate must be the no-data value when there is no traffic")
	hasTraffic, ok := sys["has_traffic"].(bool)
	require.True(t, ok, "has_traffic must be present as a bool")
	assert.False(t, hasTraffic, "has_traffic must be false on zero traffic")
}

// Partial-health fleet with no traffic derives status from endpoint health
// alone: below 50% healthy is critical, between 50% and 80% is degraded.
// The absent success rate must not pull a partially-healthy fleet into a
// harsher band than the traffic-present path would.
func TestStatusHandler_PartialHealthZeroTraffic_StatusFromHealth(t *testing.T) {
	endpoints := []*domain.Endpoint{
		{Name: "up", Type: "ollama", URLString: "http://up:11434", Status: domain.StatusHealthy, Priority: 1},
		{Name: "down1", Type: "ollama", URLString: "http://d1:11434", Status: domain.StatusUnhealthy, Priority: 1},
		{Name: "down2", Type: "ollama", URLString: "http://d2:11434", Status: domain.StatusUnhealthy, Priority: 1},
	}
	app := buildSystemStatusApp(t, endpoints, ports.ProxyStats{})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w := httptest.NewRecorder()
	app.statusHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	sys, _ := raw["system"].(map[string]interface{})
	require.NotNil(t, sys)

	assert.Equal(t, statusCritical, sys["status"], "1/3 healthy is below 0.5 so status comes from health alone")
	assert.Equal(t, "N/A", sys["success_rate"])
	assert.False(t, sys["has_traffic"].(bool))
}

// Traffic-present path keeps its dual thresholds: an all-healthy fleet with
// a high failure rate still lands on critical, proving the no-traffic
// shortcut did not silently weaken the existing verdict.
func TestStatusHandler_TrafficHighFailureRate_CriticalDespiteHealthyEndpoints(t *testing.T) {
	app := buildSystemStatusApp(t, allHealthyEndpoints(), ports.ProxyStats{
		TotalRequests:      100,
		SuccessfulRequests: 80,
		FailedRequests:     20,
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w := httptest.NewRecorder()
	app.statusHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	sys, _ := raw["system"].(map[string]interface{})
	require.NotNil(t, sys)

	assert.Equal(t, statusCritical, sys["status"], "80% success rate is below the 90% threshold even with all endpoints healthy")
	assert.NotEqual(t, "N/A", sys["success_rate"], "success_rate must reflect real traffic, not the no-data sentinel")
	assert.True(t, sys["has_traffic"].(bool))
}

// C5 mirror: the /internal/status comparator in buildUnifiedEndpoints has
// the same missing tie-breaker as the /internal/status/endpoints one.
// Equal-priority same-health endpoints must order by name (then URL) so
// polls are stable regardless of map-iteration order. Input is reverse-
// alphabetical by name; without the tie-breaker the current comparator
// leaves it in input order, so the alphabetical assertion fails until the
// tie-breaker is appended.
func TestBuildUnifiedEndpoints_TieBreakerStableOrder(t *testing.T) {
	endpoints := []*domain.Endpoint{
		{Name: "zebra", URLString: "http://z:11434", Status: domain.StatusHealthy, Priority: 5},
		{Name: "mango", URLString: "http://m:11434", Status: domain.StatusHealthy, Priority: 5},
		{Name: "alpha", URLString: "http://a:11434", Status: domain.StatusHealthy, Priority: 5},
	}

	app := createTestStatusApplication(endpoints)
	out := make([]EndpointResponse, len(endpoints))
	app.buildUnifiedEndpoints(endpoints, nil, nil, out, nil, buildEndpointIDs(endpoints))

	require.Len(t, out, 3)
	assert.Equal(t, "alpha", out[0].Name, "tie-breaker must sort equal-priority same-health endpoints by name")
	assert.Equal(t, "mango", out[1].Name)
	assert.Equal(t, "zebra", out[2].Name)
}

// Regression for the B2/C1 conflict: two endpoints differing only by query
// string sanitise to an identical display URL, so a URL-based tie-breaker
// ties completely and ordering falls back to map-iteration order, which is
// unstable across polls. The ID tie-breaker is derived from the raw
// pre-sanitisation URL, so it must still order deterministically.
func TestBuildUnifiedEndpoints_TieBreakerDeterministicOnCollidingDisplayURL(t *testing.T) {
	a := &domain.Endpoint{Name: "twin", URLString: "http://host:11434?v=a", Status: domain.StatusHealthy, Priority: 5}
	b := &domain.Endpoint{Name: "twin", URLString: "http://host:11434?v=b", Status: domain.StatusHealthy, Priority: 5}

	app := createTestStatusApplication([]*domain.Endpoint{a, b})

	// The repository's endpoint map is keyed on the raw URL (B2), so a and b
	// are genuinely distinct entries; the slice arriving here comes from map
	// iteration and its order is not guaranteed to be the same across polls.
	// A correct tie-breaker must resolve to the same relative order of a and
	// b regardless of which order they arrive in - that's what "stable
	// across polls" means. Feed both input orders and compare the winner by
	// identity (URLString), not by output index.
	orderings := [][]*domain.Endpoint{{a, b}, {b, a}}

	var firstWinner string
	for i, endpoints := range orderings {
		out := make([]EndpointResponse, len(endpoints))
		app.buildUnifiedEndpoints(endpoints, nil, nil, out, nil, buildEndpointIDs(endpoints))

		require.Len(t, out, 2)
		assert.NotEqual(t, out[0].ID, out[1].ID)
		assert.Equal(t, out[0].URL, out[1].URL, "test setup: display URLs should collide after sanitisation")

		winner := out[0].ID
		if i == 0 {
			firstWinner = winner
		} else {
			assert.Equal(t, firstWinner, winner, "the same endpoint must sort first regardless of input order (map-iteration order varies across polls)")
		}
	}
}
