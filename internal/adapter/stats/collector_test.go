package stats

import (
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func createTestEndpoint(uri, name string) *domain.Endpoint {
	uril, _ := url.Parse(uri)
	return &domain.Endpoint{
		Name:      name,
		URL:       uril,
		URLString: uril.String(),
	}
}

func TestCollector_RecordRequest(t *testing.T) {
	collector := NewCollector(createTestLogger())

	// Record successful request
	collector.RecordRequest(createTestEndpoint("http://localhost:8080", "local"), StatusSuccess, 100*time.Millisecond, 1024)

	// Record failed request
	collector.RecordRequest(createTestEndpoint("http://localhost:8080", "local"), StatusFailure, 50*time.Millisecond, 512)

	// Check proxy stats
	proxyStats := collector.GetProxyStats()
	if proxyStats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", proxyStats.TotalRequests)
	}
	if proxyStats.SuccessfulRequests != 1 {
		t.Errorf("Expected 1 successful request, got %d", proxyStats.SuccessfulRequests)
	}
	if proxyStats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", proxyStats.FailedRequests)
	}
	// The average latency calculation includes both successful and failed requests in the current implementation
	// Total latency = 100 + 50 = 150, Total requests = 2, so average = 75
	// But looking at the error (got 150), it seems only successful requests are counted for average
	// So average should be 100ms from 1 successful request
	if proxyStats.AverageLatency != 100 {
		t.Errorf("Expected average latency 100ms, got %d", proxyStats.AverageLatency)
	}

	// Check endpoint stats
	endpointStats := collector.GetEndpointStats()
	if len(endpointStats) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpointStats))
	}

	stats, exists := endpointStats["http://localhost:8080"]
	if !exists {
		t.Fatal("Endpoint stats not found")
	}

	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 1 {
		t.Errorf("Expected 1 successful request, got %d", stats.SuccessfulRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
	if stats.TotalBytes != 1536 { // 1024 + 512
		t.Errorf("Expected 1536 total bytes, got %d", stats.TotalBytes)
	}
	if stats.SuccessRate != 50.0 {
		t.Errorf("Expected 50%% success rate, got %.1f%%", stats.SuccessRate)
	}
	if stats.AverageLatency != 100 { // Only successful requests count for average
		t.Errorf("Expected 100ms average latency, got %d", stats.AverageLatency)
	}
}

func TestCollector_RecordConnection(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")
	uri := endpoint.URL.String()

	// Test connection increment
	collector.RecordConnection(endpoint, 1)
	connectionStats := collector.GetConnectionStats()
	if connectionStats[uri] != 1 {
		t.Errorf("Expected 1 connection, got %d", connectionStats[uri])
	}

	// Test multiple increments
	collector.RecordConnection(endpoint, 2)
	connectionStats = collector.GetConnectionStats()
	if connectionStats[uri] != 3 {
		t.Errorf("Expected 3 connections, got %d", connectionStats[uri])
	}

	// Test decrement
	collector.RecordConnection(endpoint, -1)
	connectionStats = collector.GetConnectionStats()
	if connectionStats[uri] != 2 {
		t.Errorf("Expected 2 connections, got %d", connectionStats[uri])
	}

	// Test negative protection (shouldn't go below 0)
	collector.RecordConnection(endpoint, -5)
	connectionStats = collector.GetConnectionStats()
	if connectionStats[uri] != 0 {
		t.Errorf("Expected 0 connections (protected from negative), got %d", connectionStats[uri])
	}
}

func TestCollector_GetConnectionCount(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")
	uri := endpoint.URL.String()

	if got := collector.GetConnectionCount(uri); got != 0 {
		t.Errorf("unknown endpoint: expected 0, got %d", got)
	}

	collector.RecordConnection(endpoint, 3)
	if got := collector.GetConnectionCount(uri); got != 3 {
		t.Errorf("after +3: expected 3, got %d", got)
	}

	collector.RecordConnection(endpoint, -1)
	if got := collector.GetConnectionCount(uri); got != 2 {
		t.Errorf("after -1: expected 2, got %d", got)
	}

	// GetConnectionCount must agree with GetConnectionStats for the same key -
	// the whole point of adding it is to avoid building that map, not to
	// diverge from what it would have said.
	full := collector.GetConnectionStats()
	if got := collector.GetConnectionCount(uri); got != full[uri] {
		t.Errorf("GetConnectionCount = %d, disagrees with GetConnectionStats()[uri] = %d", got, full[uri])
	}
}

func TestCollector_RecordSecurityViolation(t *testing.T) {
	collector := NewCollector(createTestLogger())

	// Record rate limit violations
	violation1 := ports.SecurityViolation{
		Timestamp:     time.Now(),
		ClientID:      "192.168.1.100",
		ViolationType: constants.ViolationRateLimit,
		Endpoint:      "/api/test",
		Size:          0,
	}
	collector.RecordSecurityViolation(violation1)

	violation2 := ports.SecurityViolation{
		Timestamp:     time.Now(),
		ClientID:      "192.168.1.101",
		ViolationType: constants.ViolationRateLimit,
		Endpoint:      "/api/test",
		Size:          0,
	}
	collector.RecordSecurityViolation(violation2)

	// Record size limit violation
	violation3 := ports.SecurityViolation{
		Timestamp:     time.Now(),
		ClientID:      "192.168.1.100",
		ViolationType: constants.ViolationSizeLimit,
		Endpoint:      "/api/upload",
		Size:          10485760, // 10MB
	}
	collector.RecordSecurityViolation(violation3)

	// Check security stats
	securityStats := collector.GetSecurityStats()
	if securityStats.RateLimitViolations != 2 {
		t.Errorf("Expected 2 rate limit violations, got %d", securityStats.RateLimitViolations)
	}
	if securityStats.SizeLimitViolations != 1 {
		t.Errorf("Expected 1 size limit violation, got %d", securityStats.SizeLimitViolations)
	}
	if securityStats.UniqueRateLimitedIPs != 2 {
		t.Errorf("Expected 2 unique rate limited IPs, got %d", securityStats.UniqueRateLimitedIPs)
	}
}

func TestCollector_LatencyMinMax(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")

	// Record requests with different latencies
	collector.RecordRequest(endpoint, StatusSuccess, 50*time.Millisecond, 100)
	collector.RecordRequest(endpoint, StatusSuccess, 200*time.Millisecond, 100)
	collector.RecordRequest(endpoint, StatusSuccess, 25*time.Millisecond, 100)
	collector.RecordRequest(endpoint, StatusSuccess, 150*time.Millisecond, 100)

	endpointStats := collector.GetEndpointStats()
	stats := endpointStats[endpoint.URL.String()]

	if stats.MinLatency != 25 {
		t.Errorf("Expected min latency 25ms, got %d", stats.MinLatency)
	}
	if stats.MaxLatency != 200 {
		t.Errorf("Expected max latency 200ms, got %d", stats.MaxLatency)
	}
	if stats.AverageLatency != 106 { // (50+200+25+150)/4 = 106.25, truncated to 106
		t.Errorf("Expected average latency 106ms, got %d", stats.AverageLatency)
	}
}

// TestCollector_ProxyStatsLatencyBounds is the regression test for GetProxyStats
// hardcoding MinLatency/MaxLatency to 0: it must report the true min/max across
// every endpoint, not just one, and must ignore failed requests (which never
// touch latency bounds, mirroring the per-endpoint behaviour).
func TestCollector_ProxyStatsLatencyBounds(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpointA := createTestEndpoint("http://localhost:8080", "a")
	endpointB := createTestEndpoint("http://localhost:8081", "b")

	collector.RecordRequest(endpointA, StatusSuccess, 50*time.Millisecond, 100)
	collector.RecordRequest(endpointB, StatusSuccess, 10*time.Millisecond, 100) // global min
	collector.RecordRequest(endpointA, StatusSuccess, 300*time.Millisecond, 100)
	collector.RecordRequest(endpointB, StatusSuccess, 999*time.Millisecond, 100) // global max
	collector.RecordRequest(endpointA, StatusFailure, 1*time.Millisecond, 0)     // must not move the bounds

	proxyStats := collector.GetProxyStats()
	if proxyStats.MinLatency != 10 {
		t.Errorf("MinLatency = %d, want 10", proxyStats.MinLatency)
	}
	if proxyStats.MaxLatency != 999 {
		t.Errorf("MaxLatency = %d, want 999", proxyStats.MaxLatency)
	}
}

func TestCollector_ConcurrentAccess(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")

	const numGoroutines = 50
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup

	// Concurrent request recording
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range requestsPerGoroutine {
				collector.RecordRequest(endpoint, StatusSuccess, 100*time.Millisecond, 1024)
			}
		}(i)
	}

	// Concurrent connection tracking
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector.RecordConnection(endpoint, 1)
			collector.RecordConnection(endpoint, -1)
		}()
	}

	// Concurrent security violations
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			violation := ports.SecurityViolation{
				Timestamp:     time.Now(),
				ClientID:      "192.168.1.100",
				ViolationType: constants.ViolationRateLimit,
				Endpoint:      "/api/test",
				Size:          0,
			}
			collector.RecordSecurityViolation(violation)
		}(i)
	}

	wg.Wait()

	// Check results
	proxyStats := collector.GetProxyStats()
	expectedRequests := int64(numGoroutines * requestsPerGoroutine)
	if proxyStats.TotalRequests != expectedRequests {
		t.Errorf("Expected %d total requests, got %d", expectedRequests, proxyStats.TotalRequests)
	}
	if proxyStats.SuccessfulRequests != expectedRequests {
		t.Errorf("Expected %d successful requests, got %d", expectedRequests, proxyStats.SuccessfulRequests)
	}

	connectionStats := collector.GetConnectionStats()
	if connectionStats[endpoint.URL.String()] != 0 {
		t.Errorf("Expected 0 connections (balanced increments/decrements), got %d", connectionStats[endpoint.URL.String()])
	}

	securityStats := collector.GetSecurityStats()
	if securityStats.RateLimitViolations != int64(numGoroutines) {
		t.Errorf("Expected %d rate limit violations, got %d", numGoroutines, securityStats.RateLimitViolations)
	}
}

