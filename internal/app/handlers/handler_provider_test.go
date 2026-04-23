package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// mockDiscoveryService for testing
type mockDiscoveryService struct{}

func (m *mockDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *mockDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *mockDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (m *mockDiscoveryService) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

// createTestApplication creates a minimal Application for testing
func createTestApplication(t *testing.T) *Application {
	logger := &mockStyledLogger{}
	profileFactory := &mockProfileFactory{
		validProfiles: map[string]bool{
			"ollama":    true,
			"lmstudio":  true,
			"lm-studio": true,
			"openai":    true,
			"vllm":      true,
		},
	}

	// Create minimal config
	cfg := &config.Config{
		Server: config.ServerConfig{
			RateLimits: config.ServerRateLimits{},
		},
	}

	// Create empty inspector chain
	inspectorChain := inspector.NewChain(logger)

	return &Application{
		Config:           cfg,
		logger:           logger,
		discoveryService: &mockDiscoveryService{},
		profileFactory:   profileFactory,
		inspectorChain:   inspectorChain,
		StartTime:        time.Now(),
	}
}

// TestProviderRouting tests basic provider routing functionality by calling the actual handler
func TestProviderRouting(t *testing.T) {
	tests := []struct {
		name               string
		url                string
		method             string
		expectedStatusCode int
		expectedError      string
	}{
		{
			name:               "Invalid provider path - too short",
			url:                "/olla",
			method:             "POST",
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Invalid path format",
		},
		{
			name:               "Unknown provider type",
			url:                "/olla/unknown/api/test",
			method:             "POST",
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Unknown provider type: unknown",
		},
		{
			name:               "Valid Ollama provider",
			url:                "/olla/ollama/api/generate",
			method:             "POST",
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "No ollama endpoints available",
		},
		{
			name:               "Valid LM Studio provider",
			url:                "/olla/lmstudio/v1/chat/completions",
			method:             "POST",
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "No lm-studio endpoints available",
		},
		{
			name:               "Valid OpenAI provider",
			url:                "/olla/openai/v1/models",
			method:             "POST",
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "No openai endpoints available",
		},
		{
			name:               "Valid vLLM provider",
			url:                "/olla/vllm/generate",
			method:             "POST",
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "No vllm endpoints available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestApplication(t)
			req := httptest.NewRequest(tt.method, tt.url, nil)
			w := httptest.NewRecorder()

			// Call the actual handler method
			app.providerProxyHandler(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			if tt.expectedError != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, body)
				}
			}
		})
	}
}

// mockDiscoveryServiceWithHealthy returns a single healthy endpoint matching the
// requested provider type so provider-scoped routing can reach the proxy stage.
type mockDiscoveryServiceWithHealthy struct {
	endpoints []*domain.Endpoint
}

func (m *mockDiscoveryServiceWithHealthy) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}
func (m *mockDiscoveryServiceWithHealthy) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}
func (m *mockDiscoveryServiceWithHealthy) RefreshEndpoints(ctx context.Context) error { return nil }
func (m *mockDiscoveryServiceWithHealthy) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

// captureProxyService records the request context so tests can assert which
// values the handler propagated to the proxy engine.
type captureProxyService struct {
	capturedCtx context.Context
}

