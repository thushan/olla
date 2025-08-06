package olla

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

// TestEndpointPoolCleanup_NoMemoryLeak verifies endpoint pools are cleaned up
func TestEndpointPoolCleanup_NoMemoryLeak(t *testing.T) {
	// Create a minimal service with cleanup enabled
	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		configuration: &Configuration{
			MaxIdleConns:    10,
			MaxConnsPerHost: 5,
			IdleConnTimeout: 30 * time.Second,
		},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
		cleanupTicker:   time.NewTicker(100 * time.Millisecond), // Fast cleanup for testing
		cleanupStop:     make(chan struct{}),
	}

	// Start cleanup loop
	go s.cleanupLoop()
	defer func() {
		close(s.cleanupStop)
		s.cleanupTicker.Stop()
	}()

	// Create many endpoint pools
	for i := 0; i < 50; i++ {
		endpoint := fmt.Sprintf("endpoint-%d", i)
		pool := s.getOrCreateEndpointPool(endpoint)

		// Simulate some usage
		atomic.StoreInt64(&pool.lastUsed, time.Now().UnixNano())
	}

	// Verify pools were created
	poolCount := 0
	s.endpointPools.Range(func(k string, v *connectionPool) bool {
		poolCount++
		return true
	})
	if poolCount != 50 {
		t.Fatalf("Expected 50 pools, got %d", poolCount)
	}

	// Set all pools as old (last used > 5 minutes ago)
	oldTime := time.Now().Add(-10 * time.Minute).UnixNano()
	s.endpointPools.Range(func(k string, pool *connectionPool) bool {
		atomic.StoreInt64(&pool.lastUsed, oldTime)
		return true
	})

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Verify pools were cleaned up
	poolCount = 0
	s.endpointPools.Range(func(k string, v *connectionPool) bool {
		poolCount++
		return true
	})
	if poolCount != 0 {
		t.Errorf("Expected 0 pools after cleanup, got %d", poolCount)
	}
}

// TestCircuitBreakerCleanup_NoMemoryLeak verifies circuit breakers are cleaned up
func TestCircuitBreakerCleanup_NoMemoryLeak(t *testing.T) {
	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		configuration:   &Configuration{},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
		cleanupTicker:   time.NewTicker(100 * time.Millisecond),
		cleanupStop:     make(chan struct{}),
	}

	// Start cleanup loop
	go s.cleanupLoop()
	defer func() {
		close(s.cleanupStop)
		s.cleanupTicker.Stop()
	}()

	// Create circuit breakers without corresponding pools
	for i := 0; i < 30; i++ {
		endpoint := fmt.Sprintf("endpoint-%d", i)
		cb := s.GetCircuitBreaker(endpoint)
		// Set as closed with no recent failures
		atomic.StoreInt64(&cb.state, 0)
		atomic.StoreInt64(&cb.lastFailure, 0)
	}

	// Verify circuit breakers were created
	cbCount := 0
	s.circuitBreakers.Range(func(k string, v *circuitBreaker) bool {
		cbCount++
		return true
	})
	if cbCount != 30 {
		t.Fatalf("Expected 30 circuit breakers, got %d", cbCount)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify circuit breakers were cleaned up
	cbCount = 0
	s.circuitBreakers.Range(func(k string, v *circuitBreaker) bool {
		cbCount++
		return true
	})
	if cbCount != 0 {
		t.Errorf("Expected 0 circuit breakers after cleanup, got %d", cbCount)
	}
}

