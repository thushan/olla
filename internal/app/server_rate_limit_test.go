package app

import (
	"fmt"
	"github.com/thushan/olla/internal/util"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

func createTestRateLimitLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}

func TestNewRateLimiter(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               50,
		HealthRequestsPerMinute: 500,
		CleanupInterval:         time.Minute,
		IPExtractionTrustProxy:  true,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	if rl.globalRequestsPerMinute != 1000 {
		t.Errorf("Expected global limit 1000, got %d", rl.globalRequestsPerMinute)
	}
	if rl.perIPRequestsPerMinute != 100 {
		t.Errorf("Expected per-IP limit 100, got %d", rl.perIPRequestsPerMinute)
	}
	if rl.burstSize != 50 {
		t.Errorf("Expected burst size 50, got %d", rl.burstSize)
	}
	if !rl.trustProxyHeaders {
		t.Error("Expected trust proxy headers to be true")
	}
}

func TestRateLimiter_ExtractClientIP(t *testing.T) {
	testCases := []struct {
		name          string
		trustProxy    bool
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		expectedIP    string
	}{
		{
			name:       "direct connection",
			trustProxy: false,
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:          "x-forwarded-for with trust",
			trustProxy:    true,
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.1, 10.0.0.1",
			expectedIP:    "203.0.113.1",
		},
		{
			name:          "x-forwarded-for without trust",
			trustProxy:    false,
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.1, 10.0.0.1",
			expectedIP:    "10.0.0.1",
		},
		{
			name:       "x-real-ip with trust",
			trustProxy: true,
			remoteAddr: "10.0.0.1:12345",
			xRealIP:    "203.0.113.2",
			expectedIP: "203.0.113.2",
		},
		{
			name:       "x-real-ip without trust",
			trustProxy: false,
			remoteAddr: "10.0.0.1:12345",
			xRealIP:    "203.0.113.2",
			expectedIP: "10.0.0.1",
		},
		{
			name:          "x-forwarded-for priority over x-real-ip",
			trustProxy:    true,
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.1",
			xRealIP:       "203.0.113.2",
			expectedIP:    "203.0.113.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			limits := config.ServerRateLimits{
				IPExtractionTrustProxy: tc.trustProxy,
			}
			rl := NewRateLimiter(limits, createTestRateLimitLogger())
			defer rl.Stop()

			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tc.xForwardedFor)
			}
			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}

			ip := util.GetClientIP(req, rl.trustProxyHeaders)
			if ip != tc.expectedIP {
				t.Errorf("Expected IP %s, got %s", tc.expectedIP, ip)
			}
		})
	}
}

func TestRateLimiter_BasicRateLimit(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  5, // Very low for testing
		BurstSize:               3,
		HealthRequestsPerMinute: 10,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	middleware := rl.Middleware(false) // Not health endpoint

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Should allow burst requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d should be allowed, got status %d", i+1, w.Code)
		}

		// Check rate limit headers
		if limit := w.Header().Get("X-RateLimit-Limit"); limit != "5" {
			t.Errorf("Expected limit 5, got %s", limit)
		}

		remaining, _ := strconv.Atoi(w.Header().Get("X-RateLimit-Remaining"))
		if remaining != 2-i {
			t.Errorf("Expected remaining %d, got %d", 2-i, remaining)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", w.Code)
	}
	if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
		t.Error("Expected Retry-After header")
	}
	if remaining := w.Header().Get("X-RateLimit-Remaining"); remaining != "0" {
		t.Errorf("Expected remaining 0, got %s", remaining)
	}
}

func TestRateLimiter_PerIPIsolation(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  2,
		BurstSize:               2,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	middleware := rl.Middleware(false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First IP uses its quota
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP1 request %d should be allowed", i+1)
		}
	}

	// First IP should be rate limited
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusTooManyRequests {
		t.Error("IP1 should be rate limited")
	}

	// Second IP should still be allowed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.101:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Error("IP2 should be allowed")
	}
}

