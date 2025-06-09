package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
	"github.com/thushan/olla/theme"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
)

func TestDiscoverModels(t *testing.T) {
	tests := []struct {
		name           string
		endpointType   string
		serverResponse string
		serverStatus   int
		expectedModels int
		expectedError  bool
		errorType      interface{}
	}{
		{
			name:         "Ollama endpoint success",
			endpointType: domain.ProfileOllama,
			serverResponse: `{
				"models": [
					{"name": "llama4:128x17b", "size": 3825819519},
					{"name": "gemma3:12b", "size": 7365960935}
				]
			}`,
			serverStatus:   200,
			expectedModels: 2,
			expectedError:  false,
		},
		{
			name:         "LM Studio endpoint success",
			endpointType: domain.ProfileLmStudio,
			serverResponse: `{
				"object": "list",
				"data": [
					{"id": "microsoft/DialoGPT-medium", "object": "model"},
					{"id": "gpt-3.5-turbo", "object": "model"}
				]
			}`,
			serverStatus:   200,
			expectedModels: 2,
			expectedError:  false,
		},
		{
			name:         "OpenAI Compatible endpoint success",
			endpointType: domain.ProfileOpenAICompatible,
			serverResponse: `{
				"object": "list",
				"data": [
					{"id": "text-davinci-003", "object": "model"}
				]
			}`,
			serverStatus:   200,
			expectedModels: 1,
			expectedError:  false,
		},
		{
			name:           "HTTP 404 error",
			endpointType:   domain.ProfileOllama,
			serverResponse: `{"error": "not found"}`,
			serverStatus:   404,
			expectedError:  true,
			errorType:      &DiscoveryError{},
		},
		{
			name:           "HTTP 500 error",
			endpointType:   domain.ProfileOllama,
			serverResponse: `{"error": "internal server error"}`,
			serverStatus:   500,
			expectedError:  true,
			errorType:      &DiscoveryError{},
		},
		{
			name:           "Invalid JSON response",
			endpointType:   domain.ProfileOllama,
			serverResponse: `{"models": [`,
			serverStatus:   200,
			expectedError:  true,
			errorType:      &DiscoveryError{},
		},
		{
			name:           "Empty response",
			endpointType:   domain.ProfileOllama,
			serverResponse: `{"models": []}`,
			serverStatus:   200,
			expectedModels: 0,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				ExpectedUserAgent := fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version)

				if r.Header.Get("User-Agent") != ExpectedUserAgent {
					t.Errorf("Expected User-Agent header to be %s, got %s", ExpectedUserAgent, r.Header.Get("User-Agent"))
				}
				if r.Header.Get("Accept") != DefaultContentType {
					t.Errorf("Expected Accept header to be %s, got %s", DefaultContentType, r.Header.Get("Accept"))
				}

				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			endpoint := createTestEndpoint(server.URL, tt.endpointType)
			client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

			ctx := context.Background()
			models, err := client.DiscoverModels(ctx, endpoint)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}

				if tt.errorType != nil {
					switch tt.errorType.(type) {
					case *DiscoveryError:
						var discoveryError *DiscoveryError
						if !errors.As(err, &discoveryError) {
							t.Errorf("Expected DiscoveryError, got %T", err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(models) != tt.expectedModels {
				t.Errorf("Expected %d models, got %d", tt.expectedModels, len(models))
			}

			for i, model := range models {
				if model.Name == "" {
					t.Errorf("Model %d has empty name", i)
				}
				if model.LastSeen.IsZero() {
					t.Errorf("Model %d has zero LastSeen time", i)
				}
			}
		})
	}
}

func TestDiscoverModelsAutoDetection(t *testing.T) {
	tests := []struct {
		name            string
		serverResponses map[string]serverResponse // path -> response
		expectedModels  int
		expectedError   bool
	}{
		{
			name: "Ollama detection success",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 200,
					body: `{
						"models": [
							{"name": "llama4:128x17b", "size": 3825819519}
						]
					}`,
				},
				"/v1/models": {
					status: 404,
					body:   `{"error": "not found"}`,
				},
			},
			expectedModels: 1,
			expectedError:  false,
		},
		{
			name: "LM Studio detection after Ollama fails",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 404,
					body:   `{"error": "not found"}`,
				},
				"/v1/models": {
					status: 200,
					body: `{
						"object": "list",
						"data": [
							{"id": "gpt-3.5-turbo", "object": "model"}
						]
					}`,
				},
			},
			expectedModels: 1,
			expectedError:  false,
		},
		{
			name: "OpenAI Compatible fallback success",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 404,
					body:   `{"error": "not found"}`,
				},
				"/v1/models": {
					status: 200,
					body: `{
						"object": "list",
						"data": [
							{"id": "text-davinci-003", "object": "model"}
						]
					}`,
				},
			},
			expectedModels: 1,
			expectedError:  false,
		},
		{
			name: "All profiles fail",
			serverResponses: map[string]serverResponse{
				"/api/tags":  {status: 404, body: `{"error": "not found"}`},
				"/v1/models": {status: 404, body: `{"error": "not found"}`},
			},
			expectedError: true,
		},
		{
			name: "Non-recoverable error stops detection",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 200,
					body:   `{"models": [`, // Invalid JSON
				},
				"/v1/models": {
					status: 404,
					body:   `{"error": "not found"}`,
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Logf("Request received: %s %s", r.Method, r.URL.Path)
				response, exists := tt.serverResponses[r.URL.Path]
				if !exists {
					t.Logf("Path not found: %s", r.URL.Path)
					w.WriteHeader(404)
					w.Write([]byte(`{"error": "path not found"}`))
					return
				}

				t.Logf("Responding with status %d for path %s", response.status, r.URL.Path)
				w.WriteHeader(response.status)
				w.Write([]byte(response.body))
			}))
			defer server.Close()

			//  auto detection
			endpoint := createTestEndpoint(server.URL, domain.ProfileAuto)
			t.Logf("Created endpoint with URL: %s, Type: %s", endpoint.URLString, endpoint.Type)

			client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

			ctx := context.Background()
			models, err := client.DiscoverModels(ctx, endpoint)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else {
					t.Logf("Got expected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(models) != tt.expectedModels {
				t.Errorf("Expected %d models, got %d", tt.expectedModels, len(models))
			} else {
				t.Logf("Successfully discovered %d models", len(models))
			}
		})
	}
}

