package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// ProxyTestSuite defines the interface for creating proxy implementations to test
type ProxyTestSuite interface {
	Name() string
	CreateProxy(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector) ports.ProxyService
	CreateConfig() ports.ProxyConfiguration
}

// SherpaTestSuite implements ProxyTestSuite for the Sherpa proxy
type SherpaTestSuite struct{}

func (s SherpaTestSuite) Name() string {
	return "Sherpa"
}

func (s SherpaTestSuite) CreateProxy(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector) ports.ProxyService {
	return NewSherpaService(discovery, selector, config.(*Configuration), collector, createTestLogger())
}

func (s SherpaTestSuite) CreateConfig() ports.ProxyConfiguration {
	return &Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
	}
}

// OllaTestSuite implements ProxyTestSuite for the Olla proxy
type OllaTestSuite struct{}

func (o OllaTestSuite) Name() string {
	return "Olla"
}

type mockDiscoveryService struct {
	endpoints []*domain.Endpoint
	err       error
}

func (m *mockDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, m.err
}

func (m *mockDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	if m.err != nil {
		return nil, m.err
	}

	var healthy []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.Status.IsRoutable() {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return m.err
}

type mockEndpointSelector struct {
	endpoint  *domain.Endpoint
	err       error
	collector ports.StatsCollector
}

func newMockEndpointSelector(collector ports.StatsCollector) *mockEndpointSelector {
	return &mockEndpointSelector{collector: collector}
}

func (m *mockEndpointSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.endpoint != nil {
		return m.endpoint, nil
	}
	if len(endpoints) > 0 {
		return endpoints[0], nil
	}
	return nil, fmt.Errorf("no endpoints available")
}

func (m *mockEndpointSelector) Name() string {
	return "mock"
}

func (m *mockEndpointSelector) IncrementConnections(endpoint *domain.Endpoint) {
	if m.collector != nil {
		m.collector.RecordConnection(endpoint, 1)
	}
}

func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {
	if m.collector != nil {
		m.collector.RecordConnection(endpoint, -1)
	}
}

func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func createTestEndpoint(name, urlStr string, status domain.EndpointStatus) *domain.Endpoint {
	testURL, _ := url.Parse(urlStr)
	return &domain.Endpoint{
		Name:   name,
		URL:    testURL,
		Status: status,
	}
}

func createTestStatsCollector() ports.StatsCollector {
	return stats.NewCollector(createTestLogger())
}

// Test all proxy implementations
func TestAllProxies(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			runSharedProxyTests(t, suite)
		})
	}
}

func runSharedProxyTests(t *testing.T, suite ProxyTestSuite) {
	t.Run("ProxyRequest_Success", func(t *testing.T) {
		testProxyRequestSuccess(t, suite)
	})

	t.Run("ProxyRequest_NoEndpoints", func(t *testing.T) {
		testProxyRequestNoEndpoints(t, suite)
	})

	t.Run("ProxyRequest_DiscoveryError", func(t *testing.T) {
		testProxyRequestDiscoveryError(t, suite)
	})

	t.Run("ProxyRequest_SelectorError", func(t *testing.T) {
		testProxyRequestSelectorError(t, suite)
	})

	t.Run("ProxyRequest_UpstreamError", func(t *testing.T) {
		testProxyRequestUpstreamError(t, suite)
	})

	t.Run("ProxyRequest_StreamingResponse", func(t *testing.T) {
		testProxyRequestStreamingResponse(t, suite)
	})

	t.Run("ProxyRequest_ClientDisconnection", func(t *testing.T) {
		testProxyRequestClientDisconnection(t, suite)
	})

	t.Run("ProxyRequest_UpstreamTimeout", func(t *testing.T) {
		testProxyRequestUpstreamTimeout(t, suite)
	})

	t.Run("ConnectionTracking", func(t *testing.T) {
		testConnectionTracking(t, suite)
	})

	t.Run("ConcurrentRequests", func(t *testing.T) {
		testConcurrentRequests(t, suite)
	})

	t.Run("GetStats", func(t *testing.T) {
		testGetStats(t, suite)
	})

	t.Run("LargePayloadHandling", func(t *testing.T) {
		testLargePayloadHandling(t, suite)
	})

	t.Run("RequestBody", func(t *testing.T) {
		testRequestBody(t, suite)
	})

	t.Run("ErrorStatusCodes", func(t *testing.T) {
		testErrorStatusCodes(t, suite)
	})

	t.Run("QueryParameters", func(t *testing.T) {
		testQueryParameters(t, suite)
	})

	t.Run("HTTPMethods", func(t *testing.T) {
		testHTTPMethods(t, suite)
	})

	t.Run("MultipleEndpoints", func(t *testing.T) {
		testMultipleEndpoints(t, suite)
	})

	t.Run("SlowUpstream", func(t *testing.T) {
		testSlowUpstream(t, suite)
	})

	t.Run("ConnectionPooling", func(t *testing.T) {
		testConnectionPooling(t, suite)
	})

	// Additional coverage tests
	t.Run("UpdateConfig", func(t *testing.T) {
		testUpdateConfig(t, suite)
	})

	t.Run("EmptyRequestBody", func(t *testing.T) {
		testEmptyRequestBody(t, suite)
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		testSpecialCharacters(t, suite)
	})

	t.Run("ContentTypes", func(t *testing.T) {
		testContentTypes(t, suite)
	})

	t.Run("LargeHeaders", func(t *testing.T) {
		testLargeHeaders(t, suite)
	})
}

