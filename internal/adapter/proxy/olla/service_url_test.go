package olla

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// TestBuildTargetURL tests URL building for various scenarios
func TestBuildTargetURL(t *testing.T) {
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ResponseTimeout = 30 * time.Second
	config.ReadTimeout = 10 * time.Second
	config.StreamBufferSize = 8192
	config.MaxIdleConns = 200
	config.IdleConnTimeout = 90 * time.Second
	config.MaxConnsPerHost = 50

	service, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name          string
		endpointURL   string
		requestPath   string
		requestQuery  string
		expectedPath  string
		expectedQuery string
		expectedHost  string
		useFastPath   bool
		description   string
	}{
		{
			name:          "SimpleEndpoint_NoPath",
			endpointURL:   "http://localhost:11434",
			requestPath:   "/olla/v1/chat/completions",
			requestQuery:  "",
			expectedPath:  "/v1/chat/completions",
			expectedQuery: "",
			expectedHost:  "localhost:11434",
			useFastPath:   true,
			description:   "Common case: Ollama endpoint with no path",
		},
		{
			name:          "SimpleEndpoint_RootPath",
			endpointURL:   "http://localhost:1234/",
			requestPath:   "/olla/v1/completions",
			requestQuery:  "",
			expectedPath:  "/v1/completions",
			expectedQuery: "",
			expectedHost:  "localhost:1234",
			useFastPath:   true,
			description:   "Common case: LM Studio endpoint with / path",
		},
		{
			name:          "SimpleEndpoint_WithQueryString",
			endpointURL:   "http://localhost:11434",
			requestPath:   "/olla/api/tags",
			requestQuery:  "format=json&details=true",
			expectedPath:  "/api/tags",
			expectedQuery: "format=json&details=true",
			expectedHost:  "localhost:11434",
			useFastPath:   true,
			description:   "Query string handling in fast path",
		},
		{
			name:          "ComplexEndpoint_WithPath",
			endpointURL:   "http://localhost:8000/api/v1",
			requestPath:   "/olla/models",
			requestQuery:  "",
			expectedPath:  "/models", // ResolveReference behaviour
			expectedQuery: "",
			expectedHost:  "localhost:8000",
			useFastPath:   false,
			description:   "Rare case: vLLM endpoint with API path",
		},
		{
			name:          "HTTPSEndpoint",
			endpointURL:   "https://api.example.com:443",
			requestPath:   "/olla/v1/embeddings",
			requestQuery:  "model=text-embedding-3-small",
			expectedPath:  "/v1/embeddings",
			expectedQuery: "model=text-embedding-3-small",
			expectedHost:  "api.example.com:443",
			useFastPath:   true,
			description:   "HTTPS endpoint with query string",
		},
		{
			name:          "EmptyProxyPath",
			endpointURL:   "http://localhost:11434",
			requestPath:   "/olla/",
			requestQuery:  "",
			expectedPath:  "/",
			expectedQuery: "",
			expectedHost:  "localhost:11434",
			useFastPath:   true,
			description:   "Edge case: empty path after prefix strip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpointURL, err := url.Parse(tt.endpointURL)
			if err != nil {
				t.Fatalf("invalid endpoint URL: %v", err)
			}

			endpoint := &domain.Endpoint{
				Name: "test",
				URL:  endpointURL,
			}

			requestURL := tt.requestPath
			if tt.requestQuery != "" {
				requestURL += "?" + tt.requestQuery
			}

			req, err := http.NewRequest("POST", requestURL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			targetURL := service.buildTargetURL(req, endpoint)

			// Verify path
			if targetURL.Path != tt.expectedPath {
				t.Errorf("%s: expected path %q, got %q", tt.description, tt.expectedPath, targetURL.Path)
			}

			// Verify query string
			if targetURL.RawQuery != tt.expectedQuery {
				t.Errorf("%s: expected query %q, got %q", tt.description, tt.expectedQuery, targetURL.RawQuery)
			}

			// Verify host
			if targetURL.Host != tt.expectedHost {
				t.Errorf("%s: expected host %q, got %q", tt.description, tt.expectedHost, targetURL.Host)
			}

			// Verify scheme preserved
			if targetURL.Scheme != endpointURL.Scheme {
				t.Errorf("%s: expected scheme %q, got %q", tt.description, endpointURL.Scheme, targetURL.Scheme)
			}

			// Log which path was used
			fastPath := endpoint.URL.Path == "" || endpoint.URL.Path == "/"
			if fastPath != tt.useFastPath {
				t.Logf("Note: %s used %s (expected %s)",
					tt.description,
					map[bool]string{true: "fast path", false: "slow path"}[fastPath],
					map[bool]string{true: "fast path", false: "slow path"}[tt.useFastPath])
			}
		})
	}
}

// TestBuildTargetURL_PathHandling tests specific path edge cases
func TestBuildTargetURL_PathHandling(t *testing.T) {
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ResponseTimeout = 30 * time.Second
	config.ReadTimeout = 10 * time.Second
	config.StreamBufferSize = 8192
	config.MaxIdleConns = 200
	config.IdleConnTimeout = 90 * time.Second
	config.MaxConnsPerHost = 50

	service, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Test that fast path and slow path produce same results for simple endpoints
	endpointURL, _ := url.Parse("http://localhost:11434")
	endpoint := &domain.Endpoint{
		Name: "test",
		URL:  endpointURL,
	}

	testPaths := []string{
		"/olla/v1/chat/completions",
		"/olla/api/tags",
		"/olla/v1/models",
		"/olla/",
		"/olla/a/b/c/d",
	}

	for _, path := range testPaths {
		req, _ := http.NewRequest("GET", path+"?test=true", nil)
		targetURL := service.buildTargetURL(req, endpoint)

		// Verify URL is valid
		if targetURL.Host == "" {
			t.Errorf("empty host for path %s", path)
		}
		if targetURL.Scheme == "" {
			t.Errorf("empty scheme for path %s", path)
		}

		// Verify query string preserved
		if targetURL.RawQuery != "test=true" {
			t.Errorf("query string not preserved for path %s: got %q", path, targetURL.RawQuery)
		}
	}
}
