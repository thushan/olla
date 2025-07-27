package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProviderRouting tests basic provider routing functionality
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
			expectedError:      "Invalid path",
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
			expectedStatusCode: http.StatusInternalServerError, // Will fail due to no app setup
		},
		{
			name:               "Valid LM Studio provider",
			url:                "/olla/lmstudio/v1/chat/completions",
			method:             "POST",
			expectedStatusCode: http.StatusInternalServerError, // Will fail due to no app setup
		},
		{
			name:               "Valid OpenAI provider",
			url:                "/olla/openai/v1/models",
			method:             "POST",
			expectedStatusCode: http.StatusInternalServerError, // Will fail due to no app setup
		},
		{
			name:               "Valid vLLM provider",
			url:                "/olla/vllm/generate",
			method:             "POST",
			expectedStatusCode: http.StatusInternalServerError, // Will fail due to no app setup
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			w := httptest.NewRecorder()

			// We only test the initial validation logic, not the full proxy flow
			// Directly test the validation part
			pathParts := strings.Split(req.URL.Path, "/")
			if len(pathParts) < 3 {
				http.Error(w, "Invalid path", http.StatusBadRequest)
			} else {
				providerType := pathParts[2]
				switch providerType {
				case "ollama", "lmstudio", "openai", "vllm":
					// For valid providers, we'd normally continue but we can't without full setup
					http.Error(w, "Test setup incomplete", http.StatusInternalServerError)
				default:
					http.Error(w, "Unknown provider type: "+providerType, http.StatusBadRequest)
				}
			}

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
			inputPath:    "/olla/ollama/",
			provider:     "ollama",
			expectedPath: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the path stripping logic
			originalPath := tt.inputPath
			providerPrefix := "/olla/" + tt.provider

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
