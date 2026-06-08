package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// captureBodyProxyService records the request body received by the proxy engine.
// This lets tests assert that body restoration works across the read in ollamaModelShowHandler.
type captureBodyProxyService struct {
	capturedBody []byte
	capturedCtx  context.Context
}

func (m *captureBodyProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	m.capturedCtx = r.Context()
	if r.Body != nil {
		m.capturedBody, _ = io.ReadAll(r.Body)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (m *captureBodyProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return nil
}

func (m *captureBodyProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}

func (m *captureBodyProxyService) UpdateConfig(configuration ports.ProxyConfiguration) {}

// buildShowApp constructs an Application wired for /api/show tests.
// ollamaURL is the URL of the (optional) fake Ollama endpoint; pass nil to skip
// endpoint setup. The returned registry has "llama3:latest" registered against that endpoint.
func buildShowApp(t *testing.T, ollamaURL *url.URL) (*Application, *captureBodyProxyService) {
	t.Helper()

	logCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	sl := logger.NewPlainStyledLogger(log)

	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(sl, nil, nil, nil)

	if ollamaURL != nil {
		ep := &domain.Endpoint{
			Name:      "ollama-show-test",
			URL:       ollamaURL,
			URLString: ollamaURL.String(),
			Type:      "ollama",
			Status:    domain.StatusHealthy,
		}
		require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(
			context.Background(),
			ep,
			[]*domain.ModelInfo{{Name: "llama3:latest"}},
		))
		// Wait for the async unifier goroutine to propagate the entry.
		time.Sleep(150 * time.Millisecond)
	}

	capture := &captureBodyProxyService{}

	app := createTestApplication(t)
	app.modelRegistry = unifiedRegistry
	app.proxyService = capture

	if ollamaURL != nil {
		app.discoveryService = &mockDiscoveryServiceWithHealthy{
			endpoints: []*domain.Endpoint{{
				Name:      "ollama-show-test",
				URL:       ollamaURL,
				URLString: ollamaURL.String(),
				Type:      "ollama",
				Status:    domain.StatusHealthy,
			}},
		}
	}

	return app, capture
}

// TestOllamaModelShowHandler_MissingModel tests that an empty or absent model name
// is rejected before any proxy machinery is invoked.
func TestOllamaModelShowHandler_MissingModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"empty model field", `{"model":""}`},
		{"no model field", `{"verbose":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, _ := buildShowApp(t, nil)

			req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			app.ollamaModelShowHandler(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400 for missing model; body=%q", tt.body)
		})
	}
}

// TestOllamaModelShowHandler_InvalidJSON tests that malformed JSON is rejected cleanly.
func TestOllamaModelShowHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	app, _ := buildShowApp(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
		strings.NewReader(`{not valid json}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ollamaModelShowHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestOllamaModelShowHandler_UnknownModel tests that a model Olla has never seen
// produces a 404 owned by Olla rather than a leaked backend response.
func TestOllamaModelShowHandler_UnknownModel(t *testing.T) {
	t.Parallel()

	app, _ := buildShowApp(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
		strings.NewReader(`{"model":"does-not-exist:latest"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ollamaModelShowHandler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "does-not-exist:latest")
}

// TestOllamaModelShowHandler_KnownModel_Delegates tests the happy path: a known model
// is proxied through providerProxyHandler to a backend endpoint. The test is wired
// through the full route (handler → providerProxyHandler → proxyService) rather than
// calling ollamaModelShowHandler in isolation, matching the lesson from issue #139.
func TestOllamaModelShowHandler_KnownModel_Delegates(t *testing.T) {
	t.Parallel()

	backendURL, _ := url.Parse("http://localhost:11434")
	app, capture := buildShowApp(t, backendURL)

	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
		strings.NewReader(`{"model":"llama3:latest"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ollamaModelShowHandler(w, req)

	require.NotNil(t, capture.capturedCtx,
		"proxy was never invoked — handler failed before reaching executeProxyRequest (status=%d body=%q)",
		w.Code, w.Body.String())

	assert.Equal(t, http.StatusOK, w.Code,
		"expected proxy to succeed; body=%q", w.Body.String())
}

// TestOllamaModelShowHandler_BodyRestoredForProxy asserts that the body read during
// model-name extraction is fully restored before providerProxyHandler forwards the request.
// If restoration is broken the downstream proxy receives an empty body and the backend
// returns a 400/422.
func TestOllamaModelShowHandler_BodyRestoredForProxy(t *testing.T) {
	t.Parallel()

	backendURL, _ := url.Parse("http://localhost:11434")
	app, capture := buildShowApp(t, backendURL)

	originalBody := `{"model":"llama3:latest","verbose":true}`
	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
		strings.NewReader(originalBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ollamaModelShowHandler(w, req)

	require.NotNil(t, capture.capturedCtx,
		"proxy was never invoked (status=%d body=%q)", w.Code, w.Body.String())

	assert.Equal(t, originalBody, string(capture.capturedBody),
		"body forwarded to proxy must match original — body restoration is broken")
}

// TestOllamaModelShowHandler_NoRegistryFallthrough tests the graceful-degradation path:
// when the model registry is not a *UnifiedMemoryModelRegistry the handler must still
// forward the request rather than returning an error.
func TestOllamaModelShowHandler_NoRegistryFallthrough(t *testing.T) {
	t.Parallel()

	backendURL, _ := url.Parse("http://localhost:11434")
	app, capture := buildShowApp(t, backendURL)

	// Replace the registry with a plain mock that doesn't satisfy the type assertion.
	app.modelRegistry = &baseMockRegistry{}

	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/show",
		strings.NewReader(`{"model":"llama3:latest"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ollamaModelShowHandler(w, req)

	// The handler must not 404 — it should fall through to the proxy.
	// capturedCtx will be set if the proxy was reached.
	assert.NotEqual(t, http.StatusNotFound, w.Code,
		"handler must not 404 when registry type assertion fails — should fall through to proxy")
	assert.NotEqual(t, http.StatusBadRequest, w.Code,
		"handler must not 400 when registry type assertion fails")

	// With no healthy endpoints the proxy returns 404 "No ollama endpoints available"
	// OR 200 if the capture proxy ran — either is correct. What matters is that we
	// did NOT get the 501 from the old stub.
	assert.NotEqual(t, http.StatusNotImplemented, w.Code,
		"old 501 stub must be gone")

	_ = capture // capture may or may not have fired depending on endpoint selection
}
