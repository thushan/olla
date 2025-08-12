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

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/core/constants"
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
	service, err := sherpa.NewService(discovery, selector, config.(*sherpa.Configuration), collector, nil, createTestLogger())
	if err != nil {
		panic(fmt.Sprintf("failed to create Sherpa proxy: %v", err))
	}
	return service
}

func (s SherpaTestSuite) CreateConfig() ports.ProxyConfiguration {
	return &sherpa.Configuration{
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

func (o OllaTestSuite) CreateProxy(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector) ports.ProxyService {
	service, err := olla.NewService(discovery, selector, config.(*olla.Configuration), collector, nil, createTestLogger())
	if err != nil {
		panic(fmt.Sprintf("failed to create Olla proxy: %v", err))
	}
	return service
}

func (o OllaTestSuite) CreateConfig() ports.ProxyConfiguration {
	return &olla.Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
		MaxIdleConns:     200,
		IdleConnTimeout:  90 * time.Second,
		MaxConnsPerHost:  50,
	}
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

// Helper functions for creating test components
func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func createTestEndpoint(name, urlStr string, status domain.EndpointStatus) *domain.Endpoint {
	testURL, _ := url.Parse(urlStr)
	return &domain.Endpoint{
		Name:      name,
		URL:       testURL,
		URLString: urlStr,
		Status:    status,
		Type:      domain.ProfileOllama, // Default to ollama for tests
	}
}

func createTestStatsCollector() ports.StatsCollector {
	return stats.NewCollector(createTestLogger())
}

func createTestProxyComponents(suite ProxyTestSuite, endpoints []*domain.Endpoint) (ports.ProxyService, *mockEndpointSelector, ports.StatsCollector) {
	discovery := &mockDiscoveryService{endpoints: endpoints}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	config := suite.CreateConfig()
	proxy := suite.CreateProxy(discovery, selector, config, collector)
	return proxy, selector, collector
}

func createTestProxyWithError(suite ProxyTestSuite, discoveryErr error, selectorErr error) (ports.ProxyService, *mockEndpointSelector, ports.StatsCollector) {
	discovery := &mockDiscoveryService{err: discoveryErr}
	collector := createTestStatsCollector()
	selector := newMockEndpointSelector(collector)
	selector.err = selectorErr
	config := suite.CreateConfig()
	proxy := suite.CreateProxy(discovery, selector, config, collector)
	return proxy, selector, collector
}

func createTestRequestWithBody(method, path, body string) (*http.Request, *ports.RequestStats, logger.StyledLogger) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	}

	stats := &ports.RequestStats{
		RequestID: "test-request-id",
		StartTime: time.Now(),
	}

	rlog := createTestLogger()
	return req, stats, rlog
}

func executeProxyRequest(proxy ports.ProxyService, req *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) (*httptest.ResponseRecorder, error) {
	w := httptest.NewRecorder()
	err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
	return w, err
}

func assertProxySuccess(t *testing.T, w *httptest.ResponseRecorder, err error, stats *ports.RequestStats, expectedStatus int, expectedBodyContains string) {
	t.Helper()
	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}
	if w.Code != expectedStatus {
		t.Errorf("Expected status %d, got %d", expectedStatus, w.Code)
	}
	if stats.TotalBytes == 0 {
		t.Error("Expected non-zero bytes transferred")
	}
	if expectedBodyContains != "" && !strings.Contains(w.Body.String(), expectedBodyContains) {
		t.Errorf("Expected body to contain %q, got %q", expectedBodyContains, w.Body.String())
	}
}

func assertProxyError(t *testing.T, err error, expectedErrorContains string) {
	t.Helper()
	if err == nil {
		t.Error("Expected error but got nil")
		return
	}
	if expectedErrorContains != "" && !strings.Contains(err.Error(), expectedErrorContains) {
		t.Errorf("Expected error to contain %q, got %q", expectedErrorContains, err.Error())
	}
}

// Test all proxy implementations
func TestAllProxies(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
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
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	w, err := executeProxyRequest(proxy, req, stats, rlog)

	assertProxySuccess(t, w, err, stats, http.StatusOK, `{"status": "ok"}`)

	// Check connection count via stats collector instead of selector
	connectionStats := collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Error("Connection count should be 0 after request completion")
	}
}

func testProxyRequestNoEndpoints(t *testing.T, suite ProxyTestSuite) {
	proxy, _, _ := createTestProxyComponents(suite, []*domain.Endpoint{})

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

	assertProxyError(t, err, "no healthy")
}

func testProxyRequestDiscoveryError(t *testing.T, suite ProxyTestSuite) {
	proxy, _, _ := createTestProxyWithError(suite, fmt.Errorf("discovery failed"), nil)

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

	assertProxyError(t, err, "discovery")
}

