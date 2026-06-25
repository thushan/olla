package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestMetricsHandler_BasicFunctionality(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		{
			Name:      "test-endpoint-healthy",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  100,
		},
		{
			Name:      "test-endpoint-unhealthy",
			Type:      "openai",
			URLString: "http://localhost:8080",
			Status:    domain.StatusUnhealthy,
			Priority:  50,
		},
	}

	stats := &mockStatusStatsCollector{
		endpointStats: map[string]ports.EndpointStats{
			"http://localhost:11434": {
				TotalRequests:      120,
				SuccessfulRequests: 118,
				FailedRequests:     2,
				TotalBytes:         4096,
				AverageLatency:     125,
			},
		},
		proxyStats: ports.ProxyStats{
			TotalRequests:      120,
			SuccessfulRequests: 118,
			FailedRequests:     2,
			AverageLatency:     125,
		},
	}

	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: stats,
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      time.Now().Add(-2 * time.Hour),
		Config: &config.Config{
			Proxy: config.ProxyConfig{
				Engine:       "olla",
				Profile:      "auto",
				LoadBalancer: "priority",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, constants.DefaultMetricsEndpoint, nil)
	w := httptest.NewRecorder()

	app.metricsHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, constants.ContentTypePrometheus, w.Header().Get(constants.HeaderContentType))

	body := w.Body.String()
	assert.Contains(t, body, "olla_requests_total")
	assert.Contains(t, body, "olla_failures_total")
	assert.Contains(t, body, "olla_endpoints_total 2")
	assert.Contains(t, body, "olla_endpoints_healthy 1")
	assert.Contains(t, body, `olla_endpoint_up{endpoint="test-endpoint-healthy",status="healthy"} 1`)
	assert.Contains(t, body, `olla_endpoint_up{endpoint="test-endpoint-unhealthy",status="unhealthy"} 0`)
	assert.Contains(t, body, `engine="olla"`)
	assert.Contains(t, body, `balancer="priority"`)
}

func TestMetricsHandler_MatchesStatusCounts(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		{
			Name:      "ep-a",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  1,
		},
	}

	stats := &mockStatusStatsCollector{
		endpointStats: map[string]ports.EndpointStats{
			"http://localhost:11434": {
				TotalRequests:      10,
				SuccessfulRequests: 9,
				FailedRequests:     1,
				TotalBytes:         1000,
				AverageLatency:     50,
			},
		},
		proxyStats: ports.ProxyStats{
			TotalRequests:      10,
			SuccessfulRequests: 9,
			FailedRequests:     1,
			AverageLatency:     50,
		},
	}

	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: stats,
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      time.Now(),
		Config:         &config.Config{},
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	statusRec := httptest.NewRecorder()
	app.statusHandler(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code)

	metricsReq := httptest.NewRequest(http.MethodGet, constants.DefaultMetricsEndpoint, nil)
	metricsRec := httptest.NewRecorder()
	app.metricsHandler(metricsRec, metricsReq)
	require.Equal(t, http.StatusOK, metricsRec.Code)

	body := metricsRec.Body.String()
	assert.Contains(t, body, "olla_requests_total 10")
	assert.Contains(t, body, "olla_failures_total 1")
	assert.Contains(t, body, "olla_avg_latency_ms 50")
	assert.Contains(t, body, `olla_endpoint_requests_total{endpoint="ep-a",status="healthy"} 10`)
}

func TestEscapePrometheusLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `line1\nline2`, escapePrometheusLabel("line1\nline2"))
	assert.Equal(t, `say \"hello\"`, escapePrometheusLabel(`say "hello"`))
	assert.Equal(t, `path\\to`, escapePrometheusLabel(`path\to`))
}

func TestFormatPrometheusFloat(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "125", formatPrometheusFloat(125))
	assert.Equal(t, "99.6", formatPrometheusFloat(99.6))
}

func TestParsePercentage(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 99.2, parsePercentage("99.2%"), 0.001)
	assert.InDelta(t, 0, parsePercentage("0.0%"), 0.001)
}

func TestSystemStatusValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, float64(2), systemStatusValue(statusHealthy))
	assert.Equal(t, float64(1), systemStatusValue(statusDegraded))
	assert.Equal(t, float64(0), systemStatusValue(statusCritical))
}

func TestWritePrometheusLabeledGauge(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	writePrometheusLabeledGauge(&b, "olla_info", 1, "version", "0.0.28", "engine", "olla")

	assert.Equal(t, `olla_info{version="0.0.28",engine="olla"} 1`+"\n", b.String())
}
