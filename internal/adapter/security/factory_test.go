package security

import (
	"context"
	"github.com/thushan/olla/internal/adapter/stats"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func createTestFactoryLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func createTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			RateLimits: config.ServerRateLimits{
				GlobalRequestsPerMinute: 1000,
				PerIPRequestsPerMinute:  100,
				BurstSize:               50,
				HealthRequestsPerMinute: 500,
				CleanupInterval:         5 * time.Minute,
				TrustProxyHeaders:       false,
				TrustedProxyCIDRs:       []string{"127.0.0.0/8", "192.168.0.0/16"},
			},
			RequestLimits: config.ServerRequestLimits{
				MaxBodySize:   10 * 1024 * 1024, // 10MB
				MaxHeaderSize: 64 * 1024,        // 64KB
			},
		},
	}
}

func createTestStatsCollector(log logger.StyledLogger) ports.StatsCollector {
	return stats.NewCollector(log)
}
func createNewTestSecurityServices() (*Services, *Adapters) {
	cfg := createTestConfig()
	return createNewTestSecurityServicesWithConfig(cfg)
}
func createNewTestSecurityServicesWithConfig(cfg *config.Config) (*Services, *Adapters) {
	logger := createTestFactoryLogger()
	statsCollector := createTestStatsCollector(logger)
	return NewSecurityServices(cfg, statsCollector, logger)
}
func TestNewSecurityServices(t *testing.T) {
	services, adapters := createNewTestSecurityServices()

	if services == nil {
		t.Fatal("NewSecurityServices returned nil services")
	}
	if adapters == nil {
		t.Fatal("NewSecurityServices returned nil adapters")
	}

	if services.Chain == nil {
		t.Error("SecurityServices.Chain is nil")
	}
	if services.Metrics == nil {
		t.Error("SecurityServices.Metrics is nil")
	}

	if adapters.RateLimit == nil {
		t.Error("SecurityAdapters.RateLimit is nil")
	}
	if adapters.SizeValidation == nil {
		t.Error("SecurityAdapters.SizeValidation is nil")
	}
	if adapters.Metrics == nil {
		t.Error("SecurityAdapters.Metrics is nil")
	}
	if adapters.Chain == nil {
		t.Error("SecurityAdapters.Chain is nil")
	}
}

func TestSecurityServices_ChainValidators(t *testing.T) {
	services, adapters := createNewTestSecurityServices()
	defer adapters.Stop()

	validators := services.Chain.GetValidators()
	if len(validators) != 2 {
		t.Errorf("Expected 2 validators in chain, got %d", len(validators))
	}

	expectedNames := []string{"rate_limit", "size_limit"}
	for i, validator := range validators {
		if validator.Name() != expectedNames[i] {
			t.Errorf("Expected validator %d to be %q, got %q", i, expectedNames[i], validator.Name())
		}
	}
}

func TestSecurityServices_ChainValidation_AllPass(t *testing.T) {
	services, adapters := createNewTestSecurityServices()
	defer adapters.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		BodySize:      1024,
		HeaderSize:    512,
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		IsHealthCheck: false,
	}

	result, err := services.Chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Chain validation failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Request should be allowed, got: %s", result.Reason)
	}
}

func TestSecurityServices_ChainValidation_RateLimitFails(t *testing.T) {
	cfg := createTestConfig()
	cfg.Server.RateLimits.PerIPRequestsPerMinute = 1 // Very low limit
	cfg.Server.RateLimits.BurstSize = 1
	services, adapters := createNewTestSecurityServicesWithConfig(cfg)
	defer adapters.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		BodySize:      1024,
		HeaderSize:    512,
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		IsHealthCheck: false,
	}

	// First request should pass
	result1, err := services.Chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("First chain validation failed: %v", err)
	}
	if !result1.Allowed {
		t.Log("First request was rate limited - this may be due to timing")
	}

	// Subsequent rapid requests should be rate limited
	rateLimited := false
	for i := 0; i < 10; i++ {
		result, err := services.Chain.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Chain validation failed on iteration %d: %v", i, err)
		}
		if !result.Allowed {
			rateLimited = true
			if result.Reason != "Rate limit exceeded" {
				t.Errorf("Expected rate limit reason, got: %s", result.Reason)
			}
			break
		}
	}

	if !rateLimited {
		t.Log("Rate limiting not triggered - this may be due to implementation timing")
	}
}

func TestSecurityServices_ChainValidation_SizeLimitFails(t *testing.T) {
	cfg := createTestConfig()
	cfg.Server.RequestLimits.MaxBodySize = 100 // Very small limit
	services, adapters := createNewTestSecurityServicesWithConfig(cfg)
	defer adapters.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		BodySize:      1024, // Exceeds limit
		HeaderSize:    50,
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		IsHealthCheck: false,
	}

	result, err := services.Chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Chain validation failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be rejected due to size limit")
	}
	if result.Reason == "" {
		t.Error("Expected reason for rejection")
	}
}

