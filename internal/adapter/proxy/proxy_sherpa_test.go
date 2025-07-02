package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// Helper functions for creating test components
func createTestSherpaProxy(endpoints []*domain.Endpoint) (*SherpaProxyService, *mockEndpointSelector, ports.StatsCollector) {
	collector := createTestStatsCollector()
	discovery := &mockDiscoveryService{endpoints: endpoints}
	selector := newMockEndpointSelector(collector)
	config := &Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 1024,
	}
	proxy := NewSherpaService(discovery, selector, config, collector, createTestLogger())
	return proxy, selector, collector
}

func createTestSherpaProxyWithConfig(endpoints []*domain.Endpoint, config *Configuration) (*SherpaProxyService, *mockEndpointSelector, ports.StatsCollector) {
	collector := createTestStatsCollector()
	discovery := &mockDiscoveryService{endpoints: endpoints}
	selector := newMockEndpointSelector(collector)
	proxy := NewSherpaService(discovery, selector, config, collector, createTestLogger())
	return proxy, selector, collector
}

func createTestRequestWithStats(method, path, body string) (*http.Request, *ports.RequestStats, logger.StyledLogger) {
	ctx := context.WithValue(context.Background(), constants.RequestIDKey, "test-request-id")
	ctx = context.WithValue(ctx, constants.RequestTimeKey, time.Now())

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req = req.WithContext(ctx)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	stats := &ports.RequestStats{
		RequestID: "test-request-id",
		StartTime: time.Now(),
	}

	logger := createTestLogger()
	return req, stats, logger
}

func assertSuccessfulResponse(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedBodyContains string) {
	t.Helper()
	if w.Code != expectedStatus {
		t.Errorf("Expected status %d, got %d", expectedStatus, w.Code)
	}
	if expectedBodyContains != "" && !strings.Contains(w.Body.String(), expectedBodyContains) {
		t.Errorf("Expected body to contain %q, got %q", expectedBodyContains, w.Body.String())
	}
}

func assertError(t *testing.T, err error, expectedErrorContains string) {
	t.Helper()
	if err == nil {
		t.Error("Expected error but got nil")
		return
	}
	if expectedErrorContains != "" && !strings.Contains(err.Error(), expectedErrorContains) {
		t.Errorf("Expected error to contain %q, got %q", expectedErrorContains, err.Error())
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

func assertStatsPopulated(t *testing.T, stats *ports.RequestStats, expectedEndpoint string) {
	t.Helper()
	if expectedEndpoint != "" && stats.EndpointName != expectedEndpoint {
		t.Errorf("Expected endpoint name %q, got %q", expectedEndpoint, stats.EndpointName)
	}
	if stats.TotalBytes <= 0 {
		t.Error("Expected non-zero bytes transferred")
	}
	if stats.Latency < 0 {
		t.Errorf("Expected positive or zero latency but got %v, how is that possible? o_O", stats.Latency)
	}
}

// TestSherpaProxyService_ClientDisconnectLogic tests the specific logic for handling client disconnections
func TestSherpaProxyService_ClientDisconnectLogic(t *testing.T) {
	proxy := &SherpaProxyService{}

	testCases := []struct {
		name              string
		bytesRead         int
		timeSinceLastRead time.Duration
		shouldContinue    bool
	}{
		{"Large response, recent activity", 2000, 2 * time.Second, true},
		{"Small response, recent activity", 500, 2 * time.Second, false},
		{"Large response, stale", 2000, 10 * time.Second, false},
		{"No data", 0, 1 * time.Second, false},
		{"Exactly threshold bytes, recent", ClientDisconnectionBytesThreshold, 2 * time.Second, false},
		{"Just over threshold, recent", ClientDisconnectionBytesThreshold + 1, 2 * time.Second, true},
		{"Large response, exactly threshold time", 2000, ClientDisconnectionTimeThreshold, false},
		{"Large response, just under threshold time", 2000, ClientDisconnectionTimeThreshold - time.Millisecond, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := proxy.shouldContinueAfterClientDisconnect(tc.bytesRead, tc.timeSinceLastRead)
			if result != tc.shouldContinue {
				t.Errorf("Expected %v, got %v for %s", tc.shouldContinue, result, tc.name)
			}
		})
	}
}

// TestSherpaProxyService_StripRoutePrefix tests the route prefix stripping functionality
func TestSherpaProxyService_StripRoutePrefix(t *testing.T) {
	proxy := &SherpaProxyService{
		configuration: &Configuration{ProxyPrefix: constants.ProxyPathPrefix},
	}

	testCases := []struct {
		inputPath    string
		routePrefix  string
		expectedPath string
	}{
		{"/proxy/api/models", "/proxy/", "/api/models"},
		{"/olla/api/chat", "/olla/", "/api/chat"},
		{"/api/models", "/proxy/", "/api/models"}, // No prefix to strip
		{"/proxy/", "/proxy/", "/"},
		{"/proxy", "/proxy/", "/proxy"}, // Doesn't match prefix exactly
		{"", "/proxy/", ""},             // Empty path stays empty
		{"/proxy/api/v1/models", "/proxy/", "/api/v1/models"},
		{"/different/api/models", "/proxy/", "/different/api/models"}, // Different prefix
		{"/proxy/api", "/proxy/", "/api"},
		{"/proxyapi/models", "/proxy/", "/proxyapi/models"}, // Partial match shouldn't strip
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s->%s", tc.inputPath, tc.expectedPath), func(t *testing.T) {
			ctx := context.WithValue(context.Background(), constants.ProxyPathPrefix, tc.routePrefix)
			result := proxy.stripRoutePrefix(ctx, tc.inputPath)

			if result != tc.expectedPath {
				t.Errorf("Expected %q, got %q", tc.expectedPath, result)
			}
		})
	}
}

