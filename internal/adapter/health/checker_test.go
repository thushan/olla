package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type mockHTTPClient struct {
	statusCode int
	shouldErr  bool
	delay      time.Duration
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	if m.shouldErr {
		return nil, &mockNetError{timeout: false}
	}

	return &http.Response{
		StatusCode: m.statusCode,
		Body:       http.NoBody,
	}, nil
}

type mockNetError struct {
	timeout bool
}

func (e *mockNetError) Error() string   { return "mock network error" }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return false }

type mockRepository struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

func (m *mockRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoints := make([]*domain.Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

func (m *mockRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	healthy := make([]*domain.Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routable := make([]*domain.Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		if ep.Status.IsRoutable() {
			routable = append(routable, ep)
		}
	}
	return routable, nil
}

func (m *mockRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := endpoint.URL.String()
	m.endpoints[key] = endpoint
	return nil
}

func (m *mockRepository) LoadFromConfig(ctx context.Context, configs []config.EndpointConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.endpoints = make(map[string]*domain.Endpoint)
	for _, cfg := range configs {
		endpointURL, _ := url.Parse(cfg.URL)
		healthURL, _ := url.Parse(cfg.HealthCheckURL)

		endpoint := &domain.Endpoint{
			Name:                 cfg.Name,
			URL:                  endpointURL,
			HealthCheckURL:       healthURL,
			Status:               domain.StatusUnknown,
			CheckTimeout:         cfg.CheckTimeout,
			URLString:            endpointURL.String(),
			HealthCheckURLString: healthURL.String(),
		}
		m.endpoints[endpointURL.String()] = endpoint
	}
	return nil
}

func (m *mockRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.endpoints[endpointURL.String()]
	return exists
}

func TestHTTPHealthChecker_Check_Success(t *testing.T) {
	mockClient := &mockHTTPClient{statusCode: 200}
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("/health")
	endpoint := &domain.Endpoint{
		URL:            testURL,
		HealthCheckURL: healthURL,
		CheckTimeout:   time.Second,
	}

	result, err := checker.Check(context.Background(), endpoint)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result.Status != domain.StatusHealthy {
		t.Errorf("Expected StatusHealthy, got %v", result.Status)
	}
}

func TestHTTPHealthChecker_Check_NetworkError(t *testing.T) {
	mockClient := &mockHTTPClient{shouldErr: true}
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("/health")
	endpoint := &domain.Endpoint{
		URL:            testURL,
		HealthCheckURL: healthURL,
		CheckTimeout:   time.Second,
	}

	result, err := checker.Check(context.Background(), endpoint)

	if err == nil {
		t.Fatal("Expected error but got none")
	}
	if result.Status != domain.StatusOffline {
		t.Errorf("Expected StatusOffline, got %v", result.Status)
	}
}

func TestHTTPHealthChecker_Check_SlowResponse(t *testing.T) {
	mockClient := &mockHTTPClient{
		statusCode: 200,
		delay:      20 * time.Millisecond,
	}
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("/health")
	endpoint := &domain.Endpoint{
		URL:            testURL,
		HealthCheckURL: healthURL,
		CheckTimeout:   time.Minute,
	}

	result, err := checker.Check(context.Background(), endpoint)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Status != domain.StatusHealthy {
		t.Errorf("Expected StatusHealthy for fast response, got %v", result.Status)
	}

	if result.Latency > 100*time.Millisecond {
		t.Errorf("Response took too long: %v", result.Latency)
	}
}

func TestCircuitBreaker_BasicOperation(t *testing.T) {
	cb := NewCircuitBreaker()
	url := "http://localhost:11434"

	if cb.IsOpen(url) {
		t.Error("Circuit breaker should be closed initially")
	}

	// Record failures until it opens
	for range DefaultCircuitBreakerThreshold {
		cb.RecordFailure(url)
	}

	if !cb.IsOpen(url) {
		t.Error("Circuit breaker should be open after threshold failures")
	}

	// Record success should close it
	cb.RecordSuccess(url)
	if cb.IsOpen(url) {
		t.Error("Circuit breaker should be closed after success")
	}
}

func TestCircuitBreaker_Cleanup(t *testing.T) {
	cb := NewCircuitBreaker()
	url1 := "http://localhost:11434"
	url2 := "http://localhost:11435"

	cb.RecordFailure(url1)
	cb.RecordFailure(url2)

	active := cb.GetActiveEndpoints()
	if len(active) != 2 {
		t.Errorf("Expected 2 active endpoints, got %d", len(active))
	}

	cb.CleanupEndpoint(url1)
	active = cb.GetActiveEndpoints()
	if len(active) != 1 {
		t.Errorf("Expected 1 active endpoint after cleanup, got %d", len(active))
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, &mockHTTPClient{statusCode: 200})
	ctx := context.Background()

	err := checker.StartChecking(ctx)
	if err != nil {
		t.Fatalf("StartChecking failed: %v", err)
	}

	stats := checker.GetSchedulerStats()
	if !stats["isRunning"].(bool) {
		t.Error("Checker should be running")
	}

	err = checker.StopChecking(ctx)
	if err != nil {
		t.Fatalf("StopChecking failed: %v", err)
	}

	stats = checker.GetSchedulerStats()
	if stats["isRunning"].(bool) {
		t.Error("Checker should be stopped")
	}
}

func TestHTTPHealthChecker_ForceHealthCheck(t *testing.T) {
	mockRepo := newMockRepository()
	mockClient := &mockHTTPClient{statusCode: 200}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log) // Fix: add theme

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)
	ctx := context.Background()

	// Add some endpoints
	configs := []config.EndpointConfig{
		{
			Name:           "test-endpoint",
			URL:            "http://localhost:11434",
			HealthCheckURL: "/health",
			CheckTimeout:   time.Second,
		},
	}
	mockRepo.LoadFromConfig(ctx, configs)

	// Start checker
	checker.StartChecking(ctx)
	defer checker.StopChecking(ctx)

	// Force health check
	err := checker.RunHealthCheck(ctx, true)
	if err != nil {
		t.Fatalf("RunHealthCheck failed: %v", err)
	}

	// Verify endpoint was updated
	endpoints, _ := mockRepo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	if endpoints[0].Status != domain.StatusHealthy {
		t.Errorf("Expected healthy status after force check, got %v", endpoints[0].Status)
	}
}

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	mockRepo := newMockRepository()
	mockClient := &mockHTTPClient{statusCode: 200}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)
	ctx := context.Background()

	configs := make([]config.EndpointConfig, 5)
	for i := range 5 {
		configs[i] = config.EndpointConfig{
			Name:           fmt.Sprintf("endpoint-%d", i),
			URL:            fmt.Sprintf("http://localhost:%d", 11434+i),
			HealthCheckURL: "/health",
			CheckTimeout:   time.Second,
		}
	}
	mockRepo.LoadFromConfig(ctx, configs)

	err := checker.StartChecking(ctx)
	if err != nil {
		t.Fatalf("Failed to start health checker: %v", err)
	}
	defer checker.StopChecking(ctx)

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := checker.RunHealthCheck(ctx, false)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}
func TestHTTPHealthChecker_PanicRecovery(t *testing.T) {
	mockRepo := newMockRepository()

	panicClient := &panicHTTPClient{}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, panicClient)

	configs := []config.EndpointConfig{
		{
			Name:           "panic-endpoint",
			URL:            "http://localhost:11434",
			HealthCheckURL: "/health",
			CheckTimeout:   time.Second,
		},
	}
	mockRepo.LoadFromConfig(context.Background(), configs)

	ctx := context.Background()
	checker.StartChecking(ctx)
	defer checker.StopChecking(ctx)

	// This should not crash the test - panic should be recovered
	err := checker.RunHealthCheck(ctx, false)
	if err != nil {
		t.Fatalf("RunHealthCheck should not fail due to panic recovery: %v", err)
	}

	// Verify endpoint was still processed (even though it panicked)
	endpoints, _ := mockRepo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}
}
func TestHTTPHealthChecker_ConcurrentHealthChecks(t *testing.T) {
	mockRepo := newMockRepository()
	slowClient := &mockHTTPClient{
		statusCode: 200,
		delay:      50 * time.Millisecond, // Slow but not timeout
	}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, slowClient)

	configs := make([]config.EndpointConfig, 10)
	for i := range 10 {
		configs[i] = config.EndpointConfig{
			Name:           fmt.Sprintf("endpoint-%d", i),
			URL:            fmt.Sprintf("http://localhost:%d", 11434+i),
			HealthCheckURL: "/health",
			CheckTimeout:   time.Second,
		}
	}
	mockRepo.LoadFromConfig(context.Background(), configs)

	ctx := context.Background()
	checker.StartChecking(ctx)
	defer checker.StopChecking(ctx)

	// Time the health check to ensure concurrency is working
	start := time.Now()
	err := checker.RunHealthCheck(ctx, false)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("RunHealthCheck failed: %v", err)
	}

	// With 10 endpoints taking 50ms each, serial execution would take 500ms+
	// Concurrent execution should be much faster
	if duration > 200*time.Millisecond {
		t.Errorf("Health checks took too long (%v), may not be running concurrently", duration)
	}

	// Verify all endpoints were checked
	endpoints, _ := mockRepo.GetAll(ctx)
	for _, endpoint := range endpoints {
		if endpoint.Status != domain.StatusHealthy {
			t.Errorf("Endpoint %s not healthy after check: %v", endpoint.Name, endpoint.Status)
		}
	}
}