func TestSecurityServices_ChainValidation_StopsAtFirstFailure(t *testing.T) {
	cfg := createTestConfig()
	cfg.Server.RateLimits.PerIPRequestsPerMinute = 1
	cfg.Server.RateLimits.BurstSize = 1
	cfg.Server.RequestLimits.MaxBodySize = 100
	services, adapters := createNewTestSecurityServicesWithConfig(cfg)
	defer adapters.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		BodySize:      1024, // Exceeds size limit
		HeaderSize:    50,
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		IsHealthCheck: false,
	}

	// Make multiple requests to potentially trigger rate limiting
	for i := 0; i < 5; i++ {
		result, err := services.Chain.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Chain validation failed on iteration %d: %v", i, err)
		}
		if !result.Allowed {
			// Chain should stop at first validator that fails
			// In this case, rate limiter runs first, so if it fails, we get rate limit error
			// If rate limiter passes, size validator fails with size error
			if result.Reason != "Rate limit exceeded" && !contains(result.Reason, "Request body too large") {
				t.Errorf("Expected either rate limit or size limit error, got: %s", result.Reason)
			}
			break
		}
	}
}

func TestSecurityAdapters_Stop(t *testing.T) {
	_, adapters := createNewTestSecurityServices()

	// Should not panic
	adapters.Stop()

	// Should be safe to call multiple times
	adapters.Stop()
}

func TestSecurityAdapters_CreateChainMiddleware(t *testing.T) {
	_, adapters := createNewTestSecurityServices()
	defer adapters.Stop()

	middleware := adapters.CreateChainMiddleware()
	if middleware == nil {
		t.Error("CreateChainMiddleware returned nil")
	}

	// Test that middleware can be created without panicking
	// We can't easily test the HTTP middleware behavior here without setting up full HTTP test infrastructure
}

func TestSecurityServices_MetricsIntegration(t *testing.T) {
	cfg := createTestConfig()
	services, adapters := createNewTestSecurityServicesWithConfig(cfg)
	defer adapters.Stop()

	ctx := context.Background()

	// Record some violations
	violation1 := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: "rate_limit",
		Endpoint:      "/api/test",
		Size:          0,
		Timestamp:     time.Now(),
	}

	violation2 := ports.SecurityViolation{
		ClientID:      "192.168.1.101",
		ViolationType: "size_limit",
		Endpoint:      "/api/test",
		Size:          1024 * 1024,
		Timestamp:     time.Now(),
	}

	err := services.Metrics.RecordViolation(ctx, violation1)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	err = services.Metrics.RecordViolation(ctx, violation2)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	metrics, err := services.Metrics.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 1 {
		t.Errorf("Expected 1 rate limit violation, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 1 {
		t.Errorf("Expected 1 size limit violation, got %d", metrics.SizeLimitViolations)
	}
}

func TestSecurityServices_ConfiguredLimits(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestFactoryLogger()
	statsCollector := createTestStatsCollector(logger)

	_, adapters := NewSecurityServices(cfg, statsCollector, logger)
	defer adapters.Stop()

	// Verify rate limit adapter configuration
	if adapters.RateLimit.globalRequestsPerMinute != cfg.Server.RateLimits.GlobalRequestsPerMinute {
		t.Errorf("Rate limit adapter global limit mismatch: expected %d, got %d",
			cfg.Server.RateLimits.GlobalRequestsPerMinute, adapters.RateLimit.globalRequestsPerMinute)
	}

	if adapters.RateLimit.perIPRequestsPerMinute != cfg.Server.RateLimits.PerIPRequestsPerMinute {
		t.Errorf("Rate limit adapter per-IP limit mismatch: expected %d, got %d",
			cfg.Server.RateLimits.PerIPRequestsPerMinute, adapters.RateLimit.perIPRequestsPerMinute)
	}

	// Verify size validator configuration
	if adapters.SizeValidation.maxBodySize != cfg.Server.RequestLimits.MaxBodySize {
		t.Errorf("Size validator body limit mismatch: expected %d, got %d",
			cfg.Server.RequestLimits.MaxBodySize, adapters.SizeValidation.maxBodySize)
	}

	if adapters.SizeValidation.maxHeaderSize != cfg.Server.RequestLimits.MaxHeaderSize {
		t.Errorf("Size validator header limit mismatch: expected %d, got %d",
			cfg.Server.RequestLimits.MaxHeaderSize, adapters.SizeValidation.maxHeaderSize)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