// TestSherpaProxyService_StripRoutePrefix_NoContext tests behaviour when context doesn't contain prefix
func TestSherpaProxyService_StripRoutePrefix_NoContext(t *testing.T) {
	proxy := &SherpaProxyService{
		configuration: &Configuration{ProxyPrefix: constants.ProxyPathPrefix},
	}

	testPath := "/proxy/api/models"
	ctx := context.Background() // No prefix in context

	result := proxy.stripRoutePrefix(ctx, testPath)
	if result != testPath {
		t.Errorf("Expected original path %q when no prefix in context, got %q", testPath, result)
	}
}

// TestSherpaProxyService_CopyHeaders tests header copying functionality
func TestSherpaProxyService_CopyHeaders(t *testing.T) {
	proxy := &SherpaProxyService{}

	originalReq := httptest.NewRequest("POST", "/api/test", strings.NewReader("test body"))
	originalReq.Header.Set("Content-Type", "application/json")
	originalReq.Header.Set("Authorization", "Bearer token123")
	originalReq.Header.Set("X-Custom-Header", "custom-value")
	originalReq.Header.Set("User-Agent", "test-client/1.0")
	originalReq.Header.Add("Accept", "application/json")
	originalReq.Header.Add("Accept", "text/plain") // Multiple values
	originalReq.RemoteAddr = "192.168.1.100:12345"

	proxyReq := httptest.NewRequest("POST", "http://upstream/api/test", strings.NewReader("test body"))

	proxy.copyHeaders(proxyReq, originalReq)

	// Check original headers were copied
	if proxyReq.Header.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header not copied")
	}
	if proxyReq.Header.Get("Authorization") != "" {
		t.Error("Authorization header should be blocked for security")
	}
	if proxyReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header not copied")
	}
	if proxyReq.Header.Get("User-Agent") != "test-client/1.0" {
		t.Error("User-Agent header not copied")
	}

	// Check multiple values are preserved
	acceptValues := proxyReq.Header["Accept"]
	if len(acceptValues) != 2 || acceptValues[0] != "application/json" || acceptValues[1] != "text/plain" {
		t.Errorf("Accept header values not copied correctly, got %v", acceptValues)
	}

	// Check forwarding headers were added
	if proxyReq.Header.Get("X-Forwarded-Host") != originalReq.Host {
		t.Error("X-Forwarded-Host not set correctly")
	}
	if proxyReq.Header.Get("X-Forwarded-Proto") != "http" {
		t.Error("X-Forwarded-Proto not set correctly")
	}
	if proxyReq.Header.Get("X-Forwarded-For") != "192.168.1.100" {
		t.Error("X-Forwarded-For not set correctly")
	}
	if proxyReq.Header.Get("X-Proxied-By") == "" {
		t.Error("X-Proxied-By not set")
	}
	if proxyReq.Header.Get("Via") == "" {
		t.Error("Via header not set")
	}

	// Check version info is included
	proxyByHeader := proxyReq.Header.Get("X-Proxied-By")
	if !strings.Contains(proxyByHeader, "/") {
		t.Error("X-Proxied-By should contain version info")
	}
	viaHeader := proxyReq.Header.Get("Via")
	if !strings.Contains(viaHeader, "/") {
		t.Error("Via header should contain version info")
	}
}

