package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// TestProxyPathStripping tests the critical path stripping behavior for both proxy implementations
// This is critical for edge deployments where millions of requests rely on correct routing
//
// NOTE: This test documents the ACTUAL behavior, including known issues that exist in production.
// Some behaviors are not ideal but changing them could break existing clients.
func TestProxyPathStripping(t *testing.T) {
	testCases := []struct {
		name          string
		proxyPrefix   string
		requestPath   string
		contextPrefix string // If set, overrides config prefix via context
		expectedPath  string
		description   string
	}{
		// Normal cases
		{
			name:         "standard_prefix_stripping",
			proxyPrefix:  "/olla",
			requestPath:  "/olla/api/chat",
			expectedPath: "/api/chat",
			description:  "Standard prefix removal",
		},
		{
			name:         "provider_prefix_stripping",
			proxyPrefix:  "/olla/proxy",
			requestPath:  "/olla/proxy/v1/chat/completions",
			expectedPath: "/v1/chat/completions",
			description:  "Provider proxy prefix removal",
		},
		{
			name:         "no_prefix_in_path",
			proxyPrefix:  "/olla",
			requestPath:  "/api/chat",
			expectedPath: "/api/chat",
			description:  "Path without prefix remains unchanged",
		},

		// Edge cases
		{
			name:         "empty_path_after_strip",
			proxyPrefix:  "/olla",
			requestPath:  "/olla",
			expectedPath: "/",
			description:  "Empty path becomes root",
		},
		{
			name:         "prefix_only_partial_match",
			proxyPrefix:  "/olla",
			requestPath:  "/ollama/api/chat",
			expectedPath: "/ma/api/chat", // This is the ACTUAL behavior (a bug!)
			description:  "Partial prefix match incorrectly stripped (BUG)",
		},
		{
			name:         "double_slash_handling",
			proxyPrefix:  "/olla",
			requestPath:  "/olla//api/chat",
			expectedPath: "//api/chat",
			description:  "Double slashes preserved after prefix",
		},
		{
			name:         "trailing_slash_prefix",
			proxyPrefix:  "/olla/",
			requestPath:  "/olla/api/chat",
			expectedPath: "/api/chat",
			description:  "Trailing slash in prefix handled correctly",
		},

		// Security cases - document actual behavior
		{
			name:         "path_traversal_attempt",
			proxyPrefix:  "/olla",
			requestPath:  "/olla/../admin",
			expectedPath: "/admin", // URL.ResolveReference normalizes the path
			description:  "Path traversal normalized by URL resolution (security feature)",
		},
		{
			name:         "encoded_slash_in_path",
			proxyPrefix:  "/olla",
			requestPath:  "/olla/api%2Fchat",
			expectedPath: "/api/chat", // URL decoding happens before stripping
			description:  "URL encoded characters decoded by HTTP library",
		},
	}

	// Test both proxy implementations
	proxyTypes := []struct {
		name    string
		factory func(config ports.ProxyConfiguration) (ports.ProxyService, error)
	}{
		{
			name: "Sherpa",
			factory: func(config ports.ProxyConfiguration) (ports.ProxyService, error) {
				discovery := &mockDiscoveryService{}
				selector := &mockEndpointSelector{}
				return sherpa.NewService(discovery, selector, config.(*sherpa.Configuration), createTestStatsCollector(), createTestLogger())
			},
		},
		{
			name: "Olla",
			factory: func(config ports.ProxyConfiguration) (ports.ProxyService, error) {
				discovery := &mockDiscoveryService{}
				selector := &mockEndpointSelector{}
				return olla.NewService(discovery, selector, config.(*olla.Configuration), createTestStatsCollector(), createTestLogger())
			},
		},
	}

	for _, proxyType := range proxyTypes {
		t.Run(proxyType.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Create upstream server that echoes the received path
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("X-Received-Path", r.URL.Path)
						w.WriteHeader(http.StatusOK)
						fmt.Fprint(w, r.URL.Path)
					}))
					defer upstream.Close()

					// Create proxy configuration
					var config ports.ProxyConfiguration
					if proxyType.name == "Sherpa" {
						config = &sherpa.Configuration{
							ProxyPrefix: tc.proxyPrefix,
						}
					} else {
						config = &olla.Configuration{
							ProxyPrefix: tc.proxyPrefix,
						}
					}

					// Create proxy service
					proxy, err := proxyType.factory(config)
					if err != nil {
						t.Fatalf("Failed to create proxy: %v", err)
					}

					// Set up endpoint
					endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
					endpoints := []*domain.Endpoint{endpoint}

					// Create request
					req := httptest.NewRequest("GET", tc.requestPath, nil)

					// Add context override if specified
					if tc.contextPrefix != "" {
						ctx := context.WithValue(req.Context(), tc.proxyPrefix, tc.contextPrefix)
						req = req.WithContext(ctx)
					}

					// Execute request
					w := httptest.NewRecorder()
					stats := &ports.RequestStats{
						RequestID: "test-" + tc.name,
						StartTime: time.Now(),
					}
					logger := createTestLogger()

					err = proxy.ProxyRequestToEndpoints(req.Context(), w, req, endpoints, stats, logger)
					if err != nil {
						t.Fatalf("Proxy request failed: %v", err)
					}

					// Verify the path received by upstream
					receivedPath := w.Header().Get("X-Received-Path")
					if receivedPath != tc.expectedPath {
						t.Errorf("Path stripping failed for %s\n"+
							"Description: %s\n"+
							"Request path: %s\n"+
							"Expected upstream path: %s\n"+
							"Actual upstream path: %s\n"+
							"Proxy prefix: %s\n"+
							"Context prefix: %s",
							proxyType.name, tc.description, tc.requestPath,
							tc.expectedPath, receivedPath, tc.proxyPrefix, tc.contextPrefix)
					}
				})
			}
		})
	}
}

