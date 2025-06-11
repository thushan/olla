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

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
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
		validateModels func(*testing.T, []*domain.ModelInfo)
	}{
		{
			name:         "Ollama endpoint success with rich metadata",
			endpointType: domain.ProfileOllama,
			serverResponse: `{
				"models": [
					{
						"name": "devstral:latest",
						"size": 14333927918,
						"digest": "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854",
						"modified_at": "2025-05-30T14:24:44.5116551+10:00",
						"details": {
							"parameter_size": "23.6B",
							"quantization_level": "Q4_K_M",
							"family": "llama",
							"format": "gguf"
						}
					},
					{
						"name": "gemma3:12b", 
						"size": 7365960935
					}
				]
			}`,
			serverStatus:   200,
			expectedModels: 2,
			expectedError:  false,
			validateModels: func(t *testing.T, models []*domain.ModelInfo) {
				// Find the devstral model
				var devstralModel *domain.ModelInfo
				for _, model := range models {
					if model.Name == "devstral:latest" {
						devstralModel = model
						break
					}
				}
				if devstralModel == nil {
					t.Fatal("Expected to find devstral:latest model")
				}

				if devstralModel.Details == nil {
					t.Fatal("Expected devstral model to have details")
				}

				if devstralModel.Details.ParameterSize == nil || *devstralModel.Details.ParameterSize != "23.6B" {
					t.Error("Expected devstral parameter_size to be '23.6B'")
				}

				if devstralModel.Details.QuantizationLevel == nil || *devstralModel.Details.QuantizationLevel != "Q4_K_M" {
					t.Error("Expected devstral quantization_level to be 'Q4_K_M'")
				}
			},
		},
		{
			name:         "LM Studio endpoint success with rich metadata",
			endpointType: domain.ProfileLmStudio,
			serverResponse: `{
				"object": "list",
				"data": [
					{
						"id": "meta-llama-3.3-8b-instruct",
						"object": "model",
						"type": "llm",
						"publisher": "lmstudio-community",
						"arch": "llama",
						"compatibility_type": "gguf",
						"quantization": "Q4_K_M",
						"state": "not-loaded",
						"max_context_length": 131072
					},
					{
						"id": "microsoft/DialoGPT-medium", 
						"object": "model"
					}
				]
			}`,
			serverStatus:   200,
			expectedModels: 2,
			expectedError:  false,
			validateModels: func(t *testing.T, models []*domain.ModelInfo) {
				// Find the llama model
				var llamaModel *domain.ModelInfo
				for _, model := range models {
					if model.Name == "meta-llama-3.3-8b-instruct" {
						llamaModel = model
						break
					}
				}
				if llamaModel == nil {
					t.Fatal("Expected to find meta-llama-3.3-8b-instruct model")
				}

				if llamaModel.Details == nil {
					t.Fatal("Expected llama model to have details")
				}

				if llamaModel.Details.Family == nil || *llamaModel.Details.Family != "llama" {
					t.Error("Expected llama family to be 'llama'")
				}

				if llamaModel.Details.QuantizationLevel == nil || *llamaModel.Details.QuantizationLevel != "Q4_K_M" {
					t.Error("Expected llama quantization_level to be 'Q4_K_M'")
				}

				if *llamaModel.Details.MaxContextLength != int64(131072) {
					t.Error("Expected description to contain max context length")
				}
			},
		},
		{
			name:         "OpenAI Compatible endpoint success",
			endpointType: domain.ProfileOpenAICompatible,
			serverResponse: `{
				"object": "list",
				"data": [
					{
						"id": "gpt-3.5-turbo",
						"object": "model",
						"created": 1677610602,
						"owned_by": "openai"
					}
				]
			}`,
			serverStatus:   200,
			expectedModels: 1,
			expectedError:  false,
			validateModels: func(t *testing.T, models []*domain.ModelInfo) {
				model := models[0]
				if model.Name != "gpt-3.5-turbo" {
					t.Errorf("Expected name 'gpt-3.5-turbo', got %s", model.Name)
				}
				if model.Type != "model" {
					t.Errorf("Expected type 'model', got %s", model.Type)
				}
				// OpenAI should have minimal metadata
				if model.Details != nil && model.Details.ModifiedAt != nil {
					expectedTime := time.Unix(1677610602, 0)
					if !model.Details.ModifiedAt.Equal(expectedTime) {
						t.Errorf("Expected created time to be parsed correctly")
					}
				}
			},
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
				expectedUserAgent := fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version)

				if r.Header.Get("User-Agent") != expectedUserAgent {
					t.Errorf("Expected User-Agent header to be %s, got %s", expectedUserAgent, r.Header.Get("User-Agent"))
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

			if tt.validateModels != nil {
				tt.validateModels(t, models)
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
		validateResult  func(*testing.T, []*domain.ModelInfo)
	}{
		{
			name: "Ollama detection success with rich metadata",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 200,
					body: `{
						"models": [
							{
								"name": "llama3.3:8b",
								"size": 4661224676,
								"details": {
									"parameter_size": "8B",
									"quantization_level": "Q4_0",
									"family": "llama"
								}
							}
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
			validateResult: func(t *testing.T, models []*domain.ModelInfo) {
				if len(models) != 1 {
					t.Fatalf("Expected 1 model, got %d", len(models))
				}
				model := models[0]
				if model.Details == nil {
					t.Fatal("Expected Ollama model to have details")
				}
				if model.Details.ParameterSize == nil || *model.Details.ParameterSize != "8B" {
					t.Error("Expected parameter_size to be '8B'")
				}
			},
		},
		{
			name: "LM Studio detection after Ollama fails",
			serverResponses: map[string]serverResponse{
				"/api/tags": {
					status: 404,
					body:   `{"error": "not found"}`,
				},
				"/api/v0/models": { // Changed from "/v1/models"
					status: 200,
					body: `{
                "object": "list",
                "data": [
                    {
                        "id": "qwen3-vl-7b-instruct",
                        "object": "model",
                        "type": "vlm",
                        "arch": "qwen3_vl",
                        "compatibility_type": "mlx",
                        "quantization": "4bit",
                        "max_context_length": 32768
                    }
                ]
            }`,
				},
				"/v1/models": { // Add this as 404 to prevent OpenAI fallback
					status: 404,
					body:   `{"error": "not found"}`,
				},
			},
			expectedModels: 1,
			expectedError:  false,
			validateResult: func(t *testing.T, models []*domain.ModelInfo) {
				if len(models) != 1 {
					t.Fatalf("Expected 1 model, got %d", len(models))
				}
				model := models[0]
				if model.Details == nil {
					t.Fatal("Expected LM Studio model to have details")
				}
				if model.Details.Family == nil || *model.Details.Family != "qwen3_vl" {
					t.Error("Expected family to be 'qwen3_vl'")
				}
				if model.Details.QuantizationLevel == nil || *model.Details.QuantizationLevel != "4bit" {
					t.Error("Expected quantization_level to be '4bit'")
				}
			},
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
							{
								"id": "text-davinci-003",
								"object": "model",
								"created": 1669599635,
								"owned_by": "openai-internal"
							}
						]
					}`,
				},
			},
			expectedModels: 1,
			expectedError:  false,
			validateResult: func(t *testing.T, models []*domain.ModelInfo) {
				if len(models) != 1 {
					t.Fatalf("Expected 1 model, got %d", len(models))
				}
				model := models[0]
				if model.Name != "text-davinci-003" {
					t.Errorf("Expected name 'text-davinci-003', got %s", model.Name)
				}
				// OpenAI compatible should have minimal metadata
				if model.Details != nil && model.Details.ModifiedAt != nil {
					expectedTime := time.Unix(1669599635, 0)
					if !model.Details.ModifiedAt.Equal(expectedTime) {
						t.Error("Expected created timestamp to be parsed")
					}
				}
			},
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
			name: "Non-recoverable parse error stops detection",
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

			// Use auto detection
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

			// Run custom validation if provided
			if tt.validateResult != nil {
				tt.validateResult(t, models)
			}
		})
	}
}