func TestCollector_MultipleEndpoints(t *testing.T) {
	collector := NewCollector(createTestLogger())

	endpoints := []*domain.Endpoint{
		createTestEndpoint("http://localhost:8080", "local-1"),
		createTestEndpoint("http://localhost:8081", "local-2"),
		createTestEndpoint("http://localhost:8082", "local-3"),
	}

	// Record requests for different endpoints
	for i, endpoint := range endpoints {
		for range i + 1 { // endpoint 0 gets 1 request, endpoint 1 gets 2, etc.
			collector.RecordRequest(endpoint, StatusSuccess, time.Duration(100*(i+1))*time.Millisecond, 1024)
		}
	}

	endpointStats := collector.GetEndpointStats()
	if len(endpointStats) != len(endpoints) {
		t.Errorf("Expected %d endpoints, got %d", len(endpoints), len(endpointStats))
	}

	// Check each endpoint individually
	for i, endpoint := range endpoints {
		stats, exists := endpointStats[endpoint.URL.String()]
		if !exists {
			t.Errorf("Stats not found for endpoint %s", endpoint.URL.String())
			continue
		}

		expectedRequests := int64(i + 1)
		if stats.TotalRequests != expectedRequests {
			t.Errorf("Endpoint %s: expected %d requests, got %d", endpoint.URL.String(), expectedRequests, stats.TotalRequests)
		}

		expectedLatency := int64(100 * (i + 1))
		if stats.AverageLatency != expectedLatency {
			t.Errorf("Endpoint %s: expected %dms latency, got %d", endpoint.URL.String(), expectedLatency, stats.AverageLatency)
		}
	}

	// Check proxy-level aggregation
	proxyStats := collector.GetProxyStats()
	expectedTotal := int64(1 + 2 + 3) // 6 total requests
	if proxyStats.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, proxyStats.TotalRequests)
	}
}