/*
	func TestHTTPHealthChecker_StatusCodeLogging(t *testing.T) {
		mockRepo := newMockRepository()

		statusCodes := []int{200, 404, 500, 503}
		mockClient := &statusCodeHTTPClient{
			statusCodes: statusCodes,
		}

		loggerCfg := &logger.Config{Level: "debug", Theme: "default"} // Debug to capture all logs
		log, cleanup, _ := logger.New(loggerCfg)
		defer cleanup()
		styledLogger := logger.NewPlainStyledLogger(log)

		checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)

		configs := make([]config.EndpointConfig, len(statusCodes))
		for i := range statusCodes {
			configs[i] = config.EndpointConfig{
				Name:           fmt.Sprintf("endpoint-%d", i),
				URL:            fmt.Sprintf("http://localhost:%d", 11434+i),
				HealthCheckURL: "/health",
				CheckTimeout:   time.Second,
			}
		}
		mockRepo.LoadFromConfig(context.Background(), configs)

		ctx := context.Background()
		checker.StartChecking(ctx)
		defer checker.StopChecking(ctx)

		err := checker.RunHealthCheck(ctx, false)
		if err != nil {
			t.Fatalf("RunHealthCheck failed: %v", err)
		}

		// Verify endpoints have different statuses based on status codes
		endpoints, _ := mockRepo.GetAll(ctx)

		expectedStatuses := map[int]domain.EndpointStatus{
			200: domain.StatusHealthy,
			404: domain.StatusUnhealthy,
			500: domain.StatusUnhealthy,
			503: domain.StatusUnhealthy,
		}

		for i, endpoint := range endpoints {
			expectedStatus := expectedStatuses[statusCodes[i]]
			if endpoint.Status != expectedStatus {
				t.Errorf("Endpoint %d: expected status %v for HTTP %d, got %v",
					i, expectedStatus, statusCodes[i], endpoint.Status)
			}
		}
	}
*/
func TestHTTPHealthChecker_ContextCancellation(t *testing.T) {
	mockRepo := newMockRepository()

	mockClient := &mockHTTPClient{
		statusCode: 200,
		delay:      100 * time.Millisecond,
	}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)

	configs := []config.EndpointConfig{
		{
			Name:           "test-endpoint",
			URL:            "http://localhost:11434",
			HealthCheckURL: "/health",
			CheckTimeout:   time.Second,
		},
	}
	mockRepo.LoadFromConfig(context.Background(), configs)

	// Start checker
	ctx, cancel := context.WithCancel(context.Background())
	checker.StartChecking(ctx)
	defer checker.StopChecking(ctx)

	// Cancel context quickly
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	// This should handle cancellation gracefully
	err := checker.RunHealthCheck(ctx, false)

	// The error might be due to context cancellation, which is expected
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context cancellation or no error, got: %v", err)
	}
}

