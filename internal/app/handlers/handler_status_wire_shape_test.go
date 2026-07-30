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

// Wire-shape pinning test for FR-13: no pre-existing field on
// /internal/status, /internal/status/endpoints or /internal/status/models
// may be renamed, retyped or removed by the additive dashboard fields.
//
// The expected maps below were captured from main's handlers (the branch had
// not touched these files when this test was written). Each entry pins a JSON
// key to the type encoding/json produces when decoding into interface{}:
// string, float64 (numbers), bool, map[string]interface{} or
// []interface{}. Additive fields added later are allowed: this test only
// asserts the pre-existing contract is preserved, so it does not fail when
// new keys appear alongside the pinned ones.

func jsonShape(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &m), "response was not a JSON object")
	return m
}

func doStatus(t *testing.T, app *Application, target string) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	switch target {
	case "/internal/status":
		app.statusHandler(w, req)
	case "/internal/status/endpoints":
		app.endpointsStatusHandler(w, req)
	case "/internal/status/models":
		app.modelsStatusHandler(w, req)
	default:
		t.Fatalf("unknown target %s", target)
	}
	require.Equal(t, http.StatusOK, w.Code, "handler for %s returned non-200", target)
	return w.Body.Bytes()
}

func assertKeysTyped(t *testing.T, obj map[string]interface{}, expected map[string]string) {
	t.Helper()
	for key, wantType := range expected {
		got, ok := obj[key]
		require.True(t, ok, "missing pinned key %q (FR-13 regression)", key)
		switch wantType {
		case "string":
			_, ok := got.(string)
			assert.True(t, ok, "key %q: want string, got %T (%v)", key, got, got)
		case "number":
			_, ok := got.(float64)
			assert.True(t, ok, "key %q: want number, got %T (%v)", key, got, got)
		case "bool":
			_, ok := got.(bool)
			assert.True(t, ok, "key %q: want bool, got %T (%v)", key, got, got)
		case "object":
			_, ok := got.(map[string]interface{})
			assert.True(t, ok, "key %q: want object, got %T (%v)", key, got, got)
		case "array":
			_, ok := got.([]interface{})
			assert.True(t, ok, "key %q: want array, got %T (%v)", key, got, got)
		default:
			t.Fatalf("test bug: unknown wantType %q for key %q", wantType, key)
		}
	}
}

func TestWireShape_StatusResponse(t *testing.T) {
	app := seedWireShapeApp(t)
	shape := jsonShape(t, doStatus(t, app, "/internal/status"))

	assertKeysTyped(t, shape, map[string]string{
		"timestamp": "string",
		"endpoints": "array",
		"proxy":     "object",
		"security":  "object",
		"system":    "object",
	})

	sys, _ := shape["system"].(map[string]interface{})
	require.NotNil(t, sys, "system was not an object")
	assertKeysTyped(t, sys, map[string]string{
		"status":              "string",
		"endpoints_up":        "string",
		"success_rate":        "string",
		"avg_latency":         "string",
		"total_traffic":       "string",
		"uptime":              "string",
		"version":             "string",
		"commit":              "string",
		"active_connections":  "number",
		"security_violations": "number",
		"total_requests":      "number",
		"total_failures":      "number",
	})

	proxy, _ := shape["proxy"].(map[string]interface{})
	require.NotNil(t, proxy)
	assertKeysTyped(t, proxy, map[string]string{
		"engine":   "string",
		"profile":  "string",
		"balancer": "string",
	})

	sec, _ := shape["security"].(map[string]interface{})
	require.NotNil(t, sec)
	assertKeysTyped(t, sec, map[string]string{
		"status":      "string",
		"blocked_ips": "number",
		"violations":  "object",
	})

	endpoints, _ := shape["endpoints"].([]interface{})
	require.NotEmpty(t, endpoints, "seed should produce at least one endpoint")
	first, _ := endpoints[0].(map[string]interface{})
	require.NotNil(t, first)
	assertKeysTyped(t, first, map[string]string{
		"name":         "string",
		"status":       "string",
		"success_rate": "string",
		"avg_latency":  "string",
		"traffic":      "string",
		"last_check":   "string",
		"next_check":   "string",
		"issues":       "string",
		"models":       "object",
		"priority":     "number",
		"connections":  "number",
		"requests":     "number",
	})
}