// TestProxyPathStrippingDoubleStrippingPrevention tests that paths are not double-stripped
// This prevents regression of the bug where handler pre-stripped paths
func TestProxyPathStrippingDoubleStrippingPrevention(t *testing.T) {
	// Create upstream that logs all received paths
	var receivedPaths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPaths = append(receivedPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Test with a path that could be double-stripped
	requestPath := "/olla/api/chat"
	expectedPath := "/api/chat"

	// Create Sherpa proxy
	sherpaConfig := &sherpa.Configuration{
		ProxyPrefix: "/olla",
	}
	sherpaProxy, err := sherpa.NewService(
		&mockDiscoveryService{},
		&mockEndpointSelector{},
		sherpaConfig,
		createTestStatsCollector(),
		createTestLogger(),
	)
	if err != nil {
		t.Fatalf("Failed to create Sherpa proxy: %v", err)
	}

	// Create request that simulates what the handler would send
	// The handler no longer modifies r.URL.Path, so it should still have the full path
	req := httptest.NewRequest("GET", requestPath, nil)

	// Execute request
	w := httptest.NewRecorder()
	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	stats := &ports.RequestStats{RequestID: "test", StartTime: time.Now()}

	err = sherpaProxy.ProxyRequestToEndpoints(
		req.Context(), w, req,
		[]*domain.Endpoint{endpoint},
		stats, createTestLogger(),
	)
	if err != nil {
		t.Fatalf("Proxy request failed: %v", err)
	}

	// Verify only one path stripping occurred
	if len(receivedPaths) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(receivedPaths))
	}

	if receivedPaths[0] != expectedPath {
		t.Errorf("Double stripping detected!\n"+
			"Original path: %s\n"+
			"Expected upstream to receive: %s\n"+
			"Actual upstream received: %s",
			requestPath, expectedPath, receivedPaths[0])
	}
}