// nonRetryableError is a plain error (not net.Error) so classifyError returns
// ErrorTypeHTTPError, which makes shouldRetry return false. This avoids the
// exponential-backoff retry delays inside HealthClient.Check during unit tests.
type nonRetryableError struct{}

func (e *nonRetryableError) Error() string { return "non-retryable test error" }

// nonRetryingHTTPClient returns a non-retryable error, collapsing the retry
// loop inside HealthClient to a single attempt for fast unit tests.
type nonRetryingHTTPClient struct{}

func (c *nonRetryingHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	return nil, &nonRetryableError{}
}

// TestUnhealthyCallbackPredicate verifies that the unhealthy callback fires only when
// an endpoint transitions from a routable state to a non-routable one. The previous
// predicate (newStatus != Healthy && oldStatus != Unknown) incorrectly fired on
// Healthy→Busy and Healthy→Warming, evicting sticky sessions unnecessarily.
func TestUnhealthyCallbackPredicate(t *testing.T) {
	t.Parallel()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	makeEndpoint := func(urlStr string, status domain.EndpointStatus) *domain.Endpoint {
		u, _ := url.Parse(urlStr)
		hcu, _ := url.Parse(urlStr + "/health")
		return &domain.Endpoint{
			Name:                 urlStr,
			URL:                  u,
			HealthCheckURL:       hcu,
			URLString:            u.String(),
			HealthCheckURLString: hcu.String(),
			Status:               status,
			CheckTimeout:         time.Second,
		}
	}

	// errClient returns a non-retryable error → StatusUnhealthy (non-routable).
	// Using a plain error (not net.Error) avoids the retry-backoff delay in HealthClient.
	errClient := &nonRetryingHTTPClient{}
	// okClient returns HTTP 200 → StatusHealthy (routable)
	okClient := &mockHTTPClient{statusCode: 200}

	tests := []struct {
		name      string
		oldStatus domain.EndpointStatus
		client    HTTPClient
		wantFired bool
	}{
		// Routable → non-routable: callback must fire.
		{name: "Healthy→Unhealthy fires", oldStatus: domain.StatusHealthy, client: errClient, wantFired: true},
		{name: "Busy→Unhealthy fires", oldStatus: domain.StatusBusy, client: errClient, wantFired: true},
		{name: "Warming→Unhealthy fires", oldStatus: domain.StatusWarming, client: errClient, wantFired: true},

		// Already non-routable → non-routable: nothing was pinned, so no purge.
		{name: "Unknown→Unhealthy no fire", oldStatus: domain.StatusUnknown, client: errClient, wantFired: false},
		{name: "Offline→Unhealthy no fire", oldStatus: domain.StatusOffline, client: errClient, wantFired: false},

		// Routable → routable: keep sticky sessions intact.
		{name: "Healthy→Healthy no fire (no change)", oldStatus: domain.StatusHealthy, client: okClient, wantFired: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fired := make(chan struct{}, 1)
			repo := newMockRepository()
			ep := makeEndpoint("http://127.0.0.1:19999", tc.oldStatus)
			repo.mu.Lock()
			repo.endpoints[ep.URLString] = ep
			repo.mu.Unlock()

			checker := NewHTTPHealthChecker(repo, styledLogger, tc.client)
			checker.SetUnhealthyCallback(UnhealthyCallbackFunc(func(_ context.Context, _ *domain.Endpoint) {
				select {
				case fired <- struct{}{}:
				default:
				}
			}))

			ctx := context.Background()
			checker.checkEndpoint(ctx, ep)

			// The callback dispatches asynchronously in a goroutine; give it a
			// short window to fire before concluding it won't.
			var gotFired bool
			select {
			case <-fired:
				gotFired = true
			case <-time.After(200 * time.Millisecond):
			}

			if gotFired && !tc.wantFired {
				t.Errorf("unhealthy callback fired but should not have (old=%s)", tc.oldStatus)
			}
			if !gotFired && tc.wantFired {
				t.Errorf("unhealthy callback not fired but should have (old=%s)", tc.oldStatus)
			}
		})
	}
}

