package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

// TestTranslationHandler_StrictRejection_DecisionAwareStatusAndHeaders covers fix 3
// from the #191 follow-up review: the translation route's zero-endpoint branch used to
// always return a hardcoded 404 via writeTranslatorError, ignoring pr.profile.RoutingDecision
// entirely. That flattened a strict "model only on unhealthy endpoints" 503 to 404 and never
// set X-Olla-Routing-* headers, unlike the equivalent proxy-route rejection
// (writeNoRoutableEndpoints). This proves the translation route now honours the recorded
// decision's status code and reason, sets the routing headers, and still returns an
// Anthropic-shaped JSON error body (the Anthropic route must never fall back to the
// plain-text writeNoRoutableEndpoints format).
func TestTranslationHandler_StrictRejection_DecisionAwareStatusAndHeaders(t *testing.T) {
	t.Parallel()

	styledLog := &mockStyledLogger{}

	// Real inspector chain (path + body) so profile.ModelName is populated from the
	// request body exactly as it is in production, matching the #191 proxy-route tests.
	chain := inspector.NewChain(styledLog)
	bodyInspector, err := inspector.NewBodyInspector(styledLog)
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)

	trans, err := anthropic.NewTranslator(styledLog, config.AnthropicTranslatorConfig{})
	require.NoError(t, err)

	// Strict registry: the requested model exists nowhere in the fleet, so
	// GetRoutableEndpointsForModel rejects with a genuine strict decision
	// (action "rejected", reason model_not_found, status 404).
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{},
		strict:            true,
	}

	app := &Application{
		Config:           &config.Config{},
		logger:           styledLog,
		inspectorChain:   chain,
		discoveryService: &mockDiscoveryServiceForTranslation{},
		modelRegistry:    modelRegistry,
		statsCollector:   &mockStatsCollector{},
		proxyService: &mockProxyService{
			proxyFunc: nil, // must never be reached
		},
	}

	body := `{"model":"claude-not-registered","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()

	app.translationHandler(trans).ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "must use the decision's status code, not a hardcoded default")
	assert.Equal(t, "strict", w.Header().Get(constants.HeaderXOllaRoutingStrategy))
	assert.Equal(t, "rejected", w.Header().Get(constants.HeaderXOllaRoutingDecision))
	assert.Equal(t, "model_not_found", w.Header().Get(constants.HeaderXOllaRoutingReason))

	// Anthropic-shaped JSON error body must be preserved - the route must never
	// fall back to the plain-text writeNoRoutableEndpoints format.
	var errResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "error", errResp["type"])
	errObj, ok := errResp["error"].(map[string]interface{})
	require.True(t, ok, "response must carry an Anthropic-shaped error object")
	assert.Equal(t, "not_found_error", errObj["type"], "404 must map to Anthropic's not_found_error type")
}

// TestTranslationHandler_StrictRejection_ModelUnavailableMaps503 proves the 503 case
// (model exists but only on unhealthy endpoints) also survives to the translation route,
// where it previously would have been flattened to 404.
func TestTranslationHandler_StrictRejection_ModelUnavailableMaps503(t *testing.T) {
	t.Parallel()

	styledLog := &mockStyledLogger{}

	chain := inspector.NewChain(styledLog)
	bodyInspector, err := inspector.NewBodyInspector(styledLog)
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)

	trans, err := anthropic.NewTranslator(styledLog, config.AnthropicTranslatorConfig{})
	require.NoError(t, err)

	// Model is known to the fleet but not present on the one healthy candidate returned
	// by mockDiscoveryServiceForTranslation, so the strict registry rejects with 503.
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"claude-somewhere-else": {"http://some-other-host:9999"},
		},
		strict: true,
	}

	app := &Application{
		Config:           &config.Config{},
		logger:           styledLog,
		inspectorChain:   chain,
		discoveryService: &mockDiscoveryServiceForTranslation{},
		modelRegistry:    modelRegistry,
		statsCollector:   &mockStatsCollector{},
		proxyService: &mockProxyService{
			proxyFunc: nil,
		},
	}

	body := `{"model":"claude-somewhere-else","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()

	app.translationHandler(trans).ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code, "model_unavailable must map to 503, not the hardcoded 404 default")
	assert.Equal(t, "strict", w.Header().Get(constants.HeaderXOllaRoutingStrategy))
	assert.Equal(t, "rejected", w.Header().Get(constants.HeaderXOllaRoutingDecision))
	assert.Equal(t, "model_unavailable", w.Header().Get(constants.HeaderXOllaRoutingReason))

	var errResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	errObj, ok := errResp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "api_error", errObj["type"], "503 falls back to the generic api_error type per the Anthropic taxonomy")
}