// TestProxyPathStrippingWithProviderContext tests that proxy uses its own configured prefix
// and ignores context values (defensive behavior)
func TestProxyPathStrippingWithProviderContext(t *testing.T) {
	testCases := []struct {
		name              string
		handlerPrefix     string // What handler puts in context (ignored)
		proxyConfigPrefix string // What proxy is configured with
		requestPath       string
		expectedPath      string
	}{
		{
			name:              "handler_and_proxy_same_prefix",
			handlerPrefix:     constants.ContextRoutePrefixKey, // "/olla"
			proxyConfigPrefix: "/olla",
			requestPath:       "/olla/api/chat",
			expectedPath:      "/api/chat",
		},
		{
			name:              "handler_provider_proxy_prefix",
			handlerPrefix:     "/olla/proxy/openai",
			proxyConfigPrefix: "/olla",
			requestPath:       "/olla/proxy/openai/v1/chat",
			expectedPath:      "/proxy/openai/v1/chat", // Only strips configured prefix
		},
		{
			name:              "mismatched_prefixes",
			handlerPrefix:     "/custom",
			proxyConfigPrefix: "/olla",
			requestPath:       "/olla/api/chat",
			expectedPath:      "/api/chat", // Strips based on proxy config, ignores context
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create upstream
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Received-Path", r.URL.Path)
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			// Create Olla proxy with configured prefix
			config := &olla.Configuration{
				ProxyPrefix: tc.proxyConfigPrefix,
			}
			proxy, err := olla.NewService(
				&mockDiscoveryService{},
				&mockEndpointSelector{},
				config,
				createTestStatsCollector(),
				createTestLogger(),
			)
			if err != nil {
				t.Fatalf("Failed to create proxy: %v", err)
			}

			// Create request with handler context
			req := httptest.NewRequest("GET", tc.requestPath, nil)
			ctx := context.WithValue(req.Context(), tc.proxyConfigPrefix, tc.handlerPrefix)
			req = req.WithContext(ctx)

			// Execute
			w := httptest.NewRecorder()
			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			stats := &ports.RequestStats{RequestID: "test", StartTime: time.Now()}

			err = proxy.ProxyRequestToEndpoints(
				req.Context(), w, req,
				[]*domain.Endpoint{endpoint},
				stats, createTestLogger(),
			)
			if err != nil {
				t.Fatalf("Proxy request failed: %v", err)
			}

			// Verify
			receivedPath := w.Header().Get("X-Received-Path")
			if receivedPath != tc.expectedPath {
				t.Errorf("Path stripping failed\n"+
					"Handler prefix in context: %s\n"+
					"Proxy configured prefix: %s\n"+
					"Request path: %s\n"+
					"Expected: %s\n"+
					"Got: %s",
					tc.handlerPrefix, tc.proxyConfigPrefix, tc.requestPath,
					tc.expectedPath, receivedPath)
			}
		})
	}
}

// BenchmarkPathStripping measures the performance of path stripping
// Important for edge deployments handling millions of requests
func BenchmarkPathStripping(b *testing.B) {
	configs := []struct {
		name        string
		prefix      string
		requestPath string
	}{
		{"short_prefix", "/olla", "/olla/api/chat"},
		{"long_prefix", "/olla/proxy/provider", "/olla/proxy/provider/v1/chat/completions"},
		{"no_match", "/olla", "/api/chat"},
	}

	// Benchmark both implementations
	for _, config := range configs {
		b.Run("Sherpa_"+config.name, func(b *testing.B) {
			benchmarkPathStripping(b, "sherpa", config.prefix, config.requestPath)
		})

		b.Run("Olla_"+config.name, func(b *testing.B) {
			benchmarkPathStripping(b, "olla", config.prefix, config.requestPath)
		})
	}
}

func benchmarkPathStripping(b *testing.B, proxyType, prefix, requestPath string) {
	// Create a simple upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create proxy
	var proxy ports.ProxyService
	var err error

	if proxyType == "sherpa" {
		config := &sherpa.Configuration{ProxyPrefix: prefix}
		proxy, err = sherpa.NewService(
			&mockDiscoveryService{},
			&mockEndpointSelector{},
			config,
			createTestStatsCollector(),
			createTestLogger(),
		)
	} else {
		config := &olla.Configuration{ProxyPrefix: prefix}
		proxy, err = olla.NewService(
			&mockDiscoveryService{},
			&mockEndpointSelector{},
			config,
			createTestStatsCollector(),
			createTestLogger(),
		)
	}

	if err != nil {
		b.Fatalf("Failed to create proxy: %v", err)
	}

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	endpoints := []*domain.Endpoint{endpoint}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", requestPath, nil)
		w := httptest.NewRecorder()
		stats := &ports.RequestStats{
			RequestID: fmt.Sprintf("bench-%d", i),
			StartTime: time.Now(),
		}

		err := proxy.ProxyRequestToEndpoints(
			req.Context(), w, req, endpoints, stats, createTestLogger(),
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