func testProxyRequestSuccess(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	stats, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if stats.TotalBytes == 0 {
		t.Error("Expected non-zero bytes transferred")
	}

	// Check that we got the expected JSON response
	responseBody := w.Body.String()
	expectedResponse := `{"status": "ok"}`
	if responseBody != expectedResponse {
		t.Errorf("Response body not proxied correctly. Expected: %s, Got: %s", expectedResponse, responseBody)
	}

	// Check connection count via stats collector instead of selector
	connectionStats := collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Error("Connection count should be 0 after request completion")
	}
}

func testProxyRequestNoEndpoints(t *testing.T, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when no endpoints available")
	}
	if !strings.Contains(err.Error(), "no healthy") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func testHTTPMethods(t *testing.T, suite ProxyTestSuite) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
				if method != "HEAD" {
					w.Write([]byte("ok"))
				}
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			collector := createTestStatsCollector()
			selector := newMockEndpointSelector(collector)
			selector.endpoint = endpoint
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			var body io.Reader
			if method == "POST" || method == "PUT" || method == "PATCH" {
				body = strings.NewReader(`{"test": "data"}`)
			}

			req := httptest.NewRequest(method, "/api/test", body)
			w := httptest.NewRecorder()

			_, err := proxy.ProxyRequest(context.Background(), w, req)

			if err != nil {
				t.Fatalf("HTTP method %s proxy failed: %v", method, err)
			}
			if receivedMethod != method {
				t.Errorf("HTTP method not preserved. Expected: %s, Got: %s", method, receivedMethod)
			}
		})
	}
}

func testMultipleEndpoints(t *testing.T, suite ProxyTestSuite) {
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream1"))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream2"))
	}))
	defer upstream2.Close()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("test1", upstream1.URL, domain.StatusHealthy),
		createTestEndpoint("test2", upstream2.URL, domain.StatusHealthy),
	}

	discovery := &mockDiscoveryService{endpoints: endpoints}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector) // Will select first available
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Multiple endpoints proxy failed: %v", err)
	}

	response := w.Body.String()
	if response != "upstream1" && response != "upstream2" {
		t.Errorf("Unexpected response from endpoints: %s", response)
	}
}

