package sherpa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	endpoints   []*domain.Endpoint
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

func TestRetryOnConnectionFailure(t *testing.T) {
	mockLogger := createRetryTestLogger()

	// Create test configuration
	config := &Configuration{
		ProxyPrefix:         "/olla/",
		ConnectionTimeout:   time.Second,
		ConnectionKeepAlive: time.Second,
		ResponseTimeout:     time.Second,
		ReadTimeout:         time.Second,
		StreamBufferSize:    8192,
	}

	// Create test endpoints - first will fail with connection error, second will succeed
	failingEndpoint := &domain.Endpoint{
		Name:   "failing",
		URL:    &url.URL{Scheme: "http", Host: "localhost:9999"}, // Non-existent port
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

	// Create service (not used in this test, but verifies it can be created)
	_, err := NewService(discoveryService, selector, config, nil, mockLogger)
	assert.NoError(t, err)

	// Create test request
	reqBody := bytes.NewBufferString("test body")
	req := httptest.NewRequest("POST", "/olla/test", reqBody)
	w := httptest.NewRecorder()

	stats := &ports.RequestStats{
		StartTime: time.Now(),
	}

	// Create retry handler
	retryHandler := core.NewRetryHandler(discoveryService, mockLogger)

	// Define proxy function
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error {
		if endpoint.Name == "failing" {
			// Simulate connection error
			return errors.New("dial tcp 127.0.0.1:9999: connect: connection refused")
		}
		// Success case
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
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
			expected: false,
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
	config := &Configuration{
		ProxyPrefix:         "/olla/",
		ConnectionTimeout:   100 * time.Millisecond,
		ConnectionKeepAlive: time.Second,
		ResponseTimeout:     100 * time.Millisecond,
		ReadTimeout:         100 * time.Millisecond,
		StreamBufferSize:    8192,
	}

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
	_, err := NewService(discoveryService, selector, config, nil, mockLogger)
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
	config := &Configuration{
		ProxyPrefix:         "/olla/",
		ConnectionTimeout:   time.Second,
		ConnectionKeepAlive: time.Second,
		ResponseTimeout:     time.Second,
		ReadTimeout:         time.Second,
		StreamBufferSize:    8192,
	}

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
	_, err := NewService(discoveryService, selector, config, nil, mockLogger)
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
