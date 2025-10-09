package core

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestCopyHeaders(t *testing.T) {
	tests := []struct {
		name            string
		originalHeaders map[string][]string
		expectedCopied  map[string][]string
		expectedSkipped []string
	}{
		{
			name: "basic_headers_copied",
			originalHeaders: map[string][]string{
				"Content-Type":    {"application/json"},
				"Accept":          {"text/html"},
				"User-Agent":      {"test-agent"},
				"X-Custom-Header": {"custom-value"},
			},
			expectedCopied: map[string][]string{
				"Content-Type":    {"application/json"},
				"Accept":          {"text/html"},
				"User-Agent":      {"test-agent"},
				"X-Custom-Header": {"custom-value"},
			},
			expectedSkipped: []string{},
		},
		{
			name: "security_headers_filtered",
			originalHeaders: map[string][]string{
				"Content-Type":        {"application/json"},
				"Authorization":       {"Bearer secret-token"},
				"Cookie":              {"session=secret"},
				"X-Api-Key":           {"secret-api-key"},
				"X-Auth-Token":        {"secret-auth-token"},
				"Proxy-Authorization": {"Basic secret"},
			},
			expectedCopied: map[string][]string{
				"Content-Type": {"application/json"},
			},
			expectedSkipped: []string{
				"Authorization",
				"Cookie",
				"X-Api-Key",
				"X-Auth-Token",
				"Proxy-Authorization",
			},
		},
		{
			name: "hop_by_hop_headers_filtered",
			originalHeaders: map[string][]string{
				"Content-Type":        {"application/json"},
				"Connection":          {"keep-alive"},
				"Keep-Alive":          {"timeout=5"},
				"Proxy-Authenticate":  {"Basic"},
				"Proxy-Authorization": {"Basic secret"},
				"TE":                  {"trailers"},
				"Trailer":             {"X-Custom"},
				"Transfer-Encoding":   {"chunked"},
				"Upgrade":             {"websocket"},
			},
			expectedCopied: map[string][]string{
				"Content-Type": {"application/json"},
			},
			expectedSkipped: []string{
				"Connection",
				"Keep-Alive",
				"Proxy-Authenticate",
				"Proxy-Authorization",
				"TE",
				"Trailer",
				"Transfer-Encoding",
				"Upgrade",
			},
		},
		{
			name: "multi_value_headers",
			originalHeaders: map[string][]string{
				"Accept":     {"text/html", "application/json"},
				"X-Custom":   {"value1", "value2", "value3"},
				"Set-Cookie": {"cookie1=value1", "cookie2=value2"},
			},
			expectedCopied: map[string][]string{
				"Accept":     {"text/html", "application/json"},
				"X-Custom":   {"value1", "value2", "value3"},
				"Set-Cookie": {"cookie1=value1", "cookie2=value2"},
			},
			expectedSkipped: []string{},
		},
		{
			name:            "empty_headers",
			originalHeaders: map[string][]string{},
			expectedCopied:  map[string][]string{},
			expectedSkipped: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create original request with headers
			originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
			for k, values := range tt.originalHeaders {
				for _, v := range values {
					originalReq.Header.Add(k, v)
				}
			}

			// Create proxy request
			proxyReq := httptest.NewRequest("GET", "http://backend.com/test", nil)

			// Copy headers
			CopyHeaders(proxyReq, originalReq)

			// Check that expected headers were copied
			for k, expectedValues := range tt.expectedCopied {
				actualValues := proxyReq.Header[k]
				assert.Equal(t, expectedValues, actualValues, "Header %s should be copied correctly", k)
			}

			// Check that security headers were NOT copied
			for _, skippedHeader := range tt.expectedSkipped {
				assert.Empty(t, proxyReq.Header.Get(skippedHeader), "Header %s should not be copied", skippedHeader)
			}

			// Check proxy-specific headers are added
			assert.NotEmpty(t, proxyReq.Header.Get("X-Proxied-By"))
			assert.NotEmpty(t, proxyReq.Header.Get("Via"))
		})
	}
}