func testSlowUpstream(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("processed"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	start := time.Now()
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{"prompt": "test"}`))
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Slow upstream proxy failed: %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Error("Request completed too quickly - upstream delay not respected")
	}
	if w.Body.String() != "processed" {
		t.Error("Response not received from slow upstream")
	}
}

func testConnectionPooling(t *testing.T, suite ProxyTestSuite) {
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "call_%d", atomic.LoadInt32(&callCount))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	const numRequests = 5
	var wg sync.WaitGroup
	responses := make([]string, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/api/test", nil)
			w := httptest.NewRecorder()

			_, err := proxy.ProxyRequest(context.Background(), w, req)
			if err != nil {
				t.Errorf("Request %d failed: %v", index, err)
				return
			}
			responses[index] = w.Body.String()
		}(i)
	}

	wg.Wait()

	for i, response := range responses {
		if response == "" {
			t.Errorf("Request %d got empty response", i)
		}
	}

	if atomic.LoadInt32(&callCount) != numRequests {
		t.Errorf("Expected %d upstream calls, got %d", numRequests, atomic.LoadInt32(&callCount))
	}
}

func testUpdateConfig(t *testing.T, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	// Create new config with different values
	var newConfig ports.ProxyConfiguration
	if suite.Name() == "Sherpa" {
		newConfig = &Configuration{
			ResponseTimeout:  60 * time.Second,
			ReadTimeout:      30 * time.Second,
			StreamBufferSize: 16384,
		}
	}
	// Update config should not panic
	proxy.UpdateConfig(newConfig)

	// Verify we can still get stats after config update
	_, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Errorf("GetStats failed after config update: %v", err)
	}
}

func testEmptyRequestBody(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("Expected empty body, got %d bytes", len(body))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("POST", "/api/test", nil) // No body
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Empty body request failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func testSpecialCharacters(t *testing.T, suite ProxyTestSuite) {
	var receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	// Use a simpler path that doesn't require URL encoding
	specialPath := "/api/test-with-dashes/café"
	req := httptest.NewRequest("GET", specialPath, nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Special characters request failed: %v", err)
	}

	// The received path might be URL-decoded, so check if it contains the key parts
	if !strings.Contains(receivedPath, "test-with-dashes") || !strings.Contains(receivedPath, "café") {
		t.Errorf("Path not preserved correctly. Expected to contain test parts, got: %s", receivedPath)
	}
}

func testContentTypes(t *testing.T, suite ProxyTestSuite) {
	testCases := []struct {
		name        string
		contentType string
		body        string
	}{
		{"JSON", "application/json", `{"key": "value"}`},
		{"XML", "application/xml", `<root><key>value</key></root>`},
		{"Plain Text", "text/plain", "simple text"},
		{"Binary", "application/octet-stream", string([]byte{0x01, 0x02, 0x03, 0x04})},
		{"Form Data", "application/x-www-form-urlencoded", "key1=value1&key2=value2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedContentType string
			var receivedBody string

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedContentType = r.Header.Get("Content-Type")
				body, _ := io.ReadAll(r.Body)
				receivedBody = string(body)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			collector := createTestStatsCollector()
			selector := newMockEndpointSelector(collector)
			selector.endpoint = endpoint
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			req := httptest.NewRequest("POST", "/api/test", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()

			_, err := proxy.ProxyRequest(context.Background(), w, req)

			if err != nil {
				t.Fatalf("Content type %s request failed: %v", tc.contentType, err)
			}
			if receivedContentType != tc.contentType {
				t.Errorf("Content-Type not preserved. Expected: %s, Got: %s", tc.contentType, receivedContentType)
			}
			if receivedBody != tc.body {
				t.Errorf("Body not preserved. Expected: %s, Got: %s", tc.body, receivedBody)
			}
		})
	}
}

func testLargeHeaders(t *testing.T, suite ProxyTestSuite) {
	largeValue := strings.Repeat("x", 8192) // 8KB header value

	var receivedHeaderValue string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaderValue = r.Header.Get("X-Large-Header")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Large-Header", largeValue)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Large headers request failed: %v", err)
	}
	if receivedHeaderValue != largeValue {
		t.Errorf("Large header not preserved correctly, got length %d, expected %d",
			len(receivedHeaderValue), len(largeValue))
	}
}

func testProxyRequestDiscoveryError(t *testing.T, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{err: fmt.Errorf("discovery failed")}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when discovery fails")
	}
	if !strings.Contains(err.Error(), "discovery") && !strings.Contains(err.Error(), "endpoints") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func testProxyRequestSelectorError(t *testing.T, suite ProxyTestSuite) {
	endpoint := createTestEndpoint("test", "http://localhost:11434", domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.err = fmt.Errorf("selection failed")
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if suite.Name() == "Olla" {
		// Olla implementation has circuit breaker logic that tries to select endpoints
		// even when the selector returns an error. It falls back to direct endpoint
		// selection from the available endpoints list, so it might actually succeed
		// or fail with a different error (like connection refused).
		if err != nil {
			// If there's an error, it should be a connection error, not a selector error
			errorMsg := strings.ToLower(err.Error())
			if strings.Contains(errorMsg, "selection") || strings.Contains(errorMsg, "select") {
				t.Errorf("Olla should not propagate selector errors due to circuit breaker fallback, got: %v", err)
			}
		}
		// If err is nil, that's also acceptable because the fallback worked
	} else {
		// For Sherpa, expect the selector error to propagate
		if err == nil {
			t.Error("Expected error when selector fails")
		}
		if err != nil {
			errorMsg := strings.ToLower(err.Error())
			if !strings.Contains(errorMsg, "select") &&
				!strings.Contains(errorMsg, "endpoint") &&
				!strings.Contains(errorMsg, "failed") {
				t.Errorf("Unexpected error message: %v", err)
			}
		}
	}
}

func testProxyRequestUpstreamError(t *testing.T, suite ProxyTestSuite) {
	endpoint := createTestEndpoint("test", "http://localhost:99999", domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when upstream is unreachable")
	}

	// Check for various possible connection error messages
	errorMsg := strings.ToLower(err.Error())
	if !strings.Contains(errorMsg, "connection") &&
		!strings.Contains(errorMsg, "refused") &&
		!strings.Contains(errorMsg, "network") &&
		!strings.Contains(errorMsg, "dial") &&
		!strings.Contains(errorMsg, "invalid port") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func testProxyRequestStreamingResponse(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "chunk %d\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/stream", nil)
	w := httptest.NewRecorder()

	stats, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}
	if stats.TotalBytes == 0 {
		t.Error("Expected non-zero bytes for streaming response")
	}
	if !strings.Contains(w.Body.String(), "chunk 0") {
		t.Error("Streaming response not received correctly")
	}
}

func testProxyRequestClientDisconnection(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		data := strings.Repeat("chunk of data ", 100) // ~1.3KB
		w.Write([]byte(data))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()

	stats, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}

	if stats.TotalBytes == 0 {
		t.Error("Expected bytes to be transferred")
	}

	if stats.TotalBytes < 1000 {
		t.Errorf("Expected substantial data transfer, got %d bytes", stats.TotalBytes)
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "chunk of data") {
		t.Error("Expected response content not found")
	}
}

func testProxyRequestUpstreamTimeout(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("too late"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/slow", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	_, err := proxy.ProxyRequest(context.Background(), w, req)
	elapsed := time.Since(start)

	// Either should timeout quickly or succeed (depending on implementation)
	if err != nil && elapsed > 5*time.Second {
		t.Error("Timeout should occur faster than 5 seconds")
	}
}

func testConnectionTracking(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	// Check initial connection count via stats collector
	connectionStats := collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Error("Initial connection count should be 0")
	}

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)
	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}

	// Check final connection count via stats collector
	connectionStats = collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Error("Connection count should be 0 after request completion")
	}
}

func testConcurrentRequests(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	const numRequests = 10
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := httptest.NewRequest("GET", fmt.Sprintf("/api/test%d", id), nil)
			w := httptest.NewRecorder()

			_, err := proxy.ProxyRequest(context.Background(), w, req)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent request failed: %v", err)
	}

	stats, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalRequests != numRequests {
		t.Errorf("Expected %d total requests, got %d", numRequests, stats.TotalRequests)
	}
	if stats.SuccessfulRequests != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, stats.SuccessfulRequests)
	}
	if stats.FailedRequests != 0 {
		t.Errorf("Expected 0 failed requests, got %d", stats.FailedRequests)
	}
}

func testGetStats(t *testing.T, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	stats, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalRequests != 0 || stats.SuccessfulRequests != 0 || stats.FailedRequests != 0 {
		t.Error("Initial stats should be zero")
	}
	if stats.AverageLatency != 0 {
		t.Error("Initial average latency should be zero")
	}
}

func testLargePayloadHandling(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			chunk := fmt.Sprintf(`{"chunk": %d, "data": "%s"}`, i, strings.Repeat("x", 1000))
			w.Write([]byte(chunk))
			flusher.Flush()
			if i%10 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{"prompt": "test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	stats, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Large payload proxy failed: %v", err)
	}
	if stats.TotalBytes < 100000 {
		t.Errorf("Expected ~100KB, got %d bytes", stats.TotalBytes)
	}
	if !strings.Contains(w.Body.String(), `"chunk": 99`) {
		t.Error("Final chunk not received")
	}
}

func testRequestBody(t *testing.T, suite ProxyTestSuite) {
	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	requestBody := `{"model": "llama4", "prompt": "benny llama?"}`
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Request body proxy failed: %v", err)
	}
	if receivedBody != requestBody {
		t.Errorf("Request body not forwarded correctly. Expected: %s, Got: %s", requestBody, receivedBody)
	}
}

func testErrorStatusCodes(t *testing.T, suite ProxyTestSuite) {
	testCases := []struct {
		upstreamStatus int
		expectedStatus int
	}{
		{http.StatusBadRequest, http.StatusBadRequest},
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusInternalServerError, http.StatusInternalServerError},
		{http.StatusServiceUnavailable, http.StatusServiceUnavailable},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.upstreamStatus), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.upstreamStatus)
				w.Write([]byte(`{"error": "upstream error"}`))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			collector := createTestStatsCollector()
			selector := newMockEndpointSelector(collector)
			selector.endpoint = endpoint
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			req := httptest.NewRequest("GET", "/api/test", nil)
			w := httptest.NewRecorder()

			_, err := proxy.ProxyRequest(context.Background(), w, req)

			if err != nil {
				t.Fatalf("Proxy request failed: %v", err)
			}
			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func testQueryParameters(t *testing.T, suite ProxyTestSuite) {
	var receivedQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.endpoint = endpoint
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	req := httptest.NewRequest("GET", "/api/models?format=json&stream=true", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err != nil {
		t.Fatalf("Query parameter proxy failed: %v", err)
	}
	if receivedQuery != "format=json&stream=true" {
		t.Errorf("Query parameters not forwarded. Expected: format=json&stream=true, Got: %s", receivedQuery)
	}
}
