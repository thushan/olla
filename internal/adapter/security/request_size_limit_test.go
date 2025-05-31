package security

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

func createTestSizeLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}

func TestNewSizeValidator(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 512,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())

	if validator.Name() != "size_limit" {
		t.Errorf("Expected name 'size_limit', got %q", validator.Name())
	}
	if validator.maxBodySize != 1024 {
		t.Errorf("Expected max body size 1024, got %d", validator.maxBodySize)
	}
	if validator.maxHeaderSize != 512 {
		t.Errorf("Expected max header size 512, got %d", validator.maxHeaderSize)
	}
}

func TestSizeValidator_Validate_SmallRequest(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 512,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   100,
		HeaderSize: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"User-Agent":   {"TestAgent/1.0"},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Small request should be allowed, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_BodyTooLarge(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 1024,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   200,
		HeaderSize: 50,
		Headers:    map[string][]string{},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.Allowed {
		t.Error("Large body should be rejected")
	}
	if !strings.Contains(result.Reason, "Request body too large") {
		t.Errorf("Expected body size error, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_HeadersTooLarge(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 100,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   50,
		HeaderSize: 200,
		Headers: map[string][]string{
			"X-Large-Header": {strings.Repeat("x", 200)},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.Allowed {
		t.Error("Large headers should be rejected")
	}
	if !strings.Contains(result.Reason, "Request headers too large") {
		t.Errorf("Expected header size error, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_ZeroLimitsDisabled(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   0,
		MaxHeaderSize: 0,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   10000,
		HeaderSize: 5000,
		Headers: map[string][]string{
			"X-Huge-Header": {strings.Repeat("z", 5000)},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Request should be allowed when limits are disabled, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_NegativeLimitsDisabled(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   -1,
		MaxHeaderSize: -100,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   5000,
		HeaderSize: 2000,
		Headers: map[string][]string{
			"X-Large-Header": {strings.Repeat("z", 2000)},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Request should be allowed when limits are negative (disabled), got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_ExactLimit(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 200,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   100,
		HeaderSize: 150,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Request at exact limit should be allowed, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_OneByteTooLarge(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 200,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   101,
		HeaderSize: 150,
		Headers:    map[string][]string{},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request one byte over limit should be rejected")
	}
}

func TestSizeValidator_Validate_EmptyBody(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 200,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "GET",
		BodySize:   0,
		HeaderSize: 50,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Empty body should be allowed, got: %s", result.Reason)
	}
}

func TestSizeValidator_Validate_MultipleHeaders(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 200,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   100,
		HeaderSize: 300,
		Headers: map[string][]string{
			"Content-Type":    {"application/json"},
			"Authorization":   {"Bearer " + strings.Repeat("x", 50)},
			"X-Custom-Header": {strings.Repeat("y", 50)},
			"User-Agent":      {"TestAgent/1.0"},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.Allowed {
		t.Error("Large multiple headers should be rejected")
	}
}

func TestSizeValidator_Validate_ConcurrentRequests(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1000,
		MaxHeaderSize: 500,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 20)
	results := make(chan bool, 20)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			var bodySize int64
			if id%2 == 0 {
				bodySize = 500
			} else {
				bodySize = 1500
			}

			req := ports.SecurityRequest{
				ClientID:   "192.168.1.100",
				Endpoint:   "/api/test",
				Method:     "POST",
				BodySize:   bodySize,
				HeaderSize: 100,
				Headers:    map[string][]string{"Content-Type": {"application/json"}},
			}

			result, err := validator.Validate(ctx, req)
			if err != nil {
				errors <- err
				return
			}
			results <- result.Allowed
		}(i)
	}

	wg.Wait()
	close(errors)
	close(results)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	allowedCount := 0
	rejectedCount := 0
	for allowed := range results {
		if allowed {
			allowedCount++
		} else {
			rejectedCount++
		}
	}

	if allowedCount != 5 {
		t.Errorf("Expected 5 allowed requests, got %d", allowedCount)
	}
	if rejectedCount != 5 {
		t.Errorf("Expected 5 rejected requests, got %d", rejectedCount)
	}
}

func TestSizeValidator_Validate_MultiValueHeaders(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 150,
	}

	validator := NewSizeValidator(limits, createTestSizeLogger())
	ctx := context.Background()

	req := ports.SecurityRequest{
		ClientID:   "192.168.1.100",
		Endpoint:   "/api/test",
		Method:     "POST",
		BodySize:   100,
		HeaderSize: 200,
		Headers: map[string][]string{
			"Accept": {"application/json", "text/html", "application/xml"},
		},
	}

	result, err := validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Multi-value headers should be calculated correctly, got: %s", result.Reason)
	}

	req.Headers["Large-Header"] = []string{strings.Repeat("z", 100)}
	req.HeaderSize = 350

	result, err = validator.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.Allowed {
		t.Error("Large multi-value headers should exceed limit")
	}
}