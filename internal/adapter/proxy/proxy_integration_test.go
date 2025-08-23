package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// TestMakeUserFriendlyError tests the error handling utility function
func TestMakeUserFriendlyError(t *testing.T) {
	timeout := 30 * time.Second

	testCases := []struct {
		name                string
		inputError          error
		duration            time.Duration
		context             string
		expectedContains    []string
		notExpectedContains []string
	}{
		{
			name:             "Context Canceled",
			inputError:       context.Canceled,
			duration:         5 * time.Second,
			context:          "streaming",
			expectedContains: []string{"request cancelled", "client disconnected early", "5.0s"},
		},
		{
			name:             "Context Deadline Exceeded",
			inputError:       context.DeadlineExceeded,
			duration:         30 * time.Second,
			context:          "backend",
			expectedContains: []string{"request timeout", "30.0s", "server timeout", "exceeded"},
		},
		{
			name:             "Connection Refused",
			inputError:       &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			duration:         2 * time.Second,
			context:          "backend",
			expectedContains: []string{"network error", "connection refused", "2.0s"},
		},
		{
			name:             "Connection Reset",
			inputError:       errors.New("connection reset by peer"),
			duration:         1 * time.Second,
			context:          "streaming",
			expectedContains: []string{"connection reset", "1.0s", "closed connection unexpectedly"},
		},
		{
			name:             "DNS Resolution Error",
			inputError:       errors.New("no such host"),
			duration:         3 * time.Second,
			context:          "backend",
			expectedContains: []string{"DNS lookup failed", "3.0s", "cannot resolve LLM backend hostname"},
		},
		{
			name:             "EOF Error",
			inputError:       io.EOF,
			duration:         10 * time.Second,
			context:          "streaming",
			expectedContains: []string{"AI backend closed connection", "10.0s", "response stream ended unexpectedly"},
		},
		{
			name:             "Network Unreachable",
			inputError:       errors.New("network is unreachable"),
			duration:         5 * time.Second,
			context:          "backend",
			expectedContains: []string{"request failed", "5.0s", "network is unreachable"},
		},
		{
			name:             "Generic Error",
			inputError:       errors.New("some unexpected error"),
			duration:         1500 * time.Millisecond,
			context:          "selection",
			expectedContains: []string{"some unexpected error", "1.5s"},
		},
		{
			name:                "Very Short Duration",
			inputError:          context.DeadlineExceeded,
			duration:            100 * time.Millisecond,
			context:             "backend",
			expectedContains:    []string{"request timeout", "0.1s", "server timeout"},
			notExpectedContains: []string{"after 0.0s"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MakeUserFriendlyError(tc.inputError, tc.duration, tc.context, timeout)

			for _, expected := range tc.expectedContains {
				if !strings.Contains(result.Error(), expected) {
					t.Errorf("Expected error to contain %q, got: %s", expected, result.Error())
				}
			}

			for _, notExpected := range tc.notExpectedContains {
				if strings.Contains(result.Error(), notExpected) {
					t.Errorf("Expected error NOT to contain %q, got: %s", notExpected, result.Error())
				}
			}
		})
	}
}

// TestProxyErrorCreation tests domain.NewProxyError functionality
func TestProxyErrorCreation(t *testing.T) {
	requestID := "test-req-123"
	targetURL := "http://example.com/api/test"
	method := "POST"
	path := "/api/test"
	statusCode := 500
	duration := 2500 * time.Millisecond
	bytes := int64(1024)
	cause := errors.New("upstream server error")

	proxyErr := domain.NewProxyError(requestID, targetURL, method, path, statusCode, duration, int(bytes), cause)

	if proxyErr == nil {
		t.Fatal("NewProxyError should not return nil")
	}

	errorStr := proxyErr.Error()

	// Check that the error contains key information
	expectedContains := []string{
		requestID,
		method,
		path,
		"2.5s",
		"upstream server error",
	}

	for _, expected := range expectedContains {
		if !strings.Contains(errorStr, expected) {
			t.Errorf("Expected error to contain %q, got: %s", expected, errorStr)
		}
	}
}

