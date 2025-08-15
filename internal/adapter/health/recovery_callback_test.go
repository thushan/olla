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

	// Verify endpoint becomes unhealthy (or offline)
	assert.Eventually(t, func() bool {
		endpoints, _ = repo.GetAll(ctx)
		return endpoints[0].Status == domain.StatusUnhealthy || endpoints[0].Status == domain.StatusOffline
	}, 2*time.Second, 20*time.Millisecond, "Endpoint should become unhealthy after failed health check")

	// Verify callback was not called (no recovery yet)
	called, _ := recoveryCallback.wasCalledWith()
	assert.False(t, called)

	// Make server healthy
	serverIsHealthy = true

	// Check again - endpoint should recover
	endpoints, _ = repo.GetAll(ctx)
	checker.checkEndpoint(ctx, endpoints[0])

	// Verify callback was invoked with the expected endpoint and status
	assert.Eventually(t, func() bool {
		called, recoveredEndpoint := recoveryCallback.wasCalledWith()
		return called &&
			recoveredEndpoint != nil &&
			recoveredEndpoint.Name == "test-endpoint" &&
			recoveredEndpoint.Status == domain.StatusHealthy
	}, 2*time.Second, 20*time.Millisecond, "Recovery callback should be called with healthy endpoint")
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