func TestDiscoverModelsContextCancellation(t *testing.T) {
	// test a delay, so we can cancel the context
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"models": []}`))
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileOllama)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	// crate context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.DiscoverModels(ctx, endpoint)
	if err == nil {
		t.Errorf("Expected timeout error but got none")
	}

	// Check that it's a discovery error wrapping a network error
	var discErr *DiscoveryError
	if errors.As(err, &discErr) {
		var networkError *NetworkError
		if !errors.As(discErr.Err, &networkError) {
			t.Errorf("Expected NetwokError, got %T", discErr.Err)
		}
	}
}

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name          string
		serverStatus  int
		expectedError bool
	}{
		{
			name:          "Health check success",
			serverStatus:  200,
			expectedError: false,
		},
		{
			name:          "Health check failure - 404",
			serverStatus:  404,
			expectedError: true,
		},
		{
			name:          "Health check failure - 500",
			serverStatus:  500,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			endpoint := createTestEndpoint(server.URL, domain.ProfileOllama)
			client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

			err := client.HealthCheck(context.Background(), endpoint)

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestGetMetrics(t *testing.T) {
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	// make sure everything is zero before hero!
	metrics := client.GetMetrics()
	if metrics.TotalDiscoveries != 0 {
		t.Errorf("Expected TotalDiscoveries to be 0, got %d", metrics.TotalDiscoveries)
	}
	if metrics.SuccessfulRequests != 0 {
		t.Errorf("Expected SuccessfulRequests to be 0, got %d", metrics.SuccessfulRequests)
	}
	if metrics.FailedRequests != 0 {
		t.Errorf("Expected FailedRequests to be 0, got %d", metrics.FailedRequests)
	}

	// Test successful discovery updates metrics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"models": [{"name": "test-model"}]}`))
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileOllama)
	_, err := client.DiscoverModels(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metrics = client.GetMetrics()
	if metrics.TotalDiscoveries != 1 {
		t.Errorf("Expected TotalDiscoverries to be 1 was %d", metrics.TotalDiscoveries)
	}
	if metrics.SuccessfulRequests != 1 {
		t.Errorf("Expected SuccessfulRequests to be 1 was %d", metrics.SuccessfulRequests)
	}
	if metrics.LastDiscoveryTime.IsZero() {
		t.Errorf("Expected LastDiscoveryTime to be set")
	}
}