// TestSherpaProxyService_CopyHeaders_HTTPS tests HTTPS protocol detection
func TestSherpaProxyService_CopyHeaders_HTTPS(t *testing.T) {
	proxy := &SherpaProxyService{}

	originalReq := httptest.NewRequest("GET", "https://example.com/api/test", nil)
	originalReq.TLS = &tls.ConnectionState{} // Simulate HTTPS
	originalReq.RemoteAddr = "10.0.0.1:54321"

	proxyReq := httptest.NewRequest("GET", "http://upstream/api/test", nil)

	proxy.copyHeaders(proxyReq, originalReq)

	if proxyReq.Header.Get("X-Forwarded-Proto") != "https" {
		t.Errorf("Expected X-Forwarded-Proto to be 'https', got '%s'", proxyReq.Header.Get("X-Forwarded-Proto"))
	}
	if proxyReq.Header.Get("X-Forwarded-For") != "10.0.0.1" {
		t.Errorf("Expected X-Forwarded-For to be '10.0.0.1', got '%s'", proxyReq.Header.Get("X-Forwarded-For"))
	}
}

// TestSherpaProxyService_CopyHeaders_MalformedRemoteAddr tests handling of malformed remote addresses
func TestSherpaProxyService_CopyHeaders_MalformedRemoteAddr(t *testing.T) {
	proxy := &SherpaProxyService{}

	originalReq := httptest.NewRequest("GET", "/api/test", nil)
	originalReq.RemoteAddr = "malformed-address" // No port

	proxyReq := httptest.NewRequest("GET", "http://upstream/api/test", nil)

	proxy.copyHeaders(proxyReq, originalReq)

	// X-Forwarded-For should not be set if RemoteAddr is malformed
	if proxyReq.Header.Get("X-Forwarded-For") != "" {
		t.Errorf("Expected X-Forwarded-For to be empty for malformed address, got '%s'", proxyReq.Header.Get("X-Forwarded-For"))
	}

	// Other headers should still be set
	if proxyReq.Header.Get("X-Forwarded-Host") == "" {
		t.Error("X-Forwarded-Host should still be set")
	}
	if proxyReq.Header.Get("X-Forwarded-Proto") == "" {
		t.Error("X-Forwarded-Proto should still be set")
	}
}

// TestSherpaProxyService_CopyHeaders_EmptyHeaders tests behaviour with no headers
func TestSherpaProxyService_CopyHeaders_EmptyHeaders(t *testing.T) {
	proxy := &SherpaProxyService{}

	originalReq := httptest.NewRequest("GET", "/api/test", nil)
	originalReq.RemoteAddr = "192.168.1.1:8080"

	proxyReq := httptest.NewRequest("GET", "http://upstream/api/test", nil)

	proxy.copyHeaders(proxyReq, originalReq)

	// Standard forwarding headers should still be added
	if proxyReq.Header.Get("X-Forwarded-Host") == "" {
		t.Error("X-Forwarded-Host should be set even with no original headers")
	}
	if proxyReq.Header.Get("X-Forwarded-Proto") != "http" {
		t.Error("X-Forwarded-Proto should be 'http'")
	}
	if proxyReq.Header.Get("X-Forwarded-For") != "192.168.1.1" {
		t.Error("X-Forwarded-For should be '192.168.1.1'")
	}
}