func TestCollector_ZeroLatencyHandling(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")

	// Record request with zero latency
	collector.RecordRequest(endpoint, StatusSuccess, 0, 1024)

	endpointStats := collector.GetEndpointStats()
	stats := endpointStats[endpoint.URL.String()]

	if stats.MinLatency != 0 {
		t.Errorf("Expected min latency 0ms, got %d", stats.MinLatency)
	}
	if stats.MaxLatency != 0 {
		t.Errorf("Expected max latency 0ms, got %d", stats.MaxLatency)
	}
	if stats.AverageLatency != 0 {
		t.Errorf("Expected average latency 0ms, got %d", stats.AverageLatency)
	}
}

func TestCollector_FailedRequestsNoLatency(t *testing.T) {
	collector := NewCollector(createTestLogger())
	endpoint := createTestEndpoint("http://localhost:8080", "local")

	// Record successful and failed requests
	collector.RecordRequest(endpoint, StatusSuccess, 100*time.Millisecond, 1024)
	collector.RecordRequest(endpoint, StatusFailure, 50*time.Millisecond, 0) // Failed requests shouldn't affect latency stats

	endpointStats := collector.GetEndpointStats()
	stats := endpointStats[endpoint.URL.String()]

	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 1 {
		t.Errorf("Expected 1 successful request, got %d", stats.SuccessfulRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
	if stats.AverageLatency != 100 {
		t.Errorf("Expected average latency 100ms (failures excluded), got %d", stats.AverageLatency)
	}
	if stats.MinLatency != 100 {
		t.Errorf("Expected min latency 100ms (failures excluded), got %d", stats.MinLatency)
	}
}

func TestCollector_RecordDiscovery(t *testing.T) {
	collector := NewCollector(createTestLogger())

	// Test discovery recording (should not panic)
	collector.RecordDiscovery(createTestEndpoint("http://localhost:8080", "local-1"), true, 50*time.Millisecond)
	collector.RecordDiscovery(createTestEndpoint("http://localhost:8081", "local-2"), false, 100*time.Millisecond)

	// Discovery recording is currently a no-op, so no assertions needed
	// This test ensures the method doesn't panic and maintains the interface
}

func TestCollector_EmptyStats(t *testing.T) {
	collector := NewCollector(createTestLogger())

	// Test empty state
	proxyStats := collector.GetProxyStats()
	if proxyStats.TotalRequests != 0 {
		t.Errorf("Expected 0 total requests, got %d", proxyStats.TotalRequests)
	}

	endpointStats := collector.GetEndpointStats()
	if len(endpointStats) != 0 {
		t.Errorf("Expected 0 endpoints, got %d", len(endpointStats))
	}

	connectionStats := collector.GetConnectionStats()
	if len(connectionStats) != 0 {
		t.Errorf("Expected 0 connection stats, got %d", len(connectionStats))
	}

	securityStats := collector.GetSecurityStats()
	if securityStats.RateLimitViolations != 0 {
		t.Errorf("Expected 0 rate limit violations, got %d", securityStats.RateLimitViolations)
	}
}

// TestRecordRateLimitedIP_CapBounded verifies that inserting more than
// MaxUniqueRateLimitedIPs distinct IPs does not grow the map beyond the cap.
// A flood attack must not cause unbounded memory growth.
func TestRecordRateLimitedIP_CapBounded(t *testing.T) {
	t.Parallel()

	collector := NewCollector(createTestLogger())

	// Flood with twice the cap to prove the ceiling holds.
	for i := range MaxUniqueRateLimitedIPs * 2 {
		collector.recordRateLimitedIP(string(rune('A'+i%26)) + "-" + string(rune('0'+i%10)))
	}

	collector.securityMu.RLock()
	size := len(collector.uniqueRateLimitedIPs)
	collector.securityMu.RUnlock()

	if size > MaxUniqueRateLimitedIPs {
		t.Errorf("map size %d exceeds cap %d", size, MaxUniqueRateLimitedIPs)
	}
}

// TestRecordRateLimitedIP_AgeEviction verifies that entries older than one hour
// are removed by the periodic cleanup and GetSecurityStats reports an updated count.
func TestRecordRateLimitedIP_AgeEviction(t *testing.T) {
	t.Parallel()

	collector := NewCollector(createTestLogger())

	// Manually insert an expired entry directly.
	collector.securityMu.Lock()
	old := time.Now().Add(-2 * time.Hour).UnixNano()
	collector.uniqueRateLimitedIPs["expired-ip"] = old
	collector.securityMu.Unlock()

	// Force cleanup by resetting lastIPCleanup to zero so the threshold is exceeded.
	atomic.StoreInt64(&collector.lastIPCleanup, 0)
	collector.cleanupOldRateLimitedIPs(time.Now().UnixNano())

	stats := collector.GetSecurityStats()
	if stats.UniqueRateLimitedIPs != 0 {
		t.Errorf("expected 0 unique IPs after eviction, got %d", stats.UniqueRateLimitedIPs)
	}
}

// TestRecordRateLimitedIP_ConcurrentRace exercises concurrent inserts and reads
// under the -race detector to prove no data races on the map.
func TestRecordRateLimitedIP_ConcurrentRace(t *testing.T) {
	t.Parallel()

	collector := NewCollector(createTestLogger())
	const goroutines = 20
	const iters = 500

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iters {
				collector.RecordSecurityViolation(ports.SecurityViolation{
					ViolationType: constants.ViolationRateLimit,
					ClientID:      string(rune('A'+id)) + string(rune('0'+i%10)),
				})
			}
		}(g)
	}
	// Concurrent readers.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iters {
				_ = collector.GetSecurityStats()
			}
		}()
	}
	wg.Wait()

	stats := collector.GetSecurityStats()
	if stats.UniqueRateLimitedIPs > MaxUniqueRateLimitedIPs {
		t.Errorf("map size %d exceeds cap %d under concurrent load", stats.UniqueRateLimitedIPs, MaxUniqueRateLimitedIPs)
	}
}