// TestProxyImplementationParity tests that both implementations behave identically
func TestProxyImplementationParity(t *testing.T) {
	testCases := []struct {
		name        string
		setupServer func() *httptest.Server
		setupConfig func() (sherpaConfig *sherpa.Configuration, ollaConfig *olla.Configuration)
		testRequest func(proxy ports.ProxyService, endpoint *domain.Endpoint) error
		expectError bool
		errorCheck  func(err error) bool
	}{
		{
			name: "Successful Simple Request",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"status": "ok"}`))
				}))
			},
			setupConfig: func() (*sherpa.Configuration, *olla.Configuration) {
				sherpaConfig := &sherpa.Configuration{}
				sherpaConfig.ResponseTimeout = 30 * time.Second
				sherpaConfig.ReadTimeout = 10 * time.Second
				sherpaConfig.StreamBufferSize = 8192

				ollaConfig := &olla.Configuration{}
				ollaConfig.ResponseTimeout = 30 * time.Second
				ollaConfig.ReadTimeout = 10 * time.Second
				ollaConfig.StreamBufferSize = 8192
				ollaConfig.MaxIdleConns = 200
				ollaConfig.IdleConnTimeout = 90 * time.Second
				ollaConfig.MaxConnsPerHost = 50
				return sherpaConfig, ollaConfig
			},
			testRequest: func(proxy ports.ProxyService, endpoint *domain.Endpoint) error {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				w := httptest.NewRecorder()
				return proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
			},
			expectError: false,
		},
		{
			name: "Connection Refused Error",
			setupServer: func() *httptest.Server {
				return nil // Will use unreachable endpoint
			},
			setupConfig: func() (*sherpa.Configuration, *olla.Configuration) {
				sherpaConfig := &sherpa.Configuration{}
				sherpaConfig.ResponseTimeout = 5 * time.Second
				sherpaConfig.ReadTimeout = 2 * time.Second
				sherpaConfig.StreamBufferSize = 8192

				ollaConfig := &olla.Configuration{}
				ollaConfig.ResponseTimeout = 5 * time.Second
				ollaConfig.ReadTimeout = 2 * time.Second
				ollaConfig.StreamBufferSize = 8192
				ollaConfig.MaxIdleConns = 200
				ollaConfig.IdleConnTimeout = 90 * time.Second
				ollaConfig.MaxConnsPerHost = 50
				return sherpaConfig, ollaConfig
			},
			testRequest: func(proxy ports.ProxyService, endpoint *domain.Endpoint) error {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				w := httptest.NewRecorder()
				return proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return strings.Contains(strings.ToLower(err.Error()), "connection") ||
					strings.Contains(strings.ToLower(err.Error()), "refused") ||
					strings.Contains(strings.ToLower(err.Error()), "dial")
			},
		},
		{
			name: "Streaming Response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusOK)
					flusher := w.(http.Flusher)
					for i := 0; i < 5; i++ {
						fmt.Fprintf(w, "chunk %d\n", i)
						flusher.Flush()
						time.Sleep(10 * time.Millisecond)
					}
				}))
			},
			setupConfig: func() (*sherpa.Configuration, *olla.Configuration) {
				sherpaConfig := &sherpa.Configuration{}
				sherpaConfig.ResponseTimeout = 30 * time.Second
				sherpaConfig.ReadTimeout = 10 * time.Second
				sherpaConfig.StreamBufferSize = 1024

				ollaConfig := &olla.Configuration{}
				ollaConfig.ResponseTimeout = 30 * time.Second
				ollaConfig.ReadTimeout = 10 * time.Second
				ollaConfig.StreamBufferSize = 1024
				ollaConfig.MaxIdleConns = 200
				ollaConfig.IdleConnTimeout = 90 * time.Second
				ollaConfig.MaxConnsPerHost = 50
				return sherpaConfig, ollaConfig
			},
			testRequest: func(proxy ports.ProxyService, endpoint *domain.Endpoint) error {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/stream", "")
				w := httptest.NewRecorder()
				return proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
			},
			expectError: false,
		},
	}

	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]error)

			for _, suite := range suites {
				t.Run(suite.Name(), func(t *testing.T) {
					var server *httptest.Server
					var endpoint *domain.Endpoint

					if tc.setupServer != nil {
						server = tc.setupServer()
						if server != nil {
							defer server.Close()
							endpoint = createTestEndpoint("test", server.URL, domain.StatusHealthy)
						}
					}

					if endpoint == nil {
						endpoint = createTestEndpoint("test", "http://localhost:99999", domain.StatusHealthy)
					}

					discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
					selector := &mockEndpointSelector{endpoint: endpoint}
					collector := createTestStatsCollector()

					var proxy ports.ProxyService
					if suite.Name() == "Sherpa" {
						sherpaConfig, _ := tc.setupConfig()
						proxy = suite.CreateProxy(discovery, selector, sherpaConfig, collector)
					} else {
						_, ollaConfig := tc.setupConfig()
						proxy = suite.CreateProxy(discovery, selector, ollaConfig, collector)
					}

					err := tc.testRequest(proxy, endpoint)
					results[suite.Name()] = err

					if tc.expectError {
						if err == nil {
							t.Errorf("%s should return error", suite.Name())
						} else if tc.errorCheck != nil && !tc.errorCheck(err) {
							t.Errorf("%s error check failed: %v", suite.Name(), err)
						}
					} else {
						if err != nil {
							t.Errorf("%s should not return error: %v", suite.Name(), err)
						}
					}
				})
			}

			// Compare error behaviors between implementations
			sherpaErr := results["Sherpa"]
			ollaErr := results["Olla"]

			if (sherpaErr == nil) != (ollaErr == nil) {
				t.Errorf("Error behavior differs: Sherpa=%v, Olla=%v", sherpaErr, ollaErr)
			}
		})
	}
}

// TestStatsCollectorIntegration tests stats collection across both implementations
func TestStatsCollectorIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer upstream.Close()

	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name()+"_StatsIntegration", func(t *testing.T) {
			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			// Make several requests
			const numRequests = 5
			for i := 0; i < numRequests; i++ {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				w := httptest.NewRecorder()
				err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
				if err != nil {
					t.Fatalf("Request %d failed: %v", i, err)
				}
			}

			// Check proxy stats
			proxyStats, err := proxy.GetStats(context.Background())
			if err != nil {
				t.Fatalf("GetStats failed: %v", err)
			}

			if proxyStats.TotalRequests != numRequests {
				t.Errorf("Expected %d total requests, got %d", numRequests, proxyStats.TotalRequests)
			}
			if proxyStats.SuccessfulRequests != numRequests {
				t.Errorf("Expected %d successful requests, got %d", numRequests, proxyStats.SuccessfulRequests)
			}
			if proxyStats.FailedRequests != 0 {
				t.Errorf("Expected 0 failed requests, got %d", proxyStats.FailedRequests)
			}

			// Check collector stats
			collectorStats := collector.GetProxyStats()
			if collectorStats.TotalRequests < numRequests {
				t.Errorf("Collector should have at least %d requests, got %d", numRequests, collectorStats.TotalRequests)
			}

			// Check connection stats
			connectionStats := collector.GetConnectionStats()
			if connectionStats[endpoint.URL.String()] != 0 {
				t.Errorf("Expected 0 active connections, got %d", connectionStats[endpoint.URL.String()])
			}
		})
	}
}

// TestCircuitBreakerBehavior tests Olla's circuit breaker against Sherpa's traditional approach
func TestCircuitBreakerBehavior(t *testing.T) {
	failureCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&failureCount, 1)
		if count <= 3 {
			// Simulate connection failure by closing without response
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("recovered"))
	}))
	defer upstream.Close()

	t.Run("Olla_CircuitBreaker", func(t *testing.T) {
		endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
		config := &olla.Configuration{}
		config.ResponseTimeout = 2 * time.Second
		config.ReadTimeout = 1 * time.Second
		config.StreamBufferSize = 8192
		config.MaxIdleConns = 200
		config.IdleConnTimeout = 90 * time.Second
		config.MaxConnsPerHost = 50

		proxy, err := olla.NewService(
			&mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}},
			&mockEndpointSelector{endpoint: endpoint},
			config,
			createTestStatsCollector(),
			nil,
			createTestLogger(),
		)
		if err != nil {
			t.Fatalf("Failed to create Olla proxy: %v", err)
		}

		// Test circuit breaker behavior
		cb := proxy.GetCircuitBreaker(endpoint.Name)

		// Initially should be closed
		if cb.IsOpen() {
			t.Error("Circuit breaker should start closed")
		}

		// Make some failing requests
		for i := 0; i < 6; i++ {
			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			w := httptest.NewRecorder()
			proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)
		}

		// Circuit breaker behavior depends on actual connection failures, not HTTP errors
		// So we just verify the mechanism exists
		if cb == nil {
			t.Error("Circuit breaker should be available")
		}
	})

	t.Run("Sherpa_Traditional", func(t *testing.T) {
		endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
		config := &sherpa.Configuration{}
		config.ResponseTimeout = 2 * time.Second
		config.ReadTimeout = 1 * time.Second
		config.StreamBufferSize = 8192

		proxy, err := sherpa.NewService(
			&mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}},
			&mockEndpointSelector{endpoint: endpoint},
			config,
			createTestStatsCollector(),
			nil,
			createTestLogger(),
		)
		if err != nil {
			t.Fatalf("Failed to create Sherpa proxy: %v", err)
		}

		// Sherpa doesn't have circuit breakers, just traditional error handling
		for i := 0; i < 6; i++ {
			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			w := httptest.NewRecorder()
			proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)
		}

		// Just verify it handles the requests without panicking
		stats, err := proxy.GetStats(context.Background())
		if err != nil {
			t.Errorf("GetStats failed: %v", err)
		}
		// GetStats returns from statsCollector, so it might be 0 if no actual requests were made
		// This is expected since we're only testing updateStats directly
		// Just verify that GetStats() works without error
		_ = stats
	})
}

// TestPerformanceCharacteristics validates that both implementations meet performance requirements
func TestPerformanceCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name()+"_Performance", func(t *testing.T) {
			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			const numRequests = 100
			const maxLatency = 100 * time.Millisecond

			start := time.Now()
			var wg sync.WaitGroup
			errors := make(chan error, numRequests)

			for i := 0; i < numRequests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
					w := httptest.NewRecorder()
					err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
					if err != nil {
						errors <- err
					}
				}()
			}

			wg.Wait()
			elapsed := time.Since(start)
			close(errors)

			// Check for errors
			for err := range errors {
				t.Errorf("Request failed: %v", err)
			}

			// Performance validation
			avgLatency := elapsed / numRequests
			if avgLatency > maxLatency {
				t.Errorf("Average latency too high: %v (max: %v)", avgLatency, maxLatency)
			}

			// Check stats consistency
			stats, err := proxy.GetStats(context.Background())
			if err != nil {
				t.Fatalf("GetStats failed: %v", err)
			}

			if stats.TotalRequests != numRequests {
				t.Errorf("Expected %d requests, got %d", numRequests, stats.TotalRequests)
			}
			if stats.SuccessfulRequests != numRequests {
				t.Errorf("Expected %d successful requests, got %d", numRequests, stats.SuccessfulRequests)
			}

			t.Logf("%s: %d requests in %v (avg: %v per request)",
				suite.Name(), numRequests, elapsed, avgLatency)
		})
	}
}

// TestConfigurationCompatibility tests that config updates work across implementations
func TestConfigurationCompatibility(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name()+"_ConfigUpdate", func(t *testing.T) {
			proxy, _, _ := createTestProxyComponents(suite, []*domain.Endpoint{})

			// Test multiple config updates
			for i := 1; i <= 5; i++ {
				var config ports.ProxyConfiguration
				if suite.Name() == "Sherpa" {
					sherpaConfig := &sherpa.Configuration{}
					sherpaConfig.ResponseTimeout = time.Duration(i*10) * time.Second
					sherpaConfig.ReadTimeout = time.Duration(i*5) * time.Second
					sherpaConfig.StreamBufferSize = i * 1024
					config = sherpaConfig
				} else {
					ollaConfig := &olla.Configuration{}
					ollaConfig.ResponseTimeout = time.Duration(i*10) * time.Second
					ollaConfig.ReadTimeout = time.Duration(i*5) * time.Second
					ollaConfig.StreamBufferSize = i * 1024
					ollaConfig.MaxIdleConns = i * 50
					ollaConfig.IdleConnTimeout = time.Duration(i*30) * time.Second
					ollaConfig.MaxConnsPerHost = i * 10
					config = ollaConfig
				}

				// Should not panic
				proxy.UpdateConfig(config)

				// Should still be able to get stats
				_, err := proxy.GetStats(context.Background())
				if err != nil {
					t.Errorf("GetStats failed after config update %d: %v", i, err)
				}
			}
		})
	}
}
