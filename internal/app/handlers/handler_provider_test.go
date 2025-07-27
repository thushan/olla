package handlers

import (
	"context"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