type panicHTTPClient struct{}

func (p *panicHTTPClient) Do(req *http.Request) (*http.Response, error) {
	panic("simulated panic in health check")
}

type statusCodeHTTPClient struct {
	statusCodes []int
	callCount   int
}

func (s *statusCodeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	statusCode := s.statusCodes[s.callCount%len(s.statusCodes)]
	s.callCount++
	return &http.Response{
		StatusCode: statusCode,
		Body:       http.NoBody,
	}, nil
}

// TestStopChecking_DoubleInvoke verifies concurrent double-stops do not panic.
// Previously, two callers that both passed the isRunning.Load() guard could
// race to close(stopCh), causing a "close of closed channel" panic.
func TestStopChecking_DoubleInvoke(t *testing.T) {
	t.Parallel()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	mockRepo := newMockRepository()
	checker := NewHTTPHealthChecker(mockRepo, styledLogger, &mockHTTPClient{statusCode: 200})

	// Start the checker so isRunning == true.
	if err := checker.StartChecking(context.Background()); err != nil {
		t.Fatalf("StartChecking: %v", err)
	}

	// Two concurrent stops - neither should panic.
	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_ = checker.StopChecking(context.Background())
		}()
	}
	wg.Wait()
}

// panicRepository panics on GetAll after a configurable number of successful calls,
// then returns normally - used to verify the healthCheckLoop survives a tick panic.
type panicRepository struct {
	*mockRepository
	callsUntilPanic int
	calls           int
	mu              sync.Mutex
}