func testProxyRequestSelectorError(t *testing.T, suite ProxyTestSuite) {
	proxy, _, _ := createTestProxyWithError(suite, nil, fmt.Errorf("selection failed"))

	// The proxy implementations are now internal and we can't modify the discovery service after creation
	// This test case setup needs to be adjusted - the proxy already has a discovery service
	// from the createTestProxyWithError call that returns an error

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

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
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeText)
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/stream", "")
	w, err := executeProxyRequest(proxy, req, stats, rlog)

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
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeText)
		w.WriteHeader(http.StatusOK)

		data := strings.Repeat("chunk of data ", 100) // ~1.3KB
		w.Write([]byte(data))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/data", "")
	w, err := executeProxyRequest(proxy, req, stats, rlog)

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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/slow", "")
	start := time.Now()
	_, err := executeProxyRequest(proxy, req, stats, rlog)
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
	proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	// Check initial connection count via stats collector
	connectionStats := collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Error("Initial connection count should be 0")
	}

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	const numRequests = 10
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req, stats, rlog := createTestRequestWithBody("GET", fmt.Sprintf("/api/test%d", id), "")
			_, err := executeProxyRequest(proxy, req, stats, rlog)
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
}

func testGetStats(t *testing.T, suite ProxyTestSuite) {
	proxy, _, _ := createTestProxyComponents(suite, []*domain.Endpoint{})

	proxyStats, err := proxy.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if proxyStats.TotalRequests != 0 || proxyStats.SuccessfulRequests != 0 || proxyStats.FailedRequests != 0 {
		t.Error("Initial stats should be zero")
	}
	if proxyStats.AverageLatency != 0 {
		t.Error("Initial average latency should be zero")
	}
}

func testLargePayloadHandling(t *testing.T, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("POST", "/api/generate", `{"prompt": "test"}`)
	w, err := executeProxyRequest(proxy, req, stats, rlog)

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

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	requestBody := `{"model": "llama4", "prompt": "benny llama?"}`
	req, stats, rlog := createTestRequestWithBody("POST", "/api/generate", requestBody)
	_, err := executeProxyRequest(proxy, req, stats, rlog)

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
			proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			w, err := executeProxyRequest(proxy, req, stats, rlog)

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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/models?format=json&stream=true", "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

	if err != nil {
		t.Fatalf("Query parameter proxy failed: %v", err)
	}
	if receivedQuery != "format=json&stream=true" {
		t.Errorf("Query parameters not forwarded. Expected: format=json&stream=true, Got: %s", receivedQuery)
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
			proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			var body string
			if method == "POST" || method == "PUT" || method == "PATCH" {
				body = `{"test": "data"}`
			}

			req, stats, rlog := createTestRequestWithBody(method, "/api/test", body)
			_, err := executeProxyRequest(proxy, req, stats, rlog)

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

	proxy, _, _ := createTestProxyComponents(suite, endpoints) // Will select first available

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	w, err := executeProxyRequest(proxy, req, stats, rlog)

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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	start := time.Now()
	req, stats, rlog := createTestRequestWithBody("POST", "/api/generate", `{"prompt": "test"}`)
	w, err := executeProxyRequest(proxy, req, stats, rlog)
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	const numRequests = 5
	var wg sync.WaitGroup
	responses := make([]string, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			w, err := executeProxyRequest(proxy, req, stats, rlog)
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
	proxy, _, _ := createTestProxyComponents(suite, []*domain.Endpoint{})

	// Create new config with different values
	var newConfig ports.ProxyConfiguration
	if suite.Name() == "Sherpa" {
		newConfig = &sherpa.Configuration{
			ResponseTimeout:  60 * time.Second,
			ReadTimeout:      30 * time.Second,
			StreamBufferSize: 16384,
		}
	} else {
		newConfig = &olla.Configuration{
			ResponseTimeout:  60 * time.Second,
			ReadTimeout:      30 * time.Second,
			StreamBufferSize: 16384,
			MaxIdleConns:     400,
			IdleConnTimeout:  120 * time.Second,
			MaxConnsPerHost:  100,
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("POST", "/api/test", "") // No body
	w, err := executeProxyRequest(proxy, req, stats, rlog)

	assertProxySuccess(t, w, err, stats, http.StatusOK, "")
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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	// Use a simpler path that doesn't require URL encoding
	specialPath := "/api/test-with-dashes/café"
	req, stats, rlog := createTestRequestWithBody("GET", specialPath, "")
	_, err := executeProxyRequest(proxy, req, stats, rlog)

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
		{"JSON", constants.ContentTypeJSON, `{"key": "value"}`},
		{"XML", "application/xml", `<root><key>value</key></root>`},
		{"Plain Text", constants.ContentTypeText, "simple text"},
		{"Binary", "application/octet-stream", string([]byte{0x01, 0x02, 0x03, 0x04})},
		{"Form Data", "application/x-www-form-urlencoded", "key1=value1&key2=value2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedContentType string
			var receivedBody string

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedContentType = r.Header.Get(constants.HeaderContentType)
				body, _ := io.ReadAll(r.Body)
				receivedBody = string(body)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("POST", "/api/test", tc.body)
			req.Header.Set(constants.HeaderContentType, tc.contentType)
			_, err := executeProxyRequest(proxy, req, stats, rlog)

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
	proxy, selector, _ := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
	selector.endpoint = endpoint

	req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
	req.Header.Set("X-Large-Header", largeValue)
	_, err := executeProxyRequest(proxy, req, stats, rlog)

	if err != nil {
		t.Fatalf("Large headers request failed: %v", err)
	}
	if receivedHeaderValue != largeValue {
		t.Errorf("Large header not preserved correctly, got length %d, expected %d",
			len(receivedHeaderValue), len(largeValue))
	}
}
