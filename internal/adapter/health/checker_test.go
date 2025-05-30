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
	"github.com/thushan/olla/theme"
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

	healthy := make([]*domain.Endpoint, 0)
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

	routable := make([]*domain.Endpoint, 0)
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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
	for i := 0; i < DefaultCircuitBreakerThreshold; i++ {
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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
	styledLogger := logger.NewStyledLogger(log, theme.Default()) // Fix: add theme

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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, mockClient)
	ctx := context.Background()

	configs := make([]config.EndpointConfig, 5)
	for i := 0; i < 5; i++ {
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

	for i := 0; i < 10; i++ {
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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
	styledLogger := logger.NewStyledLogger(log, theme.Default())

	checker := NewHTTPHealthChecker(mockRepo, styledLogger, slowClient)

	configs := make([]config.EndpointConfig, 10)
	for i := 0; i < 10; i++ {
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

func TestHTTPHealthChecker_StatusCodeLogging(t *testing.T) {
	mockRepo := newMockRepository()

	statusCodes := []int{200, 404, 500, 503}
	mockClient := &statusCodeHTTPClient{
		statusCodes: statusCodes,
	}

	loggerCfg := &logger.Config{Level: "debug", Theme: "default"} // Debug to capture all logs
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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

func TestHTTPHealthChecker_ContextCancellation(t *testing.T) {
	mockRepo := newMockRepository()

	mockClient := &mockHTTPClient{
		statusCode: 200,
		delay:      100 * time.Millisecond,
	}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, theme.Default())

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