func (p *panicRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	p.mu.Lock()
	p.calls++
	calls := p.calls
	p.mu.Unlock()

	if calls == p.callsUntilPanic {
		panic("injected panic in GetAll")
	}
	return p.mockRepository.GetAll(ctx)
}

// TestHealthCheckLoop_SurvivesPanic verifies that a panic inside performHealthChecks
// does not kill the healthCheckLoop goroutine. The loop must continue firing on
// subsequent ticks and the isRunning flag must remain true after the panic.
func TestHealthCheckLoop_SurvivesPanic(t *testing.T) {
	t.Parallel()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	panicRepo := &panicRepository{
		mockRepository:  newMockRepository(),
		callsUntilPanic: 1, // panic on the first tick
	}

	// Use a very short ticker interval so the test doesn't have to wait long.
	checker := NewHTTPHealthChecker(panicRepo, styledLogger, &mockHTTPClient{statusCode: 200})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker.isRunning.Store(true)
	checker.ticker = time.NewTicker(20 * time.Millisecond)
	go checker.healthCheckLoop(ctx)

	// Wait long enough for two ticks (the first panics, the second must succeed).
	time.Sleep(120 * time.Millisecond)

	// isRunning must still be true - the loop survived the panic.
	if !checker.isRunning.Load() {
		t.Error("isRunning is false after tick panic; loop likely died")
	}

	// The second GetAll call (tick 2) must have happened, proving the loop continued.
	panicRepo.mu.Lock()
	calls := panicRepo.calls
	panicRepo.mu.Unlock()

	if calls < 2 {
		t.Errorf("GetAll called %d times; expected >= 2 (loop must have continued past the panic)", calls)
	}

	_ = checker.StopChecking(ctx)
}