func TestCopyHeaders_ProxyHeaders(t *testing.T) {
	tests := []struct {
		name                   string
		originalHeaders        map[string][]string
		remoteAddr             string
		tls                    bool
		expectedForwardedFor   string
		expectedForwardedProto string
		expectedForwardedHost  string
		expectedRealIP         string
		expectedVia            string
	}{
		{
			name:                   "http_request_basic",
			originalHeaders:        map[string][]string{},
			remoteAddr:             "192.168.1.100:12345",
			tls:                    false,
			expectedForwardedFor:   "192.168.1.100",
			expectedForwardedProto: constants.ProtocolHTTP,
			expectedForwardedHost:  "example.com",
			expectedRealIP:         "192.168.1.100",
		},
		{
			name:                   "https_request_basic",
			originalHeaders:        map[string][]string{},
			remoteAddr:             "192.168.1.100:12345",
			tls:                    true,
			expectedForwardedFor:   "192.168.1.100",
			expectedForwardedProto: constants.ProtocolHTTPS,
			expectedForwardedHost:  "example.com",
			expectedRealIP:         "192.168.1.100",
		},
		{
			name: "existing_forwarded_headers",
			originalHeaders: map[string][]string{
				"X-Forwarded-For":   {"10.0.0.1"},
				"X-Forwarded-Proto": {"https"},
				"X-Forwarded-Host":  {"original.com"},
				"X-Real-IP":         {"10.0.0.1"},
			},
			remoteAddr:             "192.168.1.100:12345",
			tls:                    false,
			expectedForwardedFor:   "10.0.0.1, 10.0.0.1", // extractClientIP will get from X-Forwarded-For
			expectedForwardedProto: "https",              // Preserves existing
			expectedForwardedHost:  "original.com",       // Preserves existing
			expectedRealIP:         "10.0.0.1",           // Preserves existing
		},
		{
			name: "via_header_chaining",
			originalHeaders: map[string][]string{
				"Via": {"1.1 proxy1"},
			},
			remoteAddr:  "192.168.1.100:12345",
			tls:         false,
			expectedVia: "1.1 proxy1, " + GetViaHeader(),
		},
		{
			name:                   "malformed_remote_addr",
			originalHeaders:        map[string][]string{},
			remoteAddr:             "invalid-addr",
			tls:                    false,
			expectedForwardedFor:   "", // Should not be set due to error
			expectedForwardedProto: constants.ProtocolHTTP,
			expectedForwardedHost:  "example.com",
			expectedRealIP:         "", // Should not be set due to error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create original request
			originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
			originalReq.Host = "example.com"
			originalReq.RemoteAddr = tt.remoteAddr

			if tt.tls {
				originalReq.TLS = &tls.ConnectionState{}
			}

			// Add headers
			for k, values := range tt.originalHeaders {
				for _, v := range values {
					originalReq.Header.Add(k, v)
				}
			}

			// Create proxy request
			proxyReq := httptest.NewRequest("GET", "http://backend.com/test", nil)

			// Copy headers
			CopyHeaders(proxyReq, originalReq)

			// Check forwarded headers
			if tt.expectedForwardedFor != "" {
				assert.Equal(t, tt.expectedForwardedFor, proxyReq.Header.Get("X-Forwarded-For"))
			}

			if tt.expectedForwardedProto != "" {
				assert.Equal(t, tt.expectedForwardedProto, proxyReq.Header.Get("X-Forwarded-Proto"))
			}

			if tt.expectedForwardedHost != "" {
				assert.Equal(t, tt.expectedForwardedHost, proxyReq.Header.Get("X-Forwarded-Host"))
			}

			if tt.expectedRealIP != "" {
				assert.Equal(t, tt.expectedRealIP, proxyReq.Header.Get("X-Real-IP"))
			}

			if tt.expectedVia != "" {
				assert.Equal(t, tt.expectedVia, proxyReq.Header.Get("Via"))
			}
		})
	}
}