// TestSherpaProxyService_StreamResponse_ReadTimeout tests read timeout behaviour
func TestSherpaProxyService_StreamResponse_ReadTimeout(t *testing.T) {
	// Create a slow upstream that takes longer than read timeout
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		// Send some data then pause longer than read timeout
		w.Write([]byte("initial data"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(200 * time.Millisecond) // Longer than our test timeout
		w.Write([]byte("more data"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	config := &Configuration{
		ResponseTimeout:  5 * time.Second,
		ReadTimeout:      100 * time.Millisecond, // Short timeout
		StreamBufferSize: 1024,
	}
	proxy, selector, _ := createTestSherpaProxyWithConfig([]*domain.Endpoint{endpoint}, config)
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithStats("GET", "/api/stream", "")
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)

	// Should get read timeout error
	if err == nil {
		t.Error("Expected timeout error")
	}
	if !strings.Contains(err.Error(), "stopped responding") {
		t.Errorf("Expected timeout error message, got: %v", err)
	}

	// Should have received initial data
	if !strings.Contains(w.Body.String(), "initial data") {
		t.Error("Should have received initial data before timeout")
	}
}

// TestSherpaProxyService_BufferPooling tests buffer pool behaviour
func TestSherpaProxyService_BufferPooling(t *testing.T) {
	config := &Configuration{StreamBufferSize: 4096}
	proxy, _, _ := createTestSherpaProxyWithConfig([]*domain.Endpoint{}, config)

	// Get buffers from pool
	buf1 := proxy.bufferPool.Get().([]byte)
	buf2 := proxy.bufferPool.Get().([]byte)

	// The Sherpa implementation uses DefaultStreamBufferSize for the buffer pool
	// regardless of the configured StreamBufferSize
	expectedSize := DefaultStreamBufferSize

	if len(buf1) != expectedSize {
		t.Errorf("Expected buffer size %d, got %d", expectedSize, len(buf1))
	}
	if len(buf2) != expectedSize {
		t.Errorf("Expected buffer size %d, got %d", expectedSize, len(buf2))
	}

	// Return to pool
	proxy.bufferPool.Put(buf1)
	proxy.bufferPool.Put(buf2)

	// Get again - should reuse
	buf3 := proxy.bufferPool.Get().([]byte)
	if len(buf3) != expectedSize {
		t.Errorf("Expected reused buffer size %d, got %d", expectedSize, len(buf3))
	}
}

// TestSherpaProxyService_ConfigDefaults tests default configuration values
func TestSherpaProxyService_ConfigDefaults(t *testing.T) {
	config := &Configuration{} // Empty config
	proxy, _, _ := createTestSherpaProxyWithConfig([]*domain.Endpoint{}, config)

	// Check transport has sensible defaults
	if proxy.transport.MaxIdleConns != DefaultMaxIdleConns {
		t.Errorf("Expected MaxIdleConns %d, got %d", DefaultMaxIdleConns, proxy.transport.MaxIdleConns)
	}
	if proxy.transport.IdleConnTimeout != DefaultIdleConnTimeout {
		t.Errorf("Expected IdleConnTimeout %v, got %v", DefaultIdleConnTimeout, proxy.transport.IdleConnTimeout)
	}
	if proxy.transport.TLSHandshakeTimeout != DefaultTLSHandshakeTimeout {
		t.Errorf("Expected TLSHandshakeTimeout %v, got %v", DefaultTLSHandshakeTimeout, proxy.transport.TLSHandshakeTimeout)
	}
}

// TestSherpaProxyService_UpdateConfig tests configuration updates
func TestSherpaProxyService_UpdateConfig(t *testing.T) {
	initialConfig := &Configuration{
		ResponseTimeout:  10 * time.Second,
		ReadTimeout:      5 * time.Second,
		StreamBufferSize: 4096,
	}
	proxy, _, _ := createTestSherpaProxyWithConfig([]*domain.Endpoint{}, initialConfig)

	// Update config
	newConfig := &Configuration{
		ResponseTimeout:  20 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
	}

	proxy.UpdateConfig(newConfig)

	// Check config was updated
	if proxy.configuration.ResponseTimeout != 20*time.Second {
		t.Errorf("Expected ResponseTimeout 20s, got %v", proxy.configuration.ResponseTimeout)
	}
	if proxy.configuration.ReadTimeout != 10*time.Second {
		t.Errorf("Expected ReadTimeout 10s, got %v", proxy.configuration.ReadTimeout)
	}
	if proxy.configuration.StreamBufferSize != 8192 {
		t.Errorf("Expected StreamBufferSize 8192, got %d", proxy.configuration.StreamBufferSize)
	}
}

func TestSherpaProxyService_StatsAccuracy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // Add some latency
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestSherpaProxy([]*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	// Make some successful requests
	for i := 0; i < 3; i++ {
		req, stats, rlog := createTestRequestWithStats("GET", "/api/test", "")
		w := httptest.NewRecorder()
		err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Make a failed request (unreachable endpoint)
	failEndpoint := createTestEndpoint("fail", "http://localhost:99999", domain.StatusHealthy)
	selector.endpoint = failEndpoint

	req, stats, rlog := createTestRequestWithStats("GET", "/api/test", "")
	w := httptest.NewRecorder()
	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{failEndpoint}, stats, rlog)
	if err == nil {
		t.Error("Expected failure for unreachable endpoint")
	}

	// Check stats
	proxyStats, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if proxyStats.TotalRequests != 4 {
		t.Errorf("Expected 4 total requests, got %d", proxyStats.TotalRequests)
	}
	if proxyStats.SuccessfulRequests != 3 {
		t.Errorf("Expected 3 successful requests, got %d", proxyStats.SuccessfulRequests)
	}
	if proxyStats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", proxyStats.FailedRequests)
	}
	if proxyStats.AverageLatency == 0 {
		t.Error("Expected non-zero average latency")
	}
}

// TestSherpaProxyService_ProxyRequestToEndpoints_Success tests the new filtered endpoints method
func TestSherpaProxyService_ProxyRequestToEndpoints_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": "test"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestSherpaProxy([]*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)

	assertNoError(t, err)
	assertSuccessfulResponse(t, w, http.StatusOK, "test")
	assertStatsPopulated(t, stats, "test")
}

// TestSherpaProxyService_ProxyRequestToEndpoints_EmptyEndpoints tests empty endpoints handling
func TestSherpaProxyService_ProxyRequestToEndpoints_EmptyEndpoints(t *testing.T) {
	proxy, _, _ := createTestSherpaProxy([]*domain.Endpoint{})

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{}, stats, rlog)

	assertError(t, err, "no healthy AI backends available")
	if stats.EndpointName != "" {
		t.Error("ProxyRequestToEndpoints() with empty endpoints should not set endpoint name")
	}
}

