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
// with the types and zero-traffic conventions agreed in spec §4.4 / A3:
// min/max latency are plain int64 zeros, avg_latency_ms is omitted when there
// is no traffic, and absolute timestamps are RFC3339 alongside the relative
// strings.
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