func TestSetResponseHeaders(t *testing.T) {
	tests := []struct {
		name            string
		stats           *ports.RequestStats
		endpoint        *domain.Endpoint
		expectedHeaders map[string]string
		checkTrailer    bool
	}{
		{
			name: "all_fields_set",
			stats: &ports.RequestStats{
				RequestID: "test-request-123",
				Model:     "gpt-4",
				StartTime: time.Now().Add(-100 * time.Millisecond),
			},
			endpoint: &domain.Endpoint{
				Name: "backend-1",
				Type: "openai",
			},
			expectedHeaders: map[string]string{
				"X-Olla-Request-ID":   "test-request-123",
				"X-Olla-Endpoint":     "backend-1",
				"X-Olla-Backend-Type": "openai",
				"X-Olla-Model":        "gpt-4",
				// X-Olla-Response-Time will be checked separately as it's dynamic
			},
			checkTrailer: true,
		},
		{
			name: "minimal_fields",
			stats: &ports.RequestStats{
				RequestID: "test-request-456",
			},
			endpoint: &domain.Endpoint{
				Name: "backend-2",
				Type: "ollama",
			},
			expectedHeaders: map[string]string{
				"X-Olla-Request-ID":   "test-request-456",
				"X-Olla-Endpoint":     "backend-2",
				"X-Olla-Backend-Type": "ollama",
			},
			checkTrailer: true,
		},
		{
			name:  "nil_stats",
			stats: nil,
			endpoint: &domain.Endpoint{
				Name: "backend-3",
				Type: "lmstudio",
			},
			expectedHeaders: map[string]string{
				"X-Olla-Endpoint":     "backend-3",
				"X-Olla-Backend-Type": "lmstudio",
			},
			checkTrailer: false,
		},
		{
			name: "nil_endpoint",
			stats: &ports.RequestStats{
				RequestID: "test-request-789",
			},
			endpoint: nil,
			expectedHeaders: map[string]string{
				"X-Olla-Request-ID": "test-request-789",
			},
			checkTrailer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			SetResponseHeaders(w, tt.stats, tt.endpoint)

			// Check standard headers
			assert.NotEmpty(t, w.Header().Get("X-Served-By"))
			assert.NotEmpty(t, w.Header().Get("Via"))

			// Check expected headers
			for k, v := range tt.expectedHeaders {
				assert.Equal(t, v, w.Header().Get(k), "Header %s should be set correctly", k)
			}

			// Check X-Olla-Response-Time header when StartTime is set
			if tt.stats != nil && !tt.stats.StartTime.IsZero() {
				responseTimeHeader := w.Header().Get("X-Olla-Response-Time")
				assert.NotEmpty(t, responseTimeHeader, "X-Olla-Response-Time should be set when StartTime is present")
				// Verify itss in milliseconds format (e.g., "123ms")
				assert.Contains(t, responseTimeHeader, "ms", "X-Olla-Response-Time should be in milliseconds format")
				// Verify it'ss still parseable as a duration
				_, err := time.ParseDuration(responseTimeHeader)
				assert.NoError(t, err, "X-Olla-Response-Time should be a valid duration")
			} else {
				// When StartTime is not set, the header should not be present
				assert.Empty(t, w.Header().Get("X-Olla-Response-Time"), "X-Olla-Response-Time should not be set when StartTime is zero")
			}

			// Note: Trailer header is set by the proxy implementation, not SetResponseHeaders
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "basic_remote_addr",
			remoteAddr: "192.168.1.100:12345",
			headers:    map[string]string{},
			expectedIP: "192.168.1.100",
		},
		{
			name:       "x_forwarded_for_single",
			remoteAddr: "192.168.1.100:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
			},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "x_forwarded_for_multiple",
			remoteAddr: "192.168.1.100:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1, 10.0.0.2, 10.0.0.3",
			},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "x_real_ip",
			remoteAddr: "192.168.1.100:12345",
			headers: map[string]string{
				"X-Real-IP": "10.0.0.5",
			},
			expectedIP: "10.0.0.5",
		},
		{
			name:       "prefer_x_forwarded_for",
			remoteAddr: "192.168.1.100:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
				"X-Real-IP":       "10.0.0.5",
			},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "malformed_remote_addr",
			remoteAddr: "invalid-addr",
			headers:    map[string]string{},
			expectedIP: "invalid-addr", // Falls back to RemoteAddr when split fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = tt.remoteAddr

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := extractClientIP(req)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestIsHopByHopHeader(t *testing.T) {
	tests := []struct {
		header   string
		expected bool
	}{
		{"Connection", true},
		{"connection", true}, // Case insensitive
		{"Keep-Alive", true},
		{"Proxy-Authenticate", true},
		{"Proxy-Authorization", true},
		{"TE", true},
		{"Trailer", true},
		{"Transfer-Encoding", true},
		{"Upgrade", true},
		{"Content-Type", false},
		{"Authorization", false},
		{"X-Custom-Header", false},
		{"Accept", false},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			result := isHopByHopHeader(tt.header)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProxyHeaderConstants(t *testing.T) {
	// Test that proxy header functions return non-empty values
	assert.NotEmpty(t, GetProxiedByHeader())
	assert.Contains(t, GetProxiedByHeader(), "/")

	assert.NotEmpty(t, GetViaHeader())
	assert.Contains(t, GetViaHeader(), "1.1")
}

func TestHeaderConstants(t *testing.T) {
	// Test that header constants are defined
	assert.Equal(t, "X-Olla-Request-ID", constants.HeaderXOllaRequestID)
	assert.Equal(t, "X-Olla-Endpoint", constants.HeaderXOllaEndpoint)
	assert.Equal(t, "X-Olla-Backend-Type", constants.HeaderXOllaBackendType)
	assert.Equal(t, "X-Olla-Model", constants.HeaderXOllaModel)
	assert.Equal(t, "X-Olla-Response-Time", constants.HeaderXOllaResponseTime)
}

// TestCopyHeaders_ExistingHeaders tests the edge case where proxyReq.Header has pre-existing entries
// This verifies that the current fix handles the scenario correctly even when the map is not nil
func TestCopyHeaders_ExistingHeaders(t *testing.T) {
	tests := []struct {
		name             string
		existingHeaders  map[string][]string
		originalHeaders  map[string][]string
		expectedHeaders  map[string][]string
		shouldClearExist bool
		description      string
	}{
		{
			name: "pre_existing_headers_overwritten",
			existingHeaders: map[string][]string{
				"X-Existing": {"old-value"},
			},
			originalHeaders: map[string][]string{
				"X-Existing": {"new-value"},
				"X-New-1":    {"value1"},
				"X-New-2":    {"value2"},
			},
			expectedHeaders: map[string][]string{
				"X-Existing": {"new-value"},
				"X-New-1":    {"value1"},
				"X-New-2":    {"value2"},
			},
			shouldClearExist: false,
			description:      "Headers with same name should be overwritten from source",
		},
		{
			name: "pre_existing_different_headers",
			existingHeaders: map[string][]string{
				"X-Pre-Existing-1": {"pre1"},
				"X-Pre-Existing-2": {"pre2"},
			},
			originalHeaders: map[string][]string{
				"X-New-1": {"new1"},
				"X-New-2": {"new2"},
			},
			expectedHeaders: map[string][]string{
				// Pre-existing headers remain because CopyHeaders doesn't clear the map
				"X-Pre-Existing-1": {"pre1"},
				"X-Pre-Existing-2": {"pre2"},
				"X-New-1":          {"new1"},
				"X-New-2":          {"new2"},
			},
			shouldClearExist: false,
			description:      "Pre-existing headers with different names persist alongside new headers",
		},
		{
			name: "many_pre_existing_few_new",
			existingHeaders: map[string][]string{
				"X-Exist-1":  {"e1"},
				"X-Exist-2":  {"e2"},
				"X-Exist-3":  {"e3"},
				"X-Exist-4":  {"e4"},
				"X-Exist-5":  {"e5"},
				"X-Exist-6":  {"e6"},
				"X-Exist-7":  {"e7"},
				"X-Exist-8":  {"e8"},
				"X-Exist-9":  {"e9"},
				"X-Exist-10": {"e10"},
			},
			originalHeaders: map[string][]string{
				"Content-Type": {"application/json"},
				"Accept":       {"text/html"},
			},
			expectedHeaders: map[string][]string{
				"X-Exist-1":    {"e1"},
				"X-Exist-2":    {"e2"},
				"X-Exist-3":    {"e3"},
				"X-Exist-4":    {"e4"},
				"X-Exist-5":    {"e5"},
				"X-Exist-6":    {"e6"},
				"X-Exist-7":    {"e7"},
				"X-Exist-8":    {"e8"},
				"X-Exist-9":    {"e9"},
				"X-Exist-10":   {"e10"},
				"Content-Type": {"application/json"},
				"Accept":       {"text/html"},
			},
			shouldClearExist: false,
			description:      "Edge case: many existing entries, few new ones - tests potential rehashing",
		},
		{
			name:            "empty_existing_map_normal_operation",
			existingHeaders: map[string][]string{
				// Empty but non-nil map (len=0)
			},
			originalHeaders: map[string][]string{
				"Content-Type": {"application/json"},
				"X-New-1":      {"value1"},
				"X-New-2":      {"value2"},
			},
			expectedHeaders: map[string][]string{
				"Content-Type": {"application/json"},
				"X-New-1":      {"value1"},
				"X-New-2":      {"value2"},
			},
			shouldClearExist: false,
			description:      "Normal case: empty but non-nil map (mimics http.NewRequestWithContext)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create original request with headers
			originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
			for k, values := range tt.originalHeaders {
				for _, v := range values {
					originalReq.Header.Add(k, v)
				}
			}

			// Create proxy request with pre-existing headers
			proxyReq := httptest.NewRequest("GET", "http://backend.com/test", nil)
			for k, values := range tt.existingHeaders {
				for _, v := range values {
					proxyReq.Header.Add(k, v)
				}
			}

			// Record initial state for debugging
			initialLen := len(proxyReq.Header)
			t.Logf("Test: %s - Initial header count: %d", tt.description, initialLen)

			// Copy headers
			CopyHeaders(proxyReq, originalReq)

			// Verify expected headers (excluding proxy-specific headers)
			for k, expectedValues := range tt.expectedHeaders {
				actualValues := proxyReq.Header[k]
				assert.Equal(t, expectedValues, actualValues,
					"Header %s should have expected value for: %s", k, tt.description)
			}

			// Verify proxy-specific headers are always added
			assert.NotEmpty(t, proxyReq.Header.Get("X-Proxied-By"))
			assert.NotEmpty(t, proxyReq.Header.Get("Via"))

			t.Logf("Final header count: %d", len(proxyReq.Header))
		})
	}
}

// TestCopyHeaders_MapPreSizingOptimization verifies the optimization behaviour
// This test documents the behaviour but doesn't fail if optimization isn't perfect
func TestCopyHeaders_MapPreSizingOptimization(t *testing.T) {
	t.Run("verify_http_request_creates_empty_map", func(t *testing.T) {
		// This test verifies that http.NewRequestWithContext creates an empty (len=0) map
		// which means our nil check will pass and we'll create a pre-sized map
		req := httptest.NewRequest("GET", "http://example.com/test", nil)

		assert.NotNil(t, req.Header, "http.Request.Header should not be nil")
		assert.Equal(t, 0, len(req.Header), "http.Request.Header should be empty (len=0)")

		// Since len=0 but Header != nil, our current optimization won't apply
		// But this is fine because an empty map doesn't cause rehashing issues
	})

	t.Run("verify_current_fix_handles_nil", func(t *testing.T) {
		// Test that nil header maps are handled correctly
		var req *http.Request
		req = &http.Request{
			Method: "GET",
			Header: nil, // Explicitly nil
		}

		originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
		originalReq.Header.Set("X-Test", "value")

		// This should work without panic
		CopyHeaders(req, originalReq)

		assert.Equal(t, "value", req.Header.Get("X-Test"))
	})
}

func BenchmarkCopyHeaders(b *testing.B) {
	// Create original request with typical headers
	originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
	originalReq.Header.Set("Content-Type", "application/json")
	originalReq.Header.Set("Accept", "application/json")
	originalReq.Header.Set("User-Agent", "test-agent/1.0")
	originalReq.Header.Set("X-Custom-Header", "custom-value")
	originalReq.RemoteAddr = "192.168.1.100:12345"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		proxyReq := httptest.NewRequest("GET", "http://backend.com/proxy", nil)
		CopyHeaders(proxyReq, originalReq)
	}
}

