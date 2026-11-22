package sherpa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// Simple test implementation of DiscoveryServiceWithEndpointUpdate
type testDiscoveryService struct {
	endpoints         []*domain.Endpoint
	updateStatusCalls []string
}

func (t *testDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return t.endpoints, nil
}

func (t *testDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return t.endpoints, nil
}

func (t *testDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (t *testDiscoveryService) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	t.updateStatusCalls = append(t.updateStatusCalls, endpoint.Name)
	return nil
}

// Simple test implementation of EndpointSelector
type testSelector struct {
	selectCount int
	selectIndex int
}

func (t *testSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no endpoints")
	}

	// For testing retry logic, return endpoints in order
	if t.selectIndex < len(endpoints) {
		ep := endpoints[t.selectIndex]
		t.selectIndex++
		t.selectCount++
		return ep, nil
	}
	return endpoints[0], nil
}

func (t *testSelector) Name() string {
	return "test"
}

func (t *testSelector) IncrementConnections(endpoint *domain.Endpoint) {}
func (t *testSelector) DecrementConnections(endpoint *domain.Endpoint) {}

func createRetryTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

type eofTrackingConn struct {
	net.Conn
	sawEOF *atomic.Bool
}

func (c *eofTrackingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if errors.Is(err, io.EOF) {
		c.sawEOF.Store(true)
	}
	return n, err
}

func TestRetryOnConnectionFailure(t *testing.T) {
	mockLogger := createRetryTestLogger()

	// Create test configuration
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ConnectionTimeout = time.Second
	config.ConnectionKeepAlive = time.Second
	config.ResponseTimeout = time.Second
	config.ReadTimeout = time.Second
	config.StreamBufferSize = 8192

	// Create a backend that closes the connection before sending response headers.
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the request to avoid a connection reset masking the EOF path.
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read backend request body: %v", err)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support connection hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack backend connection: %v", err)
		}
		conn.Close()
	}))
	defer failingServer.Close()

	failingURL, _ := url.Parse(failingServer.URL)
	failingEndpoint := &domain.Endpoint{
		Name:   "failing",
		URL:    failingURL,
		Status: domain.StatusHealthy,
	}

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer successServer.Close()

	successURL, _ := url.Parse(successServer.URL)
	successEndpoint := &domain.Endpoint{
		Name:   "success",
		URL:    successURL,
		Status: domain.StatusHealthy,
	}

	// Create test discovery service
	discoveryService := &testDiscoveryService{
		endpoints: []*domain.Endpoint{failingEndpoint, successEndpoint},
	}

	// Create test selector that returns endpoints in order
	selector := &testSelector{}

	// Create the service so the test exercises the production proxy path.
	service, err := NewService(discoveryService, selector, config, nil, nil, mockLogger)
	assert.NoError(t, err)
	defer service.Cleanup()
	var sawEOF atomic.Bool
	dialContext := service.transport.DialContext
	service.transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialContext(ctx, network, address)
		if err != nil || address != failingURL.Host {
			return conn, err
		}
		return &eofTrackingConn{Conn: conn, sawEOF: &sawEOF}, nil
	}

	// Create test request
	reqBody := bytes.NewBufferString("test body")
	req := httptest.NewRequest("POST", "/olla/test", reqBody)
	w := httptest.NewRecorder()

	stats := &ports.RequestStats{
		StartTime: time.Now(),
	}

	// Execute through the production proxy path.
	err = service.ProxyRequestToEndpointsWithRetry(
		context.Background(),
		w,
		req,
		[]*domain.Endpoint{failingEndpoint, successEndpoint},
		stats,
		mockLogger,
	)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())
	assert.True(t, sawEOF.Load(), "failing backend must produce EOF")

	// Verify that UpdateEndpointStatus was called for the failing endpoint
	assert.Contains(t, discoveryService.updateStatusCalls, "failing")
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:9999: connect: connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read tcp 127.0.0.1:1234->127.0.0.1:5678: connection reset by peer"),
			expected: true,
		},
		{
			name:     "timeout",
			err:      errors.New("dial tcp 127.0.0.1:9999: i/o timeout"),
			expected: true,
		},
		{
			name:     "no such host",
			err:      errors.New("dial tcp: lookup invalid.host: no such host"),
			expected: true,
		},
		{
			name:     "non-connection error",
			err:      errors.New("invalid request"),
			expected: false,
		},
		{
			name:     "EOF error",
			err:      io.EOF,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.IsConnectionError(tt.err)
			assert.Equal(t, tt.expected, result, "IsConnectionError(%v) = %v, want %v", tt.err, result, tt.expected)
		})
	}
}