// TestSherpaProxyService_ProxyRequestToEndpoints_FilteredEndpoints tests platform-specific filtering
func TestSherpaProxyService_ProxyRequestToEndpoints_FilteredEndpoints(t *testing.T) {
	ollamaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "ollama")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"model": "ollama-response"}`))
	}))
	defer ollamaBackend.Close()

	ollamaEndpoint := createTestEndpoint("ollama-1", ollamaBackend.URL, domain.StatusHealthy)
	ollamaEndpoint.Type = domain.ProfileOllama

	proxy, selector, _ := createTestSherpaProxy([]*domain.Endpoint{ollamaEndpoint})
	selector.endpoint = ollamaEndpoint

	req, stats, rlog := createTestRequestWithStats("POST", "/api/generate", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{ollamaEndpoint}, stats, rlog)

	assertNoError(t, err)
	assertStatsPopulated(t, stats, "ollama-1")
	if w.Header().Get("X-Backend") != "ollama" {
		t.Errorf("ProxyRequestToEndpoints() should reach Ollama backend, got %v", w.Header().Get("X-Backend"))
	}
}

// TestSherpaProxyService_ProxyRequestToEndpoints_LoadBalancing tests load balancing with filtered endpoints
func TestSherpaProxyService_ProxyRequestToEndpoints_LoadBalancing(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-ID", "backend-1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": "backend1"}`))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-ID", "backend-2")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": "backend2"}`))
	}))
	defer backend2.Close()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", backend1.URL, domain.StatusHealthy),
		createTestEndpoint("endpoint-2", backend2.URL, domain.StatusHealthy),
	}

	proxy, selector, _ := createTestSherpaProxy(endpoints)
	backendHits := make(map[string]int)

	for i := 0; i < 4; i++ {
		// selector will return endpoints in rotation for predictable testing
		selector.endpoint = endpoints[i%len(endpoints)]

		req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
		w := httptest.NewRecorder()

		err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, endpoints, stats, rlog)
		assertNoError(t, err)

		backendID := w.Header().Get("X-Backend-ID")
		if backendID != "" {
			backendHits[backendID]++
		}

		if stats.EndpointName == "" {
			t.Errorf("ProxyRequestToEndpoints() request %d should set endpoint name", i)
		}
	}

	if len(backendHits) == 0 {
		t.Error("ProxyRequestToEndpoints() should route to backends")
	}

	totalHits := 0
	for _, hits := range backendHits {
		totalHits += hits
	}

	if totalHits != 4 {
		t.Errorf("ProxyRequestToEndpoints() should have made 4 requests, got %d", totalHits)
	}
}

// TestSherpaProxyService_ProxyRequest_BackwardCompatibility tests that original method still works
func TestSherpaProxyService_ProxyRequest_BackwardCompatibility(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"legacy": "response"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestSherpaProxy([]*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)

	assertNoError(t, err)
	assertSuccessfulResponse(t, w, http.StatusOK, "legacy")
	assertStatsPopulated(t, stats, "test")
}

// TestSherpaProxyService_ProxyRequestToEndpoints_StatsCollection tests detailed stats recording
func TestSherpaProxyService_ProxyRequestToEndpoints_StatsCollection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": "response with delay"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestSherpaProxy([]*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)

	assertNoError(t, err)

	if stats.Latency <= 0 {
		t.Error("ProxyRequestToEndpoints() should record total latency")
	}

	if stats.RequestProcessingMs < 0 {
		t.Error("ProxyRequestToEndpoints() should record non-negative request processing time")
	}

	if stats.BackendResponseMs <= 0 {
		t.Error("ProxyRequestToEndpoints() should record backend response time")
	}

	if stats.StreamingMs < 0 {
		t.Error("ProxyRequestToEndpoints() should record streaming time")
	}

	if stats.TotalBytes <= 0 {
		t.Error("ProxyRequestToEndpoints() should record bytes transferred")
	}

	if stats.SelectionMs < 0 {
		t.Error("ProxyRequestToEndpoints() should record non-negative selection time")
	}
}

// TestSherpaProxyService_ProxyRequestToEndpoints_ContextCancellation tests timeout handling
func TestSherpaProxyService_ProxyRequestToEndpoints_ContextCancellation(t *testing.T) {
	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"slow": "response"}`))
	}))
	defer slowBackend.Close()

	endpoint := createTestEndpoint("test", slowBackend.URL, domain.StatusHealthy)
	config := &Configuration{
		ResponseTimeout:  10 * time.Millisecond, // Short timeout
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 1024,
	}
	proxy, selector, _ := createTestSherpaProxyWithConfig([]*domain.Endpoint{endpoint}, config)
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithStats("POST", "/v1/chat/completions", `{"model": "test"}`)
	w := httptest.NewRecorder()

	err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, []*domain.Endpoint{endpoint}, stats, rlog)

	if err == nil {
		t.Error("ProxyRequestToEndpoints() should return error on timeout")
	}

	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "context") {
		t.Errorf("ProxyRequestToEndpoints() should return timeout error, got: %v", err)
	}

	if stats.EndpointName != "test" {
		t.Error("ProxyRequestToEndpoints() should set endpoint name even on timeout")
	}
}