// TestCleanupLoop_ExitsCleanly verifies the cleanup goroutine exits properly
func TestCleanupLoop_ExitsCleanly(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create and start multiple services
	for i := 0; i < 5; i++ {
		s := &Service{
			BaseProxyComponents: &core.BaseProxyComponents{
				Logger: createTestLogger(),
			},
			configuration:   &Configuration{},
			endpointPools:   *xsync.NewMap[string, *connectionPool](),
			circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
			cleanupTicker:   time.NewTicker(50 * time.Millisecond),
			cleanupStop:     make(chan struct{}),
		}

		// Start cleanup loop
		go s.cleanupLoop()

		// Let it run briefly
		time.Sleep(100 * time.Millisecond)

		// Stop it
		close(s.cleanupStop)
		s.cleanupTicker.Stop()
	}

	// Give goroutines time to exit
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 { // Allow small variance
		t.Errorf("Goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

// TestRequestContextPool_Reset verifies request context is properly reset
func TestRequestContextPool_Reset(t *testing.T) {
	reqPool, err := pool.NewLitePool(func() *requestContext {
		return &requestContext{}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get a context and set fields
	ctx := reqPool.Get()
	ctx.requestID = "test-123"
	ctx.startTime = time.Now()
	ctx.endpoint = "test-endpoint"
	ctx.targetURL = "http://example.com"

	// Return to pool
	reqPool.Put(ctx)

	// Get it again - should be reset
	ctx2 := reqPool.Get()
	if ctx2.requestID != "" {
		t.Errorf("requestID not reset: %s", ctx2.requestID)
	}
	if !ctx2.startTime.IsZero() {
		t.Errorf("startTime not reset: %v", ctx2.startTime)
	}
	if ctx2.endpoint != "" {
		t.Errorf("endpoint not reset: %s", ctx2.endpoint)
	}
	if ctx2.targetURL != "" {
		t.Errorf("targetURL not reset: %s", ctx2.targetURL)
	}
}

// TestErrorContextPool_Reset verifies error context is properly reset
func TestErrorContextPool_Reset(t *testing.T) {
	errorPool, err := pool.NewLitePool(func() *errorContext {
		return &errorContext{}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get a context and set fields
	ctx := errorPool.Get()
	ctx.err = fmt.Errorf("test error")
	ctx.context = "test context"
	ctx.duration = 5 * time.Second
	ctx.code = 500
	ctx.allocated = true

	// Return to pool
	errorPool.Put(ctx)

	// Get it again - should be reset
	ctx2 := errorPool.Get()
	if ctx2.err != nil {
		t.Errorf("err not reset: %v", ctx2.err)
	}
	if ctx2.context != "" {
		t.Errorf("context not reset: %s", ctx2.context)
	}
	if ctx2.duration != 0 {
		t.Errorf("duration not reset: %v", ctx2.duration)
	}
	if ctx2.code != 0 {
		t.Errorf("code not reset: %d", ctx2.code)
	}
	if ctx2.allocated {
		t.Errorf("allocated not reset")
	}
}

// TestNewService_StartsCleanupGoroutine verifies cleanup goroutine starts
func TestNewService_StartsCleanupGoroutine(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create mock dependencies
	discovery := &mockDiscoveryService{}
	selector := &mockEndpointSelector{}
	collector := &mockStatsCollector{}
	logger := createTestLogger()

	config := &Configuration{
		StreamBufferSize: 8192,
	}

	// Create service
	service, err := NewService(discovery, selector, config, collector, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Give cleanup goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Check that we have one more goroutine (the cleanup loop)
	currentGoroutines := runtime.NumGoroutine()
	if currentGoroutines <= initialGoroutines {
		t.Error("Cleanup goroutine not started")
	}

	// Cleanup should stop the goroutine
	service.Cleanup()

	// Give time for goroutine to exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutines returned to initial count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 { // Allow small variance
		t.Errorf("Goroutine leak after Cleanup: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

// Mock types for testing

type mockDiscoveryService struct{}

func (m *mockDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return []*domain.Endpoint{}, nil
}

func (m *mockDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return []*domain.Endpoint{}, nil
}

func (m *mockDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	return nil
}

type mockEndpointSelector struct{}

func (m *mockEndpointSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints")
	}
	return endpoints[0], nil
}

func (m *mockEndpointSelector) Name() string {
	return "mock"
}

func (m *mockEndpointSelector) IncrementConnections(endpoint *domain.Endpoint) {}
func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {}

type mockStatsCollector struct{}

func (m *mockStatsCollector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollector) RecordConnection(endpoint *domain.Endpoint, delta int)     {}
func (m *mockStatsCollector) RecordSecurityViolation(violation ports.SecurityViolation) {}
func (m *mockStatsCollector) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
}
func (m *mockStatsCollector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
}
func (m *mockStatsCollector) GetModelStats() map[string]ports.ModelStats { return nil }
func (m *mockStatsCollector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	return nil
}
func (m *mockStatsCollector) GetProxyStats() ports.ProxyStats                  { return ports.ProxyStats{} }
func (m *mockStatsCollector) GetEndpointStats() map[string]ports.EndpointStats { return nil }
func (m *mockStatsCollector) GetSecurityStats() ports.SecurityStats            { return ports.SecurityStats{} }
func (m *mockStatsCollector) GetConnectionStats() map[string]int64             { return nil }

func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}
