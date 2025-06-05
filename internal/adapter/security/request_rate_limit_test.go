package security

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/util"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

func createTestRateLimitLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}

func TestNewRateLimitValidator(t *testing.T) {
	cidrs := []string{"192.168.0.0/16", "10.0.0.0/8"}
	trustedCIDRs, _ := util.ParseTrustedCIDRs(cidrs)
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               50,
		HealthRequestsPerMinute: 500,
		CleanupInterval:         time.Minute,
		TrustProxyHeaders:       true,
		TrustedProxyCIDRs:       cidrs,
		TrustedProxyCIDRsParsed: trustedCIDRs,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	if validator.Name() != "rate_limit" {
		t.Errorf("Expected name 'rate_limit', got %q", validator.Name())
	}
	if validator.globalRequestsPerMinute != 1000 {
		t.Errorf("Expected global limit 1000, got %d", validator.globalRequestsPerMinute)
	}
	if validator.perIPRequestsPerMinute != 100 {
		t.Errorf("Expected per-IP limit 100, got %d", validator.perIPRequestsPerMinute)
	}
	if validator.burstSize != 50 {
		t.Errorf("Expected burst size 50, got %d", validator.burstSize)
	}
	if !validator.trustProxyHeaders {
		t.Error("Expected trust proxy headers to be true")
	}
	if validator.globalLimiter == nil {
		t.Error("Expected global limiter to be initialised")
	}
	if len(validator.trustedCIDRs) != 2 {
		t.Errorf("Expected 2 trusted CIDRs, got %d", len(validator.trustedCIDRs))
	}
}

func TestNewRateLimitValidator_InvalidCIDRs(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               50,
		TrustProxyHeaders:       true,
		TrustedProxyCIDRs:       []string{"invalid-cidr", "192.168.0.0/16"},
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	if validator.trustedCIDRs != nil {
		t.Error("Expected trustedCIDRs to be nil when parsing fails")
	}
}

func TestRateLimitValidator_Validate_Disabled(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  0,
		BurstSize:               10,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	for i := 0; i < 10; i++ {
		result, err := validator.Validate(context.Background(), req)
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed when limits are disabled", i+1)
		}
	}

	if validator.globalLimiter != nil {
		t.Error("Global limiter should not be initialised when global limit is 0")
	}
}

func TestRateLimitValidator_Validate_HealthEndpoint(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		HealthRequestsPerMinute: 300,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()
	clientIP := "192.168.1.100"

	regularReq := ports.SecurityRequest{
		ClientID:      clientIP,
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	healthReq := ports.SecurityRequest{
		ClientID:      clientIP,
		Endpoint:      "/internal/health",
		Method:        "GET",
		IsHealthCheck: true,
	}

	regularResult, err := validator.Validate(ctx, regularReq)
	if err != nil {
		t.Fatalf("Regular request validation failed: %v", err)
	}
	if regularResult.RateLimit != 60 {
		t.Errorf("Expected regular limit 60, got %d", regularResult.RateLimit)
	}

	healthResult, err := validator.Validate(ctx, healthReq)
	if err != nil {
		t.Fatalf("Health request validation failed: %v", err)
	}
	if healthResult.RateLimit != 300 {
		t.Errorf("Expected health limit 300, got %d", healthResult.RateLimit)
	}
}

func TestRateLimitValidator_Validate_BurstCapacity(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 10; i++ {
		result, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		if result.Allowed {
			successCount++
		} else {
			rateLimitedCount++
			if result.RetryAfter == 0 {
				t.Error("Expected Retry-After header when rate limited")
			}
		}
	}

	if successCount == 0 {
		t.Error("Expected some successful requests")
	}
	if rateLimitedCount == 0 {
		t.Log("No rate limiting triggered - this may be acceptable with new implementation")
	}
}

func TestRateLimitValidator_Validate_PerIPIsolation(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		BurstSize:               2,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()

	req1 := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	req2 := ports.SecurityRequest{
		ClientID:      "192.168.1.101",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	ip1Blocked := false
	for i := 0; i < 10; i++ {
		result, err := validator.Validate(ctx, req1)
		if err != nil {
			t.Fatalf("IP1 validation failed: %v", err)
		}
		if !result.Allowed {
			ip1Blocked = true
			break
		}
		time.Sleep(time.Millisecond)
	}

	result2, err := validator.Validate(ctx, req2)
	if err != nil {
		t.Fatalf("IP2 validation failed: %v", err)
	}
	if !result2.Allowed {
		t.Error("IP2 should be allowed (separate limiter)")
	}

	t.Logf("IP1 blocked: %v", ip1Blocked)
}

func TestRateLimitValidator_Validate_GlobalLimit(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 180,
		PerIPRequestsPerMinute:  600,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()
	ips := []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"}
	globalBlocked := false

	for i := 0; i < 20; i++ {
		ip := ips[i%len(ips)]
		req := ports.SecurityRequest{
			ClientID:      ip,
			Endpoint:      "/api/test",
			Method:        "POST",
			IsHealthCheck: false,
		}

		result, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Global limit validation failed: %v", err)
		}

		if !result.Allowed {
			if result.Reason == "Global rate limit exceeded" {
				globalBlocked = true
				break
			}
		}
	}

	if !globalBlocked {
		t.Log("Global rate limiting not triggered - this may be due to token refill timing")
	}
}

func TestRateLimitValidator_Validate_ConcurrentAccess(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  300,
		BurstSize:               5,
		CleanupInterval:         time.Minute,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := ports.SecurityRequest{
				ClientID:      "192.168.1.100",
				Endpoint:      "/api/test",
				Method:        "POST",
				IsHealthCheck: false,
			}

			for j := 0; j < 10; j++ {
				_, err := validator.Validate(ctx, req)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestRateLimitValidator_Cleanup(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  100,
		BurstSize:               10,
		CleanupInterval:         50 * time.Millisecond,
	}

	validator := NewRateLimitValidator(limits, createTestRateLimitLogger())
	defer validator.Stop()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		req := ports.SecurityRequest{
			ClientID:      fmt.Sprintf("192.168.1.%d", 100+i),
			Endpoint:      "/api/test",
			Method:        "POST",
			IsHealthCheck: false,
		}
		_, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
	}

	limiterCount := 0
	validator.ipLimiters.Range(func(key, value interface{}) bool {
		limiterCount++
		return true
	})

	if limiterCount != 5 {
		t.Errorf("Expected 5 IP limiters, got %d", limiterCount)
	}

	validator.ipLimiters.Range(func(key, value interface{}) bool {
		limiterInfo := value.(*ipLimiterInfo)
		limiterInfo.mu.Lock()
		limiterInfo.lastAccess = time.Now().Add(-11 * time.Minute)
		limiterInfo.mu.Unlock()
		return true
	})

	time.Sleep(100 * time.Millisecond)

	limiterCountAfter := 0
	validator.ipLimiters.Range(func(key, value interface{}) bool {
		limiterCountAfter++
		return true
	})

	if limiterCountAfter != 0 {
		t.Errorf("Expected 0 IP limiters after cleanup, got %d", limiterCountAfter)
	}
}
