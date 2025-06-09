package security

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/constants"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

func createTestMetricsLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}
func createNewTestSecurityMetricsAdapter() *MetricsAdapter {
	kennyloggins := createTestMetricsLogger()
	statsCollector := createTestStatsCollector(kennyloggins)
	return NewSecurityMetricsAdapter(statsCollector, kennyloggins)
}
func TestNewSecurityMetricsAdapter(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()

	if adapter == nil {
		t.Fatal("NewSecurityMetricsAdapter returned nil")
	}
	if adapter.statsCollector == nil {
		t.Error("statsCollector not set")
	}
	if adapter.logger == nil {
		t.Error("Logger not set")
	}
}

func TestSecurityMetricsAdapter_RecordViltion_RateLimit(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	violation := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: "rate_limit",
		Endpoint:      "/api/test",
		Size:          0,
		Timestamp:     time.Now(),
	}

	err := adapter.RecordViolation(ctx, violation)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 1 {
		t.Errorf("Expected 1 rate limit violation, got %d", metrics.RateLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 1 {
		t.Errorf("Expected 1 unique rate limited IP, got %d", metrics.UniqueRateLimitedIPs)
	}
	if metrics.SizeLimitViolations != 0 {
		t.Errorf("Expected 0 size limit violations, got %d", metrics.SizeLimitViolations)
	}
}

func TestSecurityMetricsAdapter_RecordViolation_LargeRequest(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	violation := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: "size_limit",
		Endpoint:      "/api/test",
		Size:          100 * 1024 * 1024,
		Timestamp:     time.Now(),
	}

	err := adapter.RecordViolation(ctx, violation)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.SizeLimitViolations != 1 {
		t.Errorf("Expected 1 size limit violation, got %d", metrics.SizeLimitViolations)
	}
}