func TestWireShape_EndpointsStatusResponse(t *testing.T) {
	app := seedWireShapeApp(t)
	shape := jsonShape(t, doStatus(t, app, "/internal/status/endpoints"))

	assertKeysTyped(t, shape, map[string]string{
		"timestamp":      "string",
		"endpoints":      "array",
		"total_count":    "number",
		"healthy_count":  "number",
		"routable_count": "number",
	})

	endpoints, _ := shape["endpoints"].([]interface{})
	require.NotEmpty(t, endpoints)
	// Find the endpoint with traffic so request_count / health_check are
	// non-omitempty and present. Other endpoint entries pin the always-on keys.
	var withTraffic map[string]interface{}
	for _, e := range endpoints {
		em, _ := e.(map[string]interface{})
		require.NotNil(t, em)
		if _, ok := em["request_count"]; ok {
			if rc, _ := em["request_count"].(float64); rc > 0 {
				withTraffic = em
			}
		}
		if withTraffic == nil {
			withTraffic = em
		}
	}
	require.NotNil(t, withTraffic, "no endpoint found")
	assertKeysTyped(t, withTraffic, map[string]string{
		"name":          "string",
		"type":          "string",
		"status":        "string",
		"priority":      "number",
		"model_count":   "number",
		"request_count": "number",
		"success_rate":  "string",
	})
}

func TestWireShape_ModelsStatusResponse(t *testing.T) {
	app := seedWireShapeApp(t)
	shape := jsonShape(t, doStatus(t, app, "/internal/status/models"))

	assertKeysTyped(t, shape, map[string]string{
		"timestamp":        "string",
		"models_by_family": "object",
		"recent_models":    "array",
		"total_models":     "number",
		"total_families":   "number",
		"total_endpoints":  "number",
	})

	modelsByFamily, _ := shape["models_by_family"].(map[string]interface{})
	require.NotNil(t, modelsByFamily)
	for family, vals := range modelsByFamily {
		arr, ok := vals.([]interface{})
		assert.True(t, ok, "models_by_family[%q] should be array", family)
		for _, v := range arr {
			_, ok := v.(string)
			assert.True(t, ok, "models_by_family[%q] entries should be strings", family)
		}
	}

	recent, _ := shape["recent_models"].([]interface{})
	require.NotEmpty(t, recent)
	first, _ := recent[0].(map[string]interface{})
	require.NotNil(t, first)
	assertKeysTyped(t, first, map[string]string{
		"name":      "string",
		"endpoints": "array",
		"last_seen": "string",
	})
}

// seedWireShapeApp builds a minimal Application with a healthy and an
// unhealthy endpoint, real stats and a discovered model, sufficient to
// exercise every non-omitempty field the three handlers emit.
func seedWireShapeApp(t *testing.T) *Application {
	t.Helper()
	const healthyURL = "http://localhost:11434"
	const unhealthyURL = "http://localhost:8080"

	endpoints := []*domain.Endpoint{
		{
			Name:          "ollama-1",
			Type:          "ollama",
			URLString:     healthyURL,
			Status:        domain.StatusHealthy,
			Priority:      1,
			LastChecked:   time.Now().Add(-30 * time.Second),
			NextCheckTime: time.Now().Add(30 * time.Second),
			LastLatency:   14 * time.Millisecond,
		},
		{
			Name:          "openai-1",
			Type:          "openai",
			URLString:     unhealthyURL,
			Status:        domain.StatusUnhealthy,
			Priority:      2,
			LastChecked:   time.Now().Add(-2 * time.Minute),
			NextCheckTime: time.Now().Add(30 * time.Second),
		},
	}

	family := "llama"
	paramSize := "7B"
	quant := "q4_0"
	mtype := "llm"

	modelMap := map[string]*domain.EndpointModels{
		healthyURL: {
			Models: []*domain.ModelInfo{
				{
					Name:     "llama2:7b",
					Type:     mtype,
					LastSeen: time.Now().Add(-1 * time.Minute),
					Details: &domain.ModelDetails{
						Family:            &family,
						ParameterSize:     &paramSize,
						QuantizationLevel: &quant,
					},
				},
			},
			LastUpdated: time.Now().Add(-90 * time.Second),
		},
	}

	endpointStats := map[string]ports.EndpointStats{
		healthyURL: {
			Name:               "ollama-1",
			URL:                healthyURL,
			TotalRequests:      100,
			SuccessfulRequests: 95,
			FailedRequests:     5,
			TotalBytes:         1_000_000,
			AverageLatency:     120,
			MinLatency:         40,
			MaxLatency:         800,
		},
	}

	repo := &mockStatusEndpointRepository{endpoints: endpoints}
	stats := &mockStatusStatsCollector{
		endpointStats: endpointStats,
		connectionStats: map[string]int64{
			healthyURL:   3,
			unhealthyURL: 0,
		},
	}
	registry := &mockStatusModelRegistry{endpointModels: modelMap}

	return &Application{
		repository:     repo,
		statsCollector: stats,
		modelRegistry:  registry,
		StartTime:      time.Now().Add(-10 * time.Minute),
		Config:         &config.Config{Proxy: config.ProxyConfig{Engine: "olla", Profile: "balanced", LoadBalancer: "round_robin"}},
	}
}
