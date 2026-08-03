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

// newSystemSummaryApp wires a minimal Application whose statsCollector returns
// the supplied ProxyStats, so buildSystemSummary exercises the new min/max
// rollup against known inputs rather than collector-internal computation.
func newSystemSummaryApp(t *testing.T, proxy ports.ProxyStats) *Application {
	t.Helper()
	endpoints := []*domain.Endpoint{
		{Name: "e1", Type: "ollama", URLString: "http://localhost:11434", Status: domain.StatusHealthy, Priority: 1},
	}
	return &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{proxyStats: proxy},
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      time.Now().Add(-time.Minute),
		Config:         &config.Config{Proxy: config.ProxyConfig{Engine: "olla", Profile: "balanced", LoadBalancer: "round_robin"}},
	}
}

// The fleet-wide min/max latency must render as plain int64 ms fields sourced
// from ports.ProxyStats, mirroring the per-endpoint convention. An idle fleet
// (zero ProxyStats) still serialises zeros because the fields are not omitempty.
func TestStatusHandler_SystemSummaryMinMaxLatency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		proxy ports.ProxyStats
		minMs int64
		maxMs int64
	}{
		{
			name:  "under traffic",
			proxy: ports.ProxyStats{TotalRequests: 10, MinLatency: 30, MaxLatency: 400, AverageLatency: 150},
			minMs: 30,
			maxMs: 400,
		},
		{
			name:  "idle fleet zeros",
			proxy: ports.ProxyStats{},
			minMs: 0,
			maxMs: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newSystemSummaryApp(t, tc.proxy)

			req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
			w := httptest.NewRecorder()
			app.statusHandler(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var raw map[string]interface{}
			require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
			sys, _ := raw["system"].(map[string]interface{})
			require.NotNil(t, sys)

			minLat, _ := sys["min_latency_ms"].(float64)
			maxLat, _ := sys["max_latency_ms"].(float64)
			assert.Equal(t, float64(tc.minMs), minLat, "min_latency_ms must match ProxyStats.MinLatency")
			assert.Equal(t, float64(tc.maxMs), maxLat, "max_latency_ms must match ProxyStats.MaxLatency")
		})
	}
}

// The min/max fields feed the ETag alongside the other System int64 fields, so
// moving them between two otherwise-identical responses must produce a fresh
// validator. This guards against an accidental regression where a new field is
// added to the payload but forgotten in hashStatusResponse.
func TestStatusResponse_ETagChangesOnMinMaxLatencyChange(t *testing.T) {
	t.Parallel()

	base := StatusResponse{
		Proxy: ProxySummary{Engine: "olla", Profile: "ollama", Balancer: "priority"},
		System: SystemSummary{
			StartTime:    time.Now().Add(-time.Minute),
			Status:       statusHealthy,
			EndpointsUp:  "1/1",
			SuccessRate:  "100.0%",
			AvgLatency:   "12ms",
			TotalTraffic: "1.5 MB",
			Version:      "0.0.29",
			Commit:       "abc",
			MinLatencyMs: 30,
			MaxLatencyMs: 400,
		},
	}

	r2 := base
	r2.System.MinLatencyMs = 45
	r2.System.MaxLatencyMs = 900

	assert.NotEqual(t, hashStatusResponse(&base), hashStatusResponse(&r2),
		"ETag must change when min/max latency change")
}

// Inverse of the change test: two responses with identical stable fields must
// hash to the same ETag even when the relative strings (Timestamp, UptimeHuman)
// move, so a dashboard polling an unchanging fleet gets a 304.
func TestStatusResponse_ETagStableWhenMinMaxLatencyIdentical(t *testing.T) {
	t.Parallel()

	now := time.Now()
	start := now.Add(-time.Minute)

	r1 := StatusResponse{
		Timestamp: now,
		Proxy:     ProxySummary{Engine: "olla", Profile: "ollama", Balancer: "priority"},
		System: SystemSummary{
			StartTime:    start,
			Status:       statusHealthy,
			EndpointsUp:  "1/1",
			SuccessRate:  "100.0%",
			AvgLatency:   "12ms",
			TotalTraffic: "1.5 MB",
			UptimeHuman:  "1m",
			Version:      "0.0.29",
			Commit:       "abc",
			MinLatencyMs: 30,
			MaxLatencyMs: 400,
		},
	}

	r2 := r1
	r2.Timestamp = now.Add(2 * time.Second)
	r2.System.UptimeHuman = "1m 2s"

	assert.Equal(t, hashStatusResponse(&r1), hashStatusResponse(&r2),
		"ETag must stay stable when only the wall-clock-rendered strings move")
}