func TestRateLimiter_HealthEndpointSeparateLimits(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  2,  // Low limit for regular endpoints
		HealthRequestsPerMinute: 10, // Higher limit for health
		BurstSize:               5,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	regularMiddleware := rl.Middleware(false)
	healthMiddleware := rl.Middleware(true)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	regularHandler := regularMiddleware(handler)
	healthHandler := healthMiddleware(handler)

	clientIP := "192.168.1.100:12345"

	// Use up regular endpoint quota (2 requests)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = clientIP
		w := httptest.NewRecorder()
		regularHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Regular request %d should be allowed", i+1)
		}
	}

	// Regular endpoint should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = clientIP
	w := httptest.NewRecorder()
	regularHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Error("Regular endpoint should be rate limited")
	}

	// Health endpoint should still work with higher limit
	healthReq := httptest.NewRequest("GET", "/health", nil)
	healthReq.RemoteAddr = clientIP
	healthW := httptest.NewRecorder()
	healthHandler.ServeHTTP(healthW, healthReq)

	if healthW.Code != http.StatusOK {
		t.Error("Health endpoint should be allowed")
	}

	// Check that health endpoint has correct limit in headers
	if limit := healthW.Header().Get("X-RateLimit-Limit"); limit != "10" {
		t.Errorf("Expected health limit 10, got %s", limit)
	}
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 3, // Very low global limit
		PerIPRequestsPerMinute:  10,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	middleware := rl.Middleware(false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up global quota with different IPs
	ips := []string{"192.168.1.100:12345", "192.168.1.101:12345", "192.168.1.102:12345"}

	for i, ip := range ips {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Global request %d should be allowed", i+1)
		}
	}

	// Next request from any IP should be globally rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.103:12345" // Different IP
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Error("Should be globally rate limited")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  50,
		BurstSize:               25,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	middleware := rl.Middleware(false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	successCount := int32(0)
	rateLimitedCount := int32(0)

	// Launch multiple goroutines making requests concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.100:12345" // Same IP for all
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if w.Code == http.StatusTooManyRequests {
				atomic.AddInt32(&rateLimitedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	totalRequests := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&rateLimitedCount)
	if totalRequests != 100 {
		t.Errorf("Expected 100 total requests, got %d", totalRequests)
	}

	// Should allow some requests (up to burst size) and rate limit the rest
	if atomic.LoadInt32(&successCount) == 0 {
		t.Error("Expected some successful requests")
	}
	if atomic.LoadInt32(&rateLimitedCount) == 0 {
		t.Error("Expected some rate limited requests")
	}

	t.Logf("Successful: %d, Rate limited: %d",
		atomic.LoadInt32(&successCount),
		atomic.LoadInt32(&rateLimitedCount))
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 60, // 1 per second
		PerIPRequestsPerMinute:  60, // 1 per second
		BurstSize:               2,
		CleanupInterval:         time.Minute,
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	middleware := rl.Middleware(false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	clientIP := "192.168.1.100:12345"

	// Use up burst capacity
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = clientIP
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Burst request %d should be allowed", i+1)
		}
	}

	// Should be rate limited now
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = clientIP
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Error("Should be rate limited after burst")
	}

	// Wait for token refill (slightly more than 1 second for 1 token at 60/min)
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again after refill
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = clientIP
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Error("Should be allowed after token refill")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               10,
		CleanupInterval:         50 * time.Millisecond, // Very fast for testing
	}

	rl := NewRateLimiter(limits, createTestRateLimitLogger())
	defer rl.Stop()

	// Make requests from different IPs to create buckets
	for i := 0; i < 5; i++ {
		ip := fmt.Sprintf("192.168.1.%d:12345", 100+i)
		result := rl.checkRateLimit(ip, 100)
		if !result.Allowed {
			t.Errorf("Request from IP %s should be allowed", ip)
		}
	}

	// Verify buckets exist
	bucketCount := 0
	rl.ipBuckets.Range(func(key, value interface{}) bool {
		bucketCount++
		return true
	})

	if bucketCount != 5 {
		t.Errorf("Expected 5 IP buckets, got %d", bucketCount)
	}

	// Wait for cleanup (buckets should be removed after 10 minutes of inactivity)
	// We'll manipulate the last access time to simulate old buckets
	rl.ipBuckets.Range(func(key, value interface{}) bool {
		bucket := value.(*ipBucket)
		// Set last access to 11 minutes ago
		oldTime := time.Now().Add(-11 * time.Minute).UnixNano()
		atomic.StoreInt64(&bucket.lastAccess, oldTime)
		return true
	})

	// Wait for cleanup cycle
	time.Sleep(100 * time.Millisecond)

	// Buckets should be cleaned up
	bucketCountAfter := 0
	rl.ipBuckets.Range(func(key, value interface{}) bool {
		bucketCountAfter++
		return true
	})

	if bucketCountAfter != 0 {
		t.Errorf("Expected 0 IP buckets after cleanup, got %d", bucketCountAfter)
	}
}
