package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

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
	if proxyReq.Header.Get("Authorization") != "Bearer token123" {
		t.Error("Authorization header not copied")
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

	collector := createTestStatsCollector()
	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := &Configuration{
		ResponseTimeout:  5 * time.Second,
		ReadTimeout:      100 * time.Millisecond, // Short timeout
		StreamBufferSize: 1024,
	}

	proxy := NewSherpaService(discovery, selector, config, collector, createTestLogger())

	req := httptest.NewRequest("GET", "/api/stream", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

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
	collector := createTestStatsCollector()
	proxy := NewSherpaService(&mockDiscoveryService{}, newMockEndpointSelector(collector), config, collector, createTestLogger())

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
	collector := createTestStatsCollector()
	config := &Configuration{} // Empty config
	proxy := NewSherpaService(&mockDiscoveryService{}, newMockEndpointSelector(collector), config, collector, createTestLogger())

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
	collector := createTestStatsCollector()
	proxy := NewSherpaService(&mockDiscoveryService{}, newMockEndpointSelector(collector), initialConfig, collector, createTestLogger())

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
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := &Configuration{}

	proxy := NewSherpaService(discovery, selector, config, collector, createTestLogger())

	// Make some successful requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()
		_, err := proxy.ProxyRequest(context.Background(), w, req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Make a failed request (unreachable endpoint)
	failEndpoint := createTestEndpoint("fail", "http://localhost:99999", domain.StatusHealthy)
	discovery.endpoints = []*domain.Endpoint{failEndpoint}
	selector.endpoint = failEndpoint

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	_, err := proxy.ProxyRequest(context.Background(), w, req)
	if err == nil {
		t.Error("Expected failure for unreachable endpoint")
	}

	// Check stats
	stats, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalRequests != 4 {
		t.Errorf("Expected 4 total requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 3 {
		t.Errorf("Expected 3 successful requests, got %d", stats.SuccessfulRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
	if stats.AverageLatency == 0 {
		t.Error("Expected non-zero average latency")
	}
}