func TestRetryExhaustsAllEndpoints(t *testing.T) {
	mockLogger := createRetryTestLogger()

	// Create test configuration
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ConnectionTimeout = 100 * time.Millisecond
	config.ConnectionKeepAlive = time.Second
	config.ResponseTimeout = 100 * time.Millisecond
	config.ReadTimeout = 100 * time.Millisecond
	config.StreamBufferSize = 8192

	// Create test endpoints - all will fail with connection errors
	endpoint1 := &domain.Endpoint{
		Name:   "endpoint1",
		URL:    &url.URL{Scheme: "http", Host: "localhost:9991"},
		Status: domain.StatusHealthy,
	}

	endpoint2 := &domain.Endpoint{
		Name:   "endpoint2",
		URL:    &url.URL{Scheme: "http", Host: "localhost:9992"},
		Status: domain.StatusHealthy,
	}

	endpoint3 := &domain.Endpoint{
		Name:   "endpoint3",
		URL:    &url.URL{Scheme: "http", Host: "localhost:9993"},
		Status: domain.StatusHealthy,
	}

	discoveryService := &testDiscoveryService{
		endpoints: []*domain.Endpoint{endpoint1, endpoint2, endpoint3},
	}

	selector := &testSelector{}

	// Create service (not used in this test, but verifies it can be created)
	_, err := NewService(discoveryService, selector, config, nil, nil, mockLogger)
	assert.NoError(t, err)

	// Create test request
	req := httptest.NewRequest("GET", "/olla/test", nil)
	w := httptest.NewRecorder()

	stats := &ports.RequestStats{
		StartTime: time.Now(),
	}

	// Create retry handler
	retryHandler := core.NewRetryHandler(discoveryService, mockLogger)

	// Define proxy function that always fails with connection error
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error {
		return fmt.Errorf("dial tcp %s: connect: connection refused", endpoint.URL.Host)
	}

	// Execute with retry
	err = retryHandler.ExecuteWithRetry(
		context.Background(),
		w,
		req,
		[]*domain.Endpoint{endpoint1, endpoint2, endpoint3},
		selector,
		stats,
		proxyFunc,
	)

	// Verify that all endpoints were tried and failed
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all endpoints failed")

	// Verify that all endpoints were marked as unhealthy
	assert.Len(t, discoveryService.updateStatusCalls, 3)
}

func TestRetryPreservesRequestBody(t *testing.T) {
	mockLogger := createRetryTestLogger()

	// Create test configuration
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ConnectionTimeout = time.Second
	config.ConnectionKeepAlive = time.Second
	config.ResponseTimeout = time.Second
	config.ReadTimeout = time.Second
	config.StreamBufferSize = 8192

	// Track request bodies received
	var receivedBodies []string

	// Create endpoints
	failingEndpoint := &domain.Endpoint{
		Name:   "failing",
		URL:    &url.URL{Scheme: "http", Host: "localhost:9999"},
		Status: domain.StatusHealthy,
	}

	successEndpoint := &domain.Endpoint{
		Name:   "success",
		URL:    &url.URL{Scheme: "http", Host: "localhost:8888"},
		Status: domain.StatusHealthy,
	}

	discoveryService := &testDiscoveryService{
		endpoints: []*domain.Endpoint{failingEndpoint, successEndpoint},
	}

	selector := &testSelector{}

	// Create service (not used in this test, but verifies it can be created)
	_, err := NewService(discoveryService, selector, config, nil, nil, mockLogger)
	assert.NoError(t, err)

	// Create test request with body
	testBody := "test request body content"
	req := httptest.NewRequest("POST", "/olla/test", strings.NewReader(testBody))
	w := httptest.NewRecorder()

	stats := &ports.RequestStats{
		StartTime: time.Now(),
	}

	// Create retry handler
	retryHandler := core.NewRetryHandler(discoveryService, mockLogger)

	attemptCount := 0
	// Define proxy function that fails first, then succeeds
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error {
		// Read the body to verify it's preserved
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			receivedBodies = append(receivedBodies, string(body))
		}

		attemptCount++
		if attemptCount == 1 {
			// First attempt fails
			return errors.New("dial tcp 127.0.0.1:9999: connect: connection refused")
		}
		// Second attempt succeeds
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "success")
		return nil
	}

	// Execute with retry
	err = retryHandler.ExecuteWithRetry(
		context.Background(),
		w,
		req,
		[]*domain.Endpoint{failingEndpoint, successEndpoint},
		selector,
		stats,
		proxyFunc,
	)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())

	// Verify that the request body was preserved through the retry
	assert.Len(t, receivedBodies, 2)
	assert.Equal(t, testBody, receivedBodies[0])
	assert.Equal(t, testBody, receivedBodies[1])
}