func TestMaxResponseSizeLimit(t *testing.T) {
	// returns response larger than MaxResponseSize
	// cheapest way to create this payload
	largeResponse := fmt.Sprintf(`{"models": [{"name": "%s"}]}`, strings.Repeat("x", MaxResponseSize))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(largeResponse))
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileOllama)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	_, err := client.DiscoverModels(context.Background(), endpoint)
	if err == nil {
		t.Errorf("Expected error due to response size limit")
	}
}

type serverResponse struct {
	status int
	body   string
}

func createTestEndpoint(baseURL, endpointType string) *domain.Endpoint {
	parsedURL, _ := url.Parse(baseURL)

	healthCheckURL, _ := url.Parse(baseURL + "/")
	modelURL, _ := url.Parse(baseURL + "/api/tags") // This doesn't matter for auto-detection

	return &domain.Endpoint{
		Name:                 "test-endpoint",
		URL:                  parsedURL,
		Type:                 endpointType,
		Priority:             100,
		HealthCheckURL:       healthCheckURL,
		ModelUrl:             modelURL,
		URLString:            baseURL,
		HealthCheckURLString: baseURL + "/",
		ModelURLString:       baseURL + "/api/tags",
		Status:               domain.StatusHealthy,
		CheckInterval:        5 * time.Second,
		CheckTimeout:         2 * time.Second,
	}
}
func TestDiscoverModelsProfileCaching(t *testing.T) {
	requestCount := make(map[string]int)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount[r.URL.Path]++
		mu.Unlock()

		t.Logf("Request %d to path: %s", requestCount[r.URL.Path], r.URL.Path)

		switch r.URL.Path {
		case "/api/tags":
			// Ollama endpoint - return success
			w.WriteHeader(200)
			w.Write([]byte(`{
				"models": [
					{"name": "llama3:8b", "size": 4661224676}
				]
			}`))
		case "/v1/models":
			// Should never be called after caching
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "not found"}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "path not found"}`))
		}
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileAuto)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	ctx := context.Background()

	// First discovery - should try auto-detection
	t.Log("=== First discovery (auto-detection) ===")
	models1, err1 := client.DiscoverModels(ctx, endpoint)
	if err1 != nil {
		t.Fatalf("First discovery failed: %v", err1)
	}
	if len(models1) != 1 {
		t.Fatalf("Expected 1 model in first discovery, got %d", len(models1))
	}

	// Check that Ollama endpoint was called (auto-detection success)
	mu.Lock()
	ollamaRequests := requestCount["/api/tags"]
	mu.Unlock()

	if ollamaRequests != 1 {
		t.Errorf("Expected 1 Ollama request in first discovery, got %d", ollamaRequests)
	}

	// Second discovery - should use cached profile (no additional requests to /v1/models)
	t.Log("=== Second discovery (should use cache) ===")
	models2, err2 := client.DiscoverModels(ctx, endpoint)
	if err2 != nil {
		t.Fatalf("Second discovery failed: %v", err2)
	}
	if len(models2) != 1 {
		t.Fatalf("Expected 1 model in second discovery, got %d", len(models2))
	}

	// Verify cache worked - should be 2 Ollama requests, still 0 LM Studio requests
	mu.Lock()
	ollamaRequests2 := requestCount["/api/tags"]
	lmStudioRequests2 := requestCount["/v1/models"]
	mu.Unlock()

	if ollamaRequests2 != 2 {
		t.Errorf("Expected 2 Ollama requests after caching, got %d", ollamaRequests2)
	}
	if lmStudioRequests2 != 0 {
		t.Errorf("Expected 0 LM Studio requests after caching (cache should prevent this), got %d", lmStudioRequests2)
	}

	// Third discovery - verify cache is still working
	t.Log("=== Third discovery (verify cache persistence) ===")
	models3, err3 := client.DiscoverModels(ctx, endpoint)
	if err3 != nil {
		t.Fatalf("Third discovery failed: %v", err3)
	}
	if len(models3) != 1 {
		t.Fatalf("Expected 1 model in third discovery, got %d", len(models3))
	}

	// Final verification
	mu.Lock()
	finalOllamaRequests := requestCount["/api/tags"]
	finalLmStudioRequests := requestCount["/v1/models"]
	mu.Unlock()

	if finalOllamaRequests != 3 {
		t.Errorf("Expected 3 total Ollama requests, got %d", finalOllamaRequests)
	}
	if finalLmStudioRequests != 0 {
		t.Errorf("Expected 0 total LM Studio requests (cache should prevent all), got %d", finalLmStudioRequests)
	}

	t.Logf("✅ Cache working: %d Ollama requests, %d LM Studio requests (expected 3, 0)",
		finalOllamaRequests, finalLmStudioRequests)
}

func TestDiscoverModelsProfileCacheExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cache expiry test in short mode")
	}

	requestCount := make(map[string]int)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount[r.URL.Path]++
		mu.Unlock()

		// Always return success for Ollama
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(200)
			w.Write([]byte(`{"models": [{"name": "test-model"}]}`))
		} else {
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "not found"}`))
		}
	}))
	defer server.Close()

	// Temporarily reduce cache timeout for testing
	originalTimeout := cacheTimeout
	cacheTimeout = 100 * time.Millisecond
	defer func() { cacheTimeout = originalTimeout }()

	endpoint := createTestEndpoint(server.URL, domain.ProfileAuto)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	ctx := context.Background()

	// First discovery
	_, err := client.DiscoverModels(ctx, endpoint)
	if err != nil {
		t.Fatalf("First discovery failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second discovery after expiry - should go through auto-detection again
	_, err = client.DiscoverModels(ctx, endpoint)
	if err != nil {
		t.Fatalf("Second discovery failed: %v", err)
	}

	// Should have made 2 requests to Ollama (cache expired)
	mu.Lock()
	ollamaRequests := requestCount["/api/tags"]
	mu.Unlock()

	if ollamaRequests != 2 {
		t.Errorf("Expected 2 Ollama requests after cache expiry, got %d", ollamaRequests)
	}

	t.Logf("✅ Cache expiry working: %d requests after cache timeout", ollamaRequests)
}

func TestDiscoverModelsProfileCacheInvalidation(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.URL.Path == "/api/tags" {
			if callCount == 1 {
				// First call succeeds
				w.WriteHeader(200)
				w.Write([]byte(`{"models": [{"name": "test-model"}]}`))
			} else {
				// Subsequent calls fail (endpoint changed)
				w.WriteHeader(500)
				w.Write([]byte(`{"error": "server error"}`))
			}
		} else if r.URL.Path == "/v1/models" {
			// LM Studio works after Ollama fails
			w.WriteHeader(200)
			w.Write([]byte(`{
				"object": "list",
				"data": [{"id": "backup-model", "object": "model"}]
			}`))
		} else {
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileAuto)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	ctx := context.Background()

	// First discovery - Ollama works, gets cached
	models1, err1 := client.DiscoverModels(ctx, endpoint)
	if err1 != nil {
		t.Fatalf("First discovery failed: %v", err1)
	}
	if len(models1) != 1 || models1[0].Name != "test-model" {
		t.Fatalf("Expected Ollama model, got %+v", models1)
	}

	// Second discovery - cached Ollama fails, should fall back and cache LM Studio
	models2, err2 := client.DiscoverModels(ctx, endpoint)
	if err2 != nil {
		t.Fatalf("Second discovery failed: %v", err2)
	}
	if len(models2) != 1 || models2[0].Name != "backup-model" {
		t.Fatalf("Expected LM Studio model after fallback, got %+v", models2)
	}

	t.Log("✅ Cache invalidation working: fell back from failed cached profile")
}
func createTestLogger() *logger.StyledLogger {
	slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only log errors to reduce test noise
	}))

	appTheme := &theme.Theme{}

	return logger.NewStyledLogger(slogLogger, appTheme)
}