func TestCopyHeaders_SecurityFiltering(t *testing.T) {
	service := &SherpaProxyService{}

	testCases := []struct {
		name            string
		inputHeaders    map[string][]string
		expectedBlocked []string
		expectedCopied  []string
	}{
		{
			name: "blocks_all_credential_headers",
			inputHeaders: map[string][]string{
				"Authorization":       {"Bearer token123"},
				"Cookie":              {"session=abc123"},
				"X-Api-Key":           {"secret-key"},
				"X-Auth-Token":        {"auth-token"},
				"Proxy-Authorization": {"Basic dXNlcjpwYXNz"},
				"Content-Type":        {"application/json"},
				"Accept":              {"application/json"},
			},
			expectedBlocked: []string{"Authorization", "Cookie", "X-Api-Key", "X-Auth-Token", "Proxy-Authorization"},
			expectedCopied:  []string{"Content-Type", "Accept"},
		},
		{
			name: "allows_safe_headers",
			inputHeaders: map[string][]string{
				"Content-Type":     {"application/json"},
				"Accept":           {"application/json"},
				"Accept-Encoding":  {"gzip, deflate"},
				"Accept-Language":  {"en-US,en;q=0.9"},
				"User-Agent":       {"Olla/v0.0.6"},
				"X-Request-ID":     {"req-123"},
				"X-Correlation-ID": {"corr-456"},
			},
			expectedBlocked: []string{},
			expectedCopied:  []string{"Content-Type", "Accept", "Accept-Encoding", "Accept-Language", "User-Agent", "X-Request-ID", "X-Correlation-ID"},
		},
		{
			name: "handles_case_sensitivity",
			inputHeaders: map[string][]string{
				"authorization": {"Bearer token123"},  // lowercase - should still be blocked
				"COOKIE":        {"session=abc123"},   // uppercase - should still be blocked
				"Content-Type":  {"application/json"}, // mixed case - should be copied
			},
			expectedBlocked: []string{}, // Note: Go canonicalizes headers, so these become Authorization, Cookie
			expectedCopied:  []string{"Content-Type"},
		},
		{
			name: "handles_multi_value_headers",
			inputHeaders: map[string][]string{
				"Accept":          {"application/json", "text/plain"},
				"Accept-Encoding": {"gzip", "deflate", "br"},
				"Cookie":          {"session=abc", "user=xyz"}, // Should be blocked
			},
			expectedBlocked: []string{"Cookie"},
			expectedCopied:  []string{"Accept", "Accept-Encoding"},
		},
		{
			name: "handles_similar_header_names",
			inputHeaders: map[string][]string{
				"X-Api-Version":   {"v1"},           // Should be copied (not X-Api-Key)
				"X-Auth-Service":  {"auth-svc"},     // Should be copied (not X-Auth-Token)
				"X-Authorization": {"custom-auth"},  // Should be copied (not Authorization)
				"Custom-Cookie":   {"value"},        // Should be copied (not Cookie)
				"X-Api-Key":       {"secret"},       // Should be blocked
				"Authorization":   {"Bearer token"}, // Should be blocked
			},
			expectedBlocked: []string{"X-Api-Key", "Authorization"},
			expectedCopied:  []string{"X-Api-Version", "X-Auth-Service", "X-Authorization", "Custom-Cookie"},
		},
		{
			name:            "handles_empty_headers",
			inputHeaders:    map[string][]string{},
			expectedBlocked: []string{},
			expectedCopied:  []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create original request with test headers
			originalReq := httptest.NewRequest("POST", "/test", nil)
			for name, values := range tc.inputHeaders {
				for _, value := range values {
					originalReq.Header.Add(name, value)
				}
			}

			// Create proxy request
			proxyReq := httptest.NewRequest("POST", "http://backend/api", nil)

			// Copy headers
			service.copyHeaders(proxyReq, originalReq)

			// Verify blocked headers are not present
			for _, blockedHeader := range tc.expectedBlocked {
				if proxyReq.Header.Get(blockedHeader) != "" {
					t.Errorf("Expected header %s to be blocked, but it was copied", blockedHeader)
				}
			}

			// Verify safe headers are copied
			for _, copiedHeader := range tc.expectedCopied {
				originalValue := originalReq.Header.Get(copiedHeader)
				proxyValue := proxyReq.Header.Get(copiedHeader)
				if proxyValue == "" {
					t.Errorf("Expected header %s to be copied, but it was missing", copiedHeader)
				}
				if originalValue != proxyValue {
					t.Errorf("Header %s: expected %s, got %s", copiedHeader, originalValue, proxyValue)
				}
			}

			// Verify multi-value headers are copied correctly
			for _, copiedHeader := range tc.expectedCopied {
				originalValues := originalReq.Header[copiedHeader]
				proxyValues := proxyReq.Header[copiedHeader]
				if len(originalValues) > 1 {
					if len(proxyValues) != len(originalValues) {
						t.Errorf("Multi-value header %s: expected %d values, got %d", copiedHeader, len(originalValues), len(proxyValues))
					}
					for i, originalVal := range originalValues {
						if i < len(proxyValues) && proxyValues[i] != originalVal {
							t.Errorf("Multi-value header %s[%d]: expected %s, got %s", copiedHeader, i, originalVal, proxyValues[i])
						}
					}
				}
			}
		})
	}
}

