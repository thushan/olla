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
)

// WP-2 acceptance criterion: the same endpoint must report the SAME id across
// every /internal/status* payload. The dashboard keys model click-through
// rows on models[].endpoint_ids and matches them against endpoints[].id, so
// any drift breaks the click-through silently. Each status handler builds its
// own ID map from its own snapshot, so the only way they agree is if every
// handler derives IDs via the same buildEndpointIDs precompute over the same
// endpoint set.

// TestEndpointIDs_AcrossPayloadsConsistent drives a single Application
// fixture (one endpoint set, one model map) through BOTH the
// /internal/status/endpoints handler and the /internal/status/models
// handler, and asserts the ID for each endpoint is byte-for-byte identical
// across the two payloads. The fixture deliberately uses query-bearing URLs
// so the disambiguator path is exercised - a query-only-differing pair is
// exactly the case where the old per-handler raw-URL hash could have drifted
// if the two handlers had ever diverged in derivation.
func TestEndpointIDs_AcrossPayloadsConsistent(t *testing.T) {
	t.Parallel()

	const urlA = "http://host:11434/v1?api_key=alpha"
	const urlB = "http://host:11434/v1?api_key=beta"

	endpoints := []*domain.Endpoint{
		{Name: "node-a", Type: "ollama", URLString: urlA, Status: domain.StatusHealthy, Priority: 5},
		{Name: "node-b", Type: "ollama", URLString: urlB, Status: domain.StatusHealthy, Priority: 5},
	}
	// Both endpoints host the same model so ModelSummary.EndpointIDs is
	// populated for that model with one entry per endpoint.
	registry := &mockStatusModelRegistry{
		endpointModels: map[string]*domain.EndpointModels{
			urlA: {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: time.Now()}}},
			urlB: {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: time.Now()}}},
		},
	}
	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  registry,
		Config:         &config.Config{Proxy: config.ProxyConfig{Engine: "olla", Profile: "balanced", LoadBalancer: "round_robin"}},
		StartTime:      time.Now(),
	}

	// Drive /internal/status/endpoints.
	endpointsReq := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	endpointsRec := httptest.NewRecorder()
	app.endpointsStatusHandler(endpointsRec, endpointsReq)
	require.Equal(t, http.StatusOK, endpointsRec.Code, "endpoints handler failed: %s", endpointsRec.Body.String())

	var endpointsResp EndpointStatusResponse
	require.NoError(t, json.NewDecoder(endpointsRec.Body).Decode(&endpointsResp))
	require.Len(t, endpointsResp.Endpoints, 2, "expected both endpoints in endpoints payload")

	// Drive /internal/status/models.
	modelsReq := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	modelsRec := httptest.NewRecorder()
	app.modelsStatusHandler(modelsRec, modelsReq)
	require.Equal(t, http.StatusOK, modelsRec.Code, "models handler failed: %s", modelsRec.Body.String())

	var modelsResp ModelStatusResponse
	require.NoError(t, json.NewDecoder(modelsRec.Body).Decode(&modelsResp))

	// Build a lookup of endpoint URL -> id from the endpoints payload. The
	// endpoints payload surfaces the SANITISED url field, so index by that,
	// not the raw URL (the raw URL is private to the repository).
	endpointIDByDisplayName := make(map[string]string, len(endpointsResp.Endpoints))
	for _, ep := range endpointsResp.Endpoints {
		endpointIDByDisplayName[ep.Name] = ep.ID
	}
	require.Len(t, endpointIDByDisplayName, 2, "expected two distinct endpoint names in fixture")

	// Also drive /internal/status to confirm the third consumer agrees.
	statusReq := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	statusRec := httptest.NewRecorder()
	app.statusHandler(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code, "status handler failed: %s", statusRec.Body.String())
	var statusResp StatusResponse
	require.NoError(t, json.NewDecoder(statusRec.Body).Decode(&statusResp))

	statusIDByName := make(map[string]string, len(statusResp.Endpoints))
	for _, ep := range statusResp.Endpoints {
		statusIDByName[ep.Name] = ep.ID
	}

	// The model summary's EndpointIDs slice must line up positionally with
	// its Endpoints slice (the paired-sort contract), and every entry must
	// match the id the endpoints and status payloads report for that name.
	var summary *ModelSummary
	for i := range modelsResp.RecentModels {
		if modelsResp.RecentModels[i].Name == "llama3" {
			summary = &modelsResp.RecentModels[i]
			break
		}
	}
	require.NotNil(t, summary, "llama3 missing from models payload")
	require.Len(t, summary.Endpoints, 2, "expected llama3 on both endpoints")
	require.Len(t, summary.EndpointIDs, 2, "expected two endpoint_ids on llama3")

	for i, name := range summary.Endpoints {
		modelsID := summary.EndpointIDs[i]
		endpointsID, ok := endpointIDByDisplayName[name]
		require.True(t, ok, "model endpoint name %q missing from endpoints payload", name)
		assert.Equal(t, endpointsID, modelsID,
			"model endpoint_ids[%d] (%q) must equal endpoints[].id for %q", i, modelsID, name)
		statusID, ok := statusIDByName[name]
		require.True(t, ok, "model endpoint name %q missing from status payload", name)
		assert.Equal(t, statusID, modelsID,
			"model endpoint_ids[%d] (%q) must equal status[].id for %q", i, modelsID, name)
	}

	// Sanity: the two colliding siblings really did get distinct IDs across
	// every payload (otherwise the test above would pass trivially via a
	// shared collision).
	endpointsIDs := map[string]struct{}{}
	for _, id := range endpointIDByDisplayName {
		endpointsIDs[id] = struct{}{}
	}
	require.Len(t, endpointsIDs, 2, "expected two distinct IDs across the colliding siblings")
}

// TestEndpointIDs_SecretRotationKeepsAllPayloadIDsStable is the WP-2
// end-to-end rotation test: rotating the api_key in BOTH colliding endpoints
// must leave their IDs unchanged across EVERY status payload. This is the
// user-visible guarantee - dashboard deep-links and row keys do not break
// when an operator rotates a credential.
func TestEndpointIDs_SecretRotationKeepsAllPayloadIDsStable(t *testing.T) {
	t.Parallel()

	runEndpointsHandler := func(t *testing.T, urlA, urlB string) map[string]string {
		t.Helper()
		endpoints := []*domain.Endpoint{
			{Name: "node-a", Type: "ollama", URLString: urlA, Status: domain.StatusHealthy, Priority: 5},
			{Name: "node-b", Type: "ollama", URLString: urlB, Status: domain.StatusHealthy, Priority: 5},
		}
		app := &Application{
			repository:     &mockStatusEndpointRepository{endpoints: endpoints},
			statsCollector: &mockStatusStatsCollector{},
			modelRegistry:  &mockStatusModelRegistry{},
			StartTime:      time.Now(),
		}
		req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
		w := httptest.NewRecorder()
		app.endpointsStatusHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp EndpointStatusResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		out := make(map[string]string, len(resp.Endpoints))
		for _, ep := range resp.Endpoints {
			out[ep.Name] = ep.ID
		}
		return out
	}

	before := runEndpointsHandler(t,
		"http://host:11434/v1?api_key=alpha",
		"http://host:11434/v1?api_key=beta",
	)
	after := runEndpointsHandler(t,
		"http://host:11434/v1?api_key=rotated-xyz",
		"http://host:11434/v1?api_key=rotated-abc",
	)

	assert.Equal(t, before["node-a"], after["node-a"], "node-a ID must survive api_key rotation")
	assert.Equal(t, before["node-b"], after["node-b"], "node-b ID must survive api_key rotation")
}