// blockingHTTPClient blocks in Do until the request context is cancelled, then
// records that cancellation was observed. Used to verify that in-flight checks
// are cancelled when the loop context is cancelled rather than running to their
// full timeout.
type blockingHTTPClient struct {
	doEntered chan struct{} // closed the first time Do is entered, before it waits on ctx
	cancelled chan struct{} // closed when a Do call observes ctx cancellation
	doOnce    sync.Once
}

func (b *blockingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	b.doOnce.Do(func() { close(b.doEntered) })
	<-req.Context().Done()
	select {
	case <-b.cancelled:
	default:
		close(b.cancelled)
	}
	return nil, req.Context().Err()
}

// TestHealthCheckLoop_CancelledContextAbortsInFlightChecks verifies that when the
// loop context is cancelled, any in-flight health check context is also cancelled
// rather than running to its full per-tick timeout. Before the fix, checkCtx was
// derived from context.Background() so cancelling the loop ctx had no effect on
// the check in progress.
func TestHealthCheckLoop_CancelledContextAbortsInFlightChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shutdown-cancellation test in short mode")
	}
	t.Parallel()

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewPlainStyledLogger(log)

	repo := newMockRepository()

	testURL, _ := url.Parse("http://localhost:19999")
	healthURL, _ := url.Parse("http://localhost:19999/health")
	ep := &domain.Endpoint{
		Name:           "blocking-ep",
		URL:            testURL,
		HealthCheckURL: healthURL,
		CheckTimeout:   5 * time.Second,
		URLString:      testURL.String(),
	}
	repo.mu.Lock()
	repo.endpoints[testURL.String()] = ep
	repo.mu.Unlock()

	blocking := &blockingHTTPClient{doEntered: make(chan struct{}), cancelled: make(chan struct{})}
	checker := NewHTTPHealthChecker(repo, styledLogger, blocking)

	ctx, cancel := context.WithCancel(context.Background())
	// Also release the context if an assertion below aborts the test early.
	defer cancel()

	// Drive a single performHealthChecks tick directly from a goroutine, then
	// cancel the ctx once the check is genuinely in flight. We skip the ticker
	// so the test is deterministic.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Per-tick timeout must also be short so cancellation is observable.
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkCancel()
		checker.performHealthChecks(checkCtx)
	}()

	// Wait until the check has actually reached the blocking Do() call before
	// cancelling. performHealthChecks' per-endpoint goroutine races a
	// buffered-semaphore send against ctx.Done() in a single select - cancelling
	// too early means both branches can be simultaneously ready, and select
	// picks between ready cases at random (documented Go behaviour, not a
	// product bug: either branch is safe - skip the check on a cancelled
	// context, or start it and let the downstream cancelled context fail it
	// fast). That's what made this test flaky: on an unlucky draw the endpoint
	// goroutine would return via the ctx.Done() branch without ever calling
	// Do(), so blocking.cancelled would never close and the test would hang
	// for the full timeout. Synchronising on doEntered removes the race by
	// only cancelling once the check is genuinely blocked inside Do().
	select {
	case <-blocking.doEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("health check never reached the blocking Do() call")
	}

	cancel()

	// The in-flight Do call should observe the cancellation promptly -
	// cancellation propagates synchronously through the context tree once
	// cancel() is called.
	select {
	case <-blocking.cancelled:
		// pass - in-flight check was cancelled
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight health check was not cancelled within 2s after loop context was cancelled")
	}

	// Join the goroutine before returning so it can't linger past this test
	// under go test -count N or alongside t.Parallel() siblings.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("performHealthChecks goroutine did not exit after cancellation")
	}
}