func TestDiscoverModelsContextCancellation(t *testing.T) {
	// Create server with delay, so we can cancel the context
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"models": []}`))
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileOllama)
	client := NewHTTPModelDiscoveryClient(profile.NewFactory(), createTestLogger())

	// Create context with short timeout
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
			t.Errorf("Expected NetworkError, got %T", discErr.Err)
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

	// Make sure everything is zero before starting
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
		t.Errorf("Expected TotalDiscoveries to be 1, got %d", metrics.TotalDiscoveries)
	}
	if metrics.SuccessfulRequests != 1 {
		t.Errorf("Expected SuccessfulRequests to be 1, got %d", metrics.SuccessfulRequests)
	}
	if metrics.LastDiscoveryTime.IsZero() {
		t.Errorf("Expected LastDiscoveryTime to be set")
	}
}

func TestMaxResponseSizeLimit(t *testing.T) {
	// Create response larger than MaxResponseSize
	// Cheapest way to create this payload
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
			// Ollama endpoint - return success with rich metadata
			w.WriteHeader(200)
			w.Write([]byte(`{
				"models": [
					{
						"name": "llama3:8b",
						"size": 4661224676,
						"details": {
							"parameter_size": "8B",
							"quantization_level": "Q4_0",
							"family": "llama"
						}
					}
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

	// Validate rich metadata was parsed
	if models1[0].Details == nil {
		t.Fatal("Expected first model to have details")
	}
	if models1[0].Details.ParameterSize == nil || *models1[0].Details.ParameterSize != "8B" {
		t.Error("Expected parameter_size to be '8B'")
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

	t.Logf("Cache working: %d Ollama requests, %d LM Studio requests (expected 3, 0)",
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

	t.Logf("Cache expiry working: %d requests after cache timeout", ollamaRequests)
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
		} else if r.URL.Path == "/api/v0/models" {
			// LM Studio works after Ollama fails
			w.WriteHeader(200)
			w.Write([]byte(`{
        "object": "list",
        "data": [
            {
                "id": "backup-model",
                "object": "model",
                "arch": "backup",
                "quantization": "Q8_0",
                "type": "llm",
                "state": "loaded",
                "max_context_length": 4096
            }
        ]
    }`))
		} else if r.URL.Path == "/v1/models" { // Add this to prevent OpenAI fallback
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "not found"}`))
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

	// Validate that LM Studio metadata was parsed
	if models2[0].Details == nil {
		t.Fatal("Expected LM Studio model to have details")
	}
	if models2[0].Details.Family == nil || *models2[0].Details.Family != "backup" {
		t.Error("Expected LM Studio family to be parsed from arch")
	}

	t.Log("Cache invalidation working: fell back from failed cached profile")
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

func createTestLogger() logger.StyledLogger {
	slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only log errors to reduce test noise
	}))

	return logger.NewPlainStyledLogger(slogLogger)
}