func TestCopyHeaders_ProxyHeaders(t *testing.T) {
	service := &SherpaProxyService{}

	testCases := []struct {
		name            string
		setupRequest    func() *http.Request
		expectedHeaders map[string]string
	}{
		{
			name: "adds_proxy_headers_http",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.100:12345"
				return req
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-Host":  "example.com",
				"X-Forwarded-Proto": "http",
				"X-Forwarded-For":   "192.168.1.100",
				"X-Proxied-By":      "Olla/v0.0.0",
				"Via":               "1.1 olla/v0.0.0",
			},
		},
		{
			name: "adds_proxy_headers_https",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/test", nil)
				req.Host = "secure.example.com"
				req.RemoteAddr = "10.0.0.1:54321"
				req.TLS = &tls.ConnectionState{} // Simulate HTTPS
				return req
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-Host":  "secure.example.com",
				"X-Forwarded-Proto": "https",
				"X-Forwarded-For":   "10.0.0.1",
				"X-Proxied-By":      "Olla/v0.0.0",
				"Via":               "1.1 olla/v0.0.0",
			},
		},
		{
			name: "handles_malformed_remote_addr",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Host = "example.com"
				req.RemoteAddr = "malformed-address" // No port
				return req
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-Host":  "example.com",
				"X-Forwarded-Proto": "http",
				// X-Forwarded-For should not be set due to malformed address
				"X-Proxied-By": "Olla/v0.0.0",
				"Via":          "1.1 olla/v0.0.0",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalReq := tc.setupRequest()
			proxyReq := httptest.NewRequest("POST", "http://backend/api", nil)

			service.copyHeaders(proxyReq, originalReq)

			for headerName, expectedValue := range tc.expectedHeaders {
				actualValue := proxyReq.Header.Get(headerName)
				if actualValue != expectedValue {
					t.Errorf("Header %s: expected %s, got %s", headerName, expectedValue, actualValue)
				}
			}

			// Verify X-Forwarded-For is not set when RemoteAddr is malformed
			if tc.name == "handles_malformed_remote_addr" {
				if proxyReq.Header.Get("X-Forwarded-For") != "" {
					t.Error("X-Forwarded-For should not be set for malformed RemoteAddr")
				}
			}
		})
	}
}
