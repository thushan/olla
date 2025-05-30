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

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

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
	endpoint    *domain.Endpoint
	err         error
	connections map[string]int
	mu          sync.RWMutex
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connections == nil {
		m.connections = make(map[string]int)
	}
	m.connections[endpoint.URL.String()]++
}

func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connections == nil {
		m.connections = make(map[string]int)
	}
	if m.connections[endpoint.URL.String()] > 0 {
		m.connections[endpoint.URL.String()]--
	}
}

func (m *mockEndpointSelector) GetConnectionCount(endpoint *domain.Endpoint) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.connections == nil {
		return 0
	}
	return m.connections[endpoint.URL.String()]
}

func createTestLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}

func createTestEndpoint(name, urlStr string, status domain.EndpointStatus) *domain.Endpoint {
	testURL, _ := url.Parse(urlStr)
	return &domain.Endpoint{
		Name:   name,
		URL:    testURL,
		Status: status,
	}
}

func TestSherpaProxyService_ProxyRequest_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

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
	if !strings.Contains(w.Body.String(), "ok") {
		t.Error("Response body not proxied correctly")
	}

	if selector.GetConnectionCount(endpoint) != 0 {
		t.Error("Connection count should be 0 after request completion")
	}
}

func TestSherpaProxyService_ProxyRequest_NoEndpoints(t *testing.T) {
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{}}
	selector := &mockEndpointSelector{}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when no endpoints available")
	}
	if !strings.Contains(err.Error(), "no healthy endpoints available") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSherpaProxyService_ProxyRequest_DiscoveryError(t *testing.T) {
	discovery := &mockDiscoveryService{err: fmt.Errorf("discovery failed")}
	selector := &mockEndpointSelector{}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when discovery fails")
	}
	if !strings.Contains(err.Error(), "failed to get healthy endpoints") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSherpaProxyService_ProxyRequest_SelectorError(t *testing.T) {
	endpoint := createTestEndpoint("test", "http://localhost:11434", domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{err: fmt.Errorf("selection failed")}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when selector fails")
	}
	if !strings.Contains(err.Error(), "failed to select endpoint") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSherpaProxyService_ProxyRequest_UpstreamError(t *testing.T) {
	// endpoint pointing to non-existent server
	endpoint := createTestEndpoint("test", "http://localhost:99999", domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{ResponseTimeout: 1 * time.Second}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected error when upstream is unreachable")
	}
	if !strings.Contains(err.Error(), "upstream request failed") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSherpaProxyService_ProxyRequest_StreamingResponse(t *testing.T) {
	// upstream that streams response
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
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      5 * time.Second,
		StreamBufferSize: 1024,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_ProxyRequest_ClientDisconnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		data := strings.Repeat("chunk of data ", 100) // ~1.3KB
		w.Write([]byte(data))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout: 30 * time.Second,
		ReadTimeout:     5 * time.Second,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()

	// This should succeed and transfer data
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

func TestSherpaProxyService_ClientDisconnectLogic(t *testing.T) {
	// client disconnect decision logic separately
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

func TestSherpaProxyService_ProxyRequest_UpstreamTimeout(t *testing.T) {
	// Create upstream that takes too long to respond
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Longer than timeout
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("too late baby, it's too late"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout: 100 * time.Millisecond, // Shorten timeout
		ReadTimeout:     50 * time.Millisecond,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

	req := httptest.NewRequest("GET", "/api/slow", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)

	if err == nil {
		t.Error("Expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

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
		{"/ma/api/chat", "/ma/", "/api/chat"},
		{"/api/models", "/proxy/", "/api/models"}, // No prefix to strip
		{"/proxy/", "/proxy/", "/"},
		{"/proxy", "/proxy/", "/proxy"}, // Doesn't match prefix exactly, so no stripping
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

func TestSherpaProxyService_CopyHeaders(t *testing.T) {
	proxy := &SherpaProxyService{}

	originalReq := httptest.NewRequest("POST", "/api/test", strings.NewReader("test body"))
	originalReq.Header.Set("Content-Type", "application/json")
	originalReq.Header.Set("Authorization", "Bearer token123")
	originalReq.Header.Set("X-Custom-Header", "custom-value")
	originalReq.RemoteAddr = "192.168.1.100:12345"

	proxyReq := httptest.NewRequest("POST", "http://upstream/api/test", strings.NewReader("test body"))

	proxy.copyHeaders(proxyReq, originalReq)

	// make sure original headers were copied
	if proxyReq.Header.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header not copied")
	}
	if proxyReq.Header.Get("Authorization") != "Bearer token123" {
		t.Error("Authorization header not copied")
	}
	if proxyReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header not copied")
	}

	// forwarding headers we're added
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
}

func TestSherpaProxyService_ConnectionTracking(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	// initial connection count
	if selector.GetConnectionCount(endpoint) != 0 {
		t.Error("Initial connection count should be 0")
	}

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	_, err := proxy.ProxyRequest(context.Background(), w, req)
	if err != nil {
		t.Fatalf("ProxyRequest failed: %v", err)
	}

	// correction was tracked and released
	if selector.GetConnectionCount(endpoint) != 0 {
		t.Error("Connection count should be 0 after request completion")
	}
}

func TestSherpaProxyService_ConcurrentRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // faken some processing time :D
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{ResponseTimeout: 30 * time.Second}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_GetStats(t *testing.T) {
	discovery := &mockDiscoveryService{}
	selector := &mockEndpointSelector{}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	// Initial stats should be zero
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

func TestSherpaProxyService_LargePayloadHandling(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// chunking response to simulate large payload
		flusher := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			chunk := fmt.Sprintf(`{"chunk": %d, "data": "%s"}`, i, strings.Repeat("x", 1000))
			w.Write([]byte(chunk))
			flusher.Flush()
			if i%10 == 0 {
				time.Sleep(time.Millisecond) // Realistic chunking delay, thats what they tell me
			}
		}
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      5 * time.Second,
		StreamBufferSize: 4096,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_RequestBody(t *testing.T) {
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
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_ErrorStatusCodes(t *testing.T) {
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
			selector := &mockEndpointSelector{endpoint: endpoint}
			config := &Configuration{}

			proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_QueryParameters(t *testing.T) {
	var receivedQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_HTTPMethods(t *testing.T) {
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
			selector := &mockEndpointSelector{endpoint: endpoint}
			config := &Configuration{}

			proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_MultipleEndpoints(t *testing.T) {
	// multiple upstream servers
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
	selector := &mockEndpointSelector{} // Will select first available
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_SlowUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // fake processing time again, like a pro
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("processed"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{
		ResponseTimeout: 5 * time.Second, // Generous timeout
		ReadTimeout:     2 * time.Second,
	}

	proxy := NewService(discovery, selector, config, createTestLogger())

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

func TestSherpaProxyService_ConnectionPooling(t *testing.T) {
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("call_%d", atomic.LoadInt32(&callCount))))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	config := &Configuration{}

	proxy := NewService(discovery, selector, config, createTestLogger())

	// multiple requests quickly to test connection reuse
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
