package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type testRecoveryCallback struct {
	mu       sync.Mutex
	called   bool
	endpoint *domain.Endpoint
}

func (t *testRecoveryCallback) OnEndpointRecovered(ctx context.Context, endpoint *domain.Endpoint) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.called = true
	t.endpoint = endpoint
	return nil
}

func (t *testRecoveryCallback) wasCalledWith() (bool, *domain.Endpoint) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.called, t.endpoint
}

func TestHealthCheckerRecoveryCallback(t *testing.T) {
	// Create a test server that will be "down" initially then come back up
	serverIsHealthy := false
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serverIsHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer testServer.Close()

	// Create test endpoint
	endpointURL, _ := url.Parse(testServer.URL)
	endpoint := &domain.Endpoint{
		Name:                 "test-endpoint",
		URL:                  endpointURL,
		URLString:            testServer.URL,
		HealthCheckURL:       endpointURL,
		HealthCheckURLString: testServer.URL,
		Status:               domain.StatusUnknown, // Start as unknown
		CheckInterval:        100 * time.Millisecond,
		CheckTimeout:         50 * time.Millisecond,
	}

	// Create test repository and add endpoint directly
	repo := discovery.NewTestStaticEndpointRepository()
	// Add the endpoint to the repository (for testing)
	repo.AddTestEndpoint(endpoint)

	// Create logger
	logCfg := &logger.Config{Level: "error"}
	log, _, _ := logger.New(logCfg)
	testLogger := logger.NewPlainStyledLogger(log)

	// Create health checker with callback (using the embedded repository)
	checker := NewHTTPHealthCheckerWithDefaults(repo.StaticEndpointRepository, testLogger)

	// Set up recovery callback
	recoveryCallback := &testRecoveryCallback{}
	checker.SetRecoveryCallback(recoveryCallback)

	ctx := context.Background()

	// Start health checking
	err := checker.StartChecking(ctx)
	assert.NoError(t, err)
	defer checker.StopChecking(ctx)

	// Initial check - endpoint should be unhealthy
	// Get endpoint from repo to ensure we have the latest state
	endpoints, _ := repo.GetAll(ctx)
	checker.checkEndpoint(ctx, endpoints[0])

	// Wait for status to be updated in repository
	time.Sleep(50 * time.Millisecond)

	// Verify endpoint is now unhealthy (or offline)
	endpoints, _ = repo.GetAll(ctx)
	assert.True(t, endpoints[0].Status == domain.StatusUnhealthy || endpoints[0].Status == domain.StatusOffline)

	// Verify callback was not called (no recovery yet)
	assert.False(t, recoveryCallback.called)

	// Make server healthy
	serverIsHealthy = true

	// Check again - endpoint should recover
	endpoints, _ = repo.GetAll(ctx)
	checker.checkEndpoint(ctx, endpoints[0])

	// Give the async callback time to execute
	time.Sleep(100 * time.Millisecond)

	// Verify callback was called using thread-safe accessor
	called, recoveredEndpoint := recoveryCallback.wasCalledWith()
	assert.True(t, called)
	assert.NotNil(t, recoveredEndpoint)
	assert.Equal(t, "test-endpoint", recoveredEndpoint.Name)
	assert.Equal(t, domain.StatusHealthy, recoveredEndpoint.Status)
}

func TestRecoveryCallbackFunc(t *testing.T) {
	called := false
	var capturedEndpoint *domain.Endpoint

	callbackFunc := RecoveryCallbackFunc(func(ctx context.Context, endpoint *domain.Endpoint) error {
		called = true
		capturedEndpoint = endpoint
		return nil
	})

	testEndpoint := &domain.Endpoint{
		Name:   "test",
		Status: domain.StatusHealthy,
	}

	err := callbackFunc.OnEndpointRecovered(context.Background(), testEndpoint)

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, testEndpoint, capturedEndpoint)
}

func TestNoOpRecoveryCallback(t *testing.T) {
	callback := NoOpRecoveryCallback{}

	err := callback.OnEndpointRecovered(context.Background(), &domain.Endpoint{
		Name: "test",
	})

	assert.NoError(t, err)
}