func TestSecurityMetricsAdapter_RecordViolation_MultipleIPs(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	ips := []string{"192.168.1.100", "192.168.1.101", "192.168.1.102", "192.168.1.103"}

	for _, ip := range ips {
		violation := ports.SecurityViolation{
			ClientID:      ip,
			ViolationType: "rate_limit",
			Endpoint:      "/api/test",
			Size:          0,
			Timestamp:     time.Now(),
		}

		err := adapter.RecordViolation(ctx, violation)
		if err != nil {
			t.Fatalf("RecordViolation failed for IP %s: %v", ip, err)
		}
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 4 {
		t.Errorf("Expected 4 rate limit violations, got %d", metrics.RateLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 4 {
		t.Errorf("Expected 4 unique rate limited IPs, got %d", metrics.UniqueRateLimitedIPs)
	}
}

func TestSecurityMetricsAdapter_RecordViolation_DuplicateIP(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	ip := "192.168.1.100"

	for i := 0; i < 3; i++ {
		violation := ports.SecurityViolation{
			ClientID:      ip,
			ViolationType: "rate_limit",
			Endpoint:      "/api/test",
			Size:          0,
			Timestamp:     time.Now(),
		}

		err := adapter.RecordViolation(ctx, violation)
		if err != nil {
			t.Fatalf("RecordViolation failed for iteration %d: %v", i, err)
		}
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 3 {
		t.Errorf("Expected 3 rate limit violations, got %d", metrics.RateLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 1 {
		t.Errorf("Expected 1 unique rate limited IP, got %d", metrics.UniqueRateLimitedIPs)
	}
}

func TestSecurityMetricsAdapter_RecordViolation_UnknownType(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	violation := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: "unknown_type",
		Endpoint:      "/api/test",
		Size:          0,
		Timestamp:     time.Now(),
	}

	err := adapter.RecordViolation(ctx, violation)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 0 {
		t.Errorf("Expected 0 rate limit violations for unknown type, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 0 {
		t.Errorf("Expected 0 size limit violations for unknown type, got %d", metrics.SizeLimitViolations)
	}
}

func TestSecurityMetricsAdapter_ConcurrentAccess(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 5; j++ {
				violation := ports.SecurityViolation{
					ClientID:      fmt.Sprintf("192.168.1.%d", 100+id),
					ViolationType: "rate_limit",
					Endpoint:      "/api/test",
					Size:          0,
					Timestamp:     time.Now(),
				}

				err := adapter.RecordViolation(ctx, violation)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 3; j++ {
				violation := ports.SecurityViolation{
					ClientID:      fmt.Sprintf("192.168.2.%d", 100+id),
					ViolationType: "size_limit",
					Endpoint:      "/api/test",
					Size:          1024 * 1024,
					Timestamp:     time.Now(),
				}

				err := adapter.RecordViolation(ctx, violation)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				_, err := adapter.GetMetrics(ctx)
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("Final GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 100 {
		t.Errorf("Expected 100 rate limit violations, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 30 {
		t.Errorf("Expected 30 size limit violations, got %d", metrics.SizeLimitViolations)
	}
}

/*
	func TestSecurityMetricsAdapter_RecordViolation_SizeLimitA(t *testing.T) {
		mockStats := ports.NewMockStatsCollector()
		adapter := NewSecurityMetricsAdapter(mockStats, createTestMetricsLogger())
		ctx := context.Background()

		violation := ports.SecurityViolation{
			ClientID:      "192.168.1.100",
			ViolationType: "size_limit",
			Endpoint:      "/api/test",
			Size:          1024 * 1024,
			Timestamp:     time.Now(),
		}

		err := adapter.RecordViolation(ctx, violation)
		if err != nil {
			t.Fatalf("RecordViolation failed: %v", err)
		}

		metrics, err := adapter.GetMetrics(ctx)
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}

		if metrics.SizeLimitViolations != 1 {
			t.Errorf("Expected 1 size limit violation, got %d", metrics.SizeLimitViolations)
		}
		if metrics.RateLimitViolations != 0 {
			t.Errorf("Expected 0 rate limit violations, got %d", metrics.RateLimitViolations)
		}
		if metrics.UniqueRateLimitedIPs != 0 {
			t.Errorf("Expected 0 unique rate limited IPs, got %d", metrics.UniqueRateLimitedIPs)
		}
	}
*/
func TestSecurityMetricsAdapter_RecordViolation_RateLimit(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	violation := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: constants.ViolationRateLimit,
		Endpoint:      "/api/test",
		Size:          0,
		Timestamp:     time.Now(),
	}

	err := adapter.RecordViolation(ctx, violation)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	// Verify the violation was recorded in stats collector
	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 1 {
		t.Errorf("Expected 1 rate limit violation, got %d", metrics.RateLimitViolations)
	}
}

func TestSecurityMetricsAdapter_RecordViolation_SizeLimit(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	violation := ports.SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: constants.ViolationSizeLimit,
		Endpoint:      "/api/test",
		Size:          100 * 1024 * 1024, // 100MB
		Timestamp:     time.Now(),
	}

	err := adapter.RecordViolation(ctx, violation)
	if err != nil {
		t.Fatalf("RecordViolation failed: %v", err)
	}

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.SizeLimitViolations != 1 {
		t.Errorf("Expected 1 size limit violation, got %d", metrics.SizeLimitViolations)
	}
}

func TestSecurityMetricsAdapter_GetMetrics(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	// Test initial state
	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 0 {
		t.Errorf("Expected 0 initial rate limit violations, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 0 {
		t.Errorf("Expected 0 initial size limit violations, got %d", metrics.SizeLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 0 {
		t.Errorf("Expected 0 initial unique IPs, got %d", metrics.UniqueRateLimitedIPs)
	}
}
func TestSecurityMetricsAdapter_GetMetrics_Empty(t *testing.T) {
	adapter := createNewTestSecurityMetricsAdapter()
	ctx := context.Background()

	metrics, err := adapter.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.RateLimitViolations != 0 {
		t.Errorf("Expected 0 rate limit violations, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 0 {
		t.Errorf("Expected 0 size limit violations, got %d", metrics.SizeLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 0 {
		t.Errorf("Expected 0 unique rate limited IPs, got %d", metrics.UniqueRateLimitedIPs)
	}
}