func (m *captureProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	m.capturedCtx = r.Context()
	// Simulate the sticky wrapper writing outcome headers before the proxy flushes.
	if outcome, ok := r.Context().Value(constants.ContextStickyOutcomeKey).(*balancer.StickyOutcome); ok && outcome != nil {
		outcome.Result = "miss"
		outcome.Source, _ = r.Context().Value(constants.ContextStickyKeySourceKey).(string)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (m *captureProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return nil
}
func (m *captureProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}
func (m *captureProxyService) UpdateConfig(configuration ports.ProxyConfiguration) {}

// TestProviderProxyHandler_InjectsStickyKey verifies that provider-scoped routes
// (e.g. /olla/ollama/, /olla/lemonade/) invoke sticky key injection just like
// the main proxyHandler. Regression test for github.com/thushan/olla#139 where
// requests to provider URLs bypassed sticky sessions entirely — counters stayed
// at zero and no X-Olla-Sticky-Session header was ever emitted.
func TestProviderProxyHandler_InjectsStickyKey(t *testing.T) {
	app := createTestApplication(t)

	// Enable sticky sessions; without this the handler intentionally skips injection.
	app.Config.Proxy.StickySessions = config.StickySessionConfig{
		Enabled:         true,
		KeySources:      []string{"session_header", "prefix_hash", "ip"},
		MaxSessions:     100,
		IdleTTLSeconds:  60,
		PrefixHashBytes: 512,
	}

	u, _ := url.Parse("http://localhost:11434")
	app.discoveryService = &mockDiscoveryServiceWithHealthy{
		endpoints: []*domain.Endpoint{{
			Name:      "ollama-1",
			URL:       u,
			URLString: u.String(),
			Type:      "ollama",
			Status:    domain.StatusHealthy,
		}},
	}

	capture := &captureProxyService{}
	app.proxyService = capture

	sessionID := "session-abc-123"
	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", strings.NewReader(`{"model":"llama3"}`))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderXOllaSessionID, sessionID)
	w := httptest.NewRecorder()

	app.providerProxyHandler(w, req)

	if capture.capturedCtx == nil {
		t.Fatalf("proxy was never invoked; handler failed before reaching executeProxyRequest (status=%d body=%q)", w.Code, w.Body.String())
	}

	stickyKey, _ := capture.capturedCtx.Value(constants.ContextStickyKeyKey).(string)
	if stickyKey == "" {
		t.Fatal("expected sticky key to be injected into context, got empty string — providerProxyHandler is bypassing sticky sessions")
	}

	source, _ := capture.capturedCtx.Value(constants.ContextStickyKeySourceKey).(string)
	if source != "session_header" {
		t.Errorf("expected key source 'session_header' (X-Olla-Session-ID was supplied), got %q", source)
	}

	outcome, _ := capture.capturedCtx.Value(constants.ContextStickyOutcomeKey).(*balancer.StickyOutcome)
	if outcome == nil {
		t.Fatal("expected StickyOutcome pointer in context for the balancer wrapper to populate")
	}
}

// TestProviderProxyHandler_SkipsStickyWhenDisabled guards against accidental
// breakage of the config gate — requests must not pay the body-read cost or
// pollute the context when sticky sessions are disabled.
func TestProviderProxyHandler_SkipsStickyWhenDisabled(t *testing.T) {
	app := createTestApplication(t)

	// StickySessions.Enabled defaults to false via createTestApplication.

	u, _ := url.Parse("http://localhost:11434")
	app.discoveryService = &mockDiscoveryServiceWithHealthy{
		endpoints: []*domain.Endpoint{{
			Name:      "ollama-1",
			URL:       u,
			URLString: u.String(),
			Type:      "ollama",
			Status:    domain.StatusHealthy,
		}},
	}

	capture := &captureProxyService{}
	app.proxyService = capture

	req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", strings.NewReader(`{"model":"llama3"}`))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderXOllaSessionID, "abc")
	w := httptest.NewRecorder()

	app.providerProxyHandler(w, req)

	if capture.capturedCtx == nil {
		t.Fatalf("proxy was never invoked (status=%d body=%q)", w.Code, w.Body.String())
	}
	if key, _ := capture.capturedCtx.Value(constants.ContextStickyKeyKey).(string); key != "" {
		t.Errorf("expected no sticky key when disabled, got %q", key)
	}
}

// TestProviderPathStripping tests that provider prefixes are correctly stripped
func TestProviderPathStripping(t *testing.T) {
	tests := []struct {
		name         string
		inputPath    string
		provider     string
		expectedPath string
	}{
		{
			name:         "Ollama API path",
			inputPath:    "/olla/ollama/api/generate",
			provider:     "ollama",
			expectedPath: "/api/generate",
		},
		{
			name:         "LM Studio API path",
			inputPath:    "/olla/lmstudio/v1/chat/completions",
			provider:     "lmstudio",
			expectedPath: "/v1/chat/completions",
		},
		{
			name:         "Root path after stripping",
			inputPath:    "/olla/ollama",
			provider:     "ollama",
			expectedPath: "/",
		},
		{
			name:         "Path with trailing slash",
			inputPath:    constants.DefaultOllaProxyPathPrefix + "ollama/",
			provider:     "ollama",
			expectedPath: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the path stripping logic
			originalPath := tt.inputPath
			providerPrefix := constants.DefaultOllaProxyPathPrefix + tt.provider

			resultPath := originalPath
			if strings.HasPrefix(originalPath, providerPrefix) {
				resultPath = strings.TrimPrefix(originalPath, providerPrefix)
				if resultPath == "" {
					resultPath = "/"
				}
			}

			if resultPath != tt.expectedPath {
				t.Errorf("Expected path '%s', got '%s'", tt.expectedPath, resultPath)
			}
		})
	}
}
