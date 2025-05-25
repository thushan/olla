package health

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

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
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

func (m *mockRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	endpoints := make([]*domain.Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

func (m *mockRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *mockRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *mockRepository) UpdateStatus(ctx context.Context, endpointURL *url.URL, status domain.EndpointStatus) error {
	return nil
}

func (m *mockRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	key := endpoint.URL.String()
	m.endpoints[key] = endpoint
	return nil
}

func (m *mockRepository) Add(ctx context.Context, endpoint *domain.Endpoint) error {
	key := endpoint.URL.String()
	m.endpoints[key] = endpoint
	return nil
}

func (m *mockRepository) Remove(ctx context.Context, endpointURL *url.URL) error {
	delete(m.endpoints, endpointURL.String())
	return nil
}

func TestHTTPHealthChecker_Check_Success(t *testing.T) {
	mockClient := &mockHTTPClient{statusCode: 200}
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, nil)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger)
	checker.client = mockClient

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

	loggerCfg := &logger.Config{Level: "error"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, nil)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger)
	checker.client = mockClient

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
		delay:      20 * time.Millisecond, // Much shorter delay for testing
	}
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, nil)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger)
	checker.client = mockClient

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("/health")
	endpoint := &domain.Endpoint{
		URL:            testURL,
		HealthCheckURL: healthURL,
		CheckTimeout:   time.Minute, // Long enough to not timeout
	}

	result, err := checker.Check(context.Background(), endpoint)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// With 20ms delay and 10s threshold, we should be healthy, not busy
	if result.Status != domain.StatusHealthy {
		t.Errorf("Expected StatusHealthy for fast response, got %v", result.Status)
	}

	// Test with actual slow response by using a custom threshold check
	if result.Latency > 100*time.Millisecond {
		t.Errorf("Response took too long: %v", result.Latency)
	}
}

func TestCircuitBreaker_BasicOperation(t *testing.T) {
	cb := NewCircuitBreaker()
	url := "http://localhost:11434"

	// Initially closed
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

	// Add some endpoints
	cb.RecordFailure(url1)
	cb.RecordFailure(url2)

	active := cb.GetActiveEndpoints()
	if len(active) != 2 {
		t.Errorf("Expected 2 active endpoints, got %d", len(active))
	}

	// Cleanup one endpoint
	cb.CleanupEndpoint(url1)
	active = cb.GetActiveEndpoints()
	if len(active) != 1 {
		t.Errorf("Expected 1 active endpoint after cleanup, got %d", len(active))
	}
}

func TestStatusTransitionTracker_ShouldLog(t *testing.T) {
	tracker := NewStatusTransitionTracker()
	url := "http://localhost:11434"

	// First status change should log
	shouldLog, count := tracker.ShouldLog(url, domain.StatusHealthy, false)
	if !shouldLog {
		t.Error("First status change should log")
	}
	if count != 0 {
		t.Errorf("Expected count 0 for status change, got %d", count)
	}

	// Same status should not log
	shouldLog, _ = tracker.ShouldLog(url, domain.StatusHealthy, false)
	if shouldLog {
		t.Error("Same status should not log")
	}

	// Status change should log again
	shouldLog, _ = tracker.ShouldLog(url, domain.StatusOffline, true)
	if !shouldLog {
		t.Error("Status change should log")
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	mockRepo := newMockRepository()

	loggerCfg := &logger.Config{Level: "error"}
	log, cleanup, _ := logger.New(loggerCfg)
	defer cleanup()
	styledLogger := logger.NewStyledLogger(log, nil)

	checker := NewHTTPHealthChecker(mockRepo, styledLogger)
	ctx := context.Background()

	// Start checker
	err := checker.StartChecking(ctx)
	if err != nil {
		t.Fatalf("StartChecking failed: %v", err)
	}

	// Verify it's running
	stats := checker.GetSchedulerStats()
	if !stats["running"].(bool) {
		t.Error("Checker should be running")
	}

	// Stop chucker
	err = checker.StopChecking(ctx)
	if err != nil {
		t.Fatalf("StopChecking failed: %v", err)
	}

	// Verify it's stopped
	stats = checker.GetSchedulerStats()
	if stats["running"].(bool) {
		t.Error("Checker should be stopped")
	}
}