// BenchmarkCopyHeaders_WithExistingHeaders benchmarks the edge case
func BenchmarkCopyHeaders_WithExistingHeaders(b *testing.B) {
	// Create original request with typical headers
	originalReq := httptest.NewRequest("GET", "http://example.com/test", nil)
	originalReq.Header.Set("Content-Type", "application/json")
	originalReq.Header.Set("Accept", "application/json")
	originalReq.Header.Set("User-Agent", "test-agent/1.0")
	originalReq.Header.Set("X-Custom-Header", "custom-value")
	originalReq.RemoteAddr = "192.168.1.100:12345"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		proxyReq := httptest.NewRequest("GET", "http://backend.com/proxy", nil)
		// Pre-populate with some headers (edge case)
		proxyReq.Header.Set("X-Pre-Existing-1", "value1")
		proxyReq.Header.Set("X-Pre-Existing-2", "value2")
		CopyHeaders(proxyReq, originalReq)
	}
}

// BenchmarkSetResponseHeaders benchmarks the SetResponseHeaders function
func BenchmarkSetResponseHeaders(b *testing.B) {
	stats := &ports.RequestStats{
		RequestID: "test-request-123",
		Model:     "gpt-4",
		StartTime: time.Now().Add(-100 * time.Millisecond),
	}
	endpoint := &domain.Endpoint{
		Name: "backend-1",
		Type: "openai",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		SetResponseHeaders(w, stats, endpoint)
	}
}
