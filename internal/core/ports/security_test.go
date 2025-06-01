package ports

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type mockSecurityValidator struct {
	name          string
	shouldAllow   bool
	errorToReturn error
	callCount     int
}

func (m *mockSecurityValidator) Name() string {
	return m.name
}

func (m *mockSecurityValidator) Validate(ctx context.Context, req SecurityRequest) (SecurityResult, error) {
	m.callCount++

	if m.errorToReturn != nil {
		return SecurityResult{}, m.errorToReturn
	}

	if !m.shouldAllow {
		return SecurityResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Mock validator %s rejected request", m.name),
		}, nil
	}

	return SecurityResult{
		Allowed: true,
	}, nil
}

func TestNewSecurityChain(t *testing.T) {
	validator1 := &mockSecurityValidator{name: "validator1", shouldAllow: true}
	validator2 := &mockSecurityValidator{name: "validator2", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2)

	if chain == nil {
		t.Fatal("NewSecurityChain returned nil")
	}

	validators := chain.GetValidators()
	if len(validators) != 2 {
		t.Errorf("Expected 2 validators, got %d", len(validators))
	}

	if validators[0].Name() != "validator1" {
		t.Errorf("Expected first validator name 'validator1', got %q", validators[0].Name())
	}

	if validators[1].Name() != "validator2" {
		t.Errorf("Expected second validator name 'validator2', got %q", validators[1].Name())
	}
}

func TestSecurityChain_Validate_AllAllow(t *testing.T) {
	validator1 := &mockSecurityValidator{name: "validator1", shouldAllow: true}
	validator2 := &mockSecurityValidator{name: "validator2", shouldAllow: true}
	validator3 := &mockSecurityValidator{name: "validator3", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2, validator3)
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Chain validation failed: %v", err)
	}

	if !result.Allowed {
		t.Errorf("Expected request to be allowed, got: %s", result.Reason)
	}

	if validator1.callCount != 1 {
		t.Errorf("Expected validator1 to be called once, got %d", validator1.callCount)
	}
	if validator2.callCount != 1 {
		t.Errorf("Expected validator2 to be called once, got %d", validator2.callCount)
	}
	if validator3.callCount != 1 {
		t.Errorf("Expected validator3 to be called once, got %d", validator3.callCount)
	}
}

func TestSecurityChain_Validate_FirstRejects(t *testing.T) {
	validator1 := &mockSecurityValidator{name: "validator1", shouldAllow: false}
	validator2 := &mockSecurityValidator{name: "validator2", shouldAllow: true}
	validator3 := &mockSecurityValidator{name: "validator3", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2, validator3)
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Chain validation failed: %v", err)
	}

	if result.Allowed {
		t.Error("Expected request to be rejected by first validator")
	}

	if result.Reason != "Mock validator validator1 rejected request" {
		t.Errorf("Expected specific rejection reason, got: %s", result.Reason)
	}

	if validator1.callCount != 1 {
		t.Errorf("Expected validator1 to be called once, got %d", validator1.callCount)
	}
	if validator2.callCount != 0 {
		t.Errorf("Expected validator2 to not be called, got %d calls", validator2.callCount)
	}
	if validator3.callCount != 0 {
		t.Errorf("Expected validator3 to not be called, got %d calls", validator3.callCount)
	}
}

func TestSecurityChain_Validate_MiddleRejects(t *testing.T) {
	validator1 := &mockSecurityValidator{name: "validator1", shouldAllow: true}
	validator2 := &mockSecurityValidator{name: "validator2", shouldAllow: false}
	validator3 := &mockSecurityValidator{name: "validator3", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2, validator3)
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Chain validation failed: %v", err)
	}

	if result.Allowed {
		t.Error("Expected request to be rejected by middle validator")
	}

	if result.Reason != "Mock validator validator2 rejected request" {
		t.Errorf("Expected specific rejection reason, got: %s", result.Reason)
	}

	if validator1.callCount != 1 {
		t.Errorf("Expected validator1 to be called once, got %d", validator1.callCount)
	}
	if validator2.callCount != 1 {
		t.Errorf("Expected validator2 to be called once, got %d", validator2.callCount)
	}
	if validator3.callCount != 0 {
		t.Errorf("Expected validator3 to not be called, got %d calls", validator3.callCount)
	}
}

func TestSecurityChain_Validate_ErrorHandling(t *testing.T) {
	expectedError := fmt.Errorf("validator error")
	validator1 := &mockSecurityValidator{name: "validator1", shouldAllow: true}
	validator2 := &mockSecurityValidator{name: "validator2", errorToReturn: expectedError}
	validator3 := &mockSecurityValidator{name: "validator3", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2, validator3)
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err == nil {
		t.Fatal("Expected error to be returned")
	}

	if err.Error() != "validator error" {
		t.Errorf("Expected specific error, got: %v", err)
	}

	if validator1.callCount != 1 {
		t.Errorf("Expected validator1 to be called once, got %d", validator1.callCount)
	}
	if validator2.callCount != 1 {
		t.Errorf("Expected validator2 to be called once, got %d", validator2.callCount)
	}
	if validator3.callCount != 0 {
		t.Errorf("Expected validator3 to not be called, got %d calls", validator3.callCount)
	}

	if result.Allowed {
		t.Error("Expected result.Allowed to be false on error")
	}
}

func TestSecurityChain_Validate_EmptyChain(t *testing.T) {
	chain := NewSecurityChain()
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Empty chain validation failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected empty chain to allow request")
	}
}

func TestSecurityChain_Validate_SingleValidator(t *testing.T) {
	validator := &mockSecurityValidator{name: "single", shouldAllow: true}
	chain := NewSecurityChain(validator)
	ctx := context.Background()

	req := SecurityRequest{
		ClientID: "192.168.1.100",
		Endpoint: "/api/test",
		Method:   "POST",
	}

	result, err := chain.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Single validator chain failed: %v", err)
	}

	if !result.Allowed {
		t.Errorf("Expected request to be allowed, got: %s", result.Reason)
	}

	if validator.callCount != 1 {
		t.Errorf("Expected validator to be called once, got %d", validator.callCount)
	}
}

func TestSecurityChain_GetValidators(t *testing.T) {
	validator1 := &mockSecurityValidator{name: "first", shouldAllow: true}
	validator2 := &mockSecurityValidator{name: "second", shouldAllow: true}
	validator3 := &mockSecurityValidator{name: "third", shouldAllow: true}

	chain := NewSecurityChain(validator1, validator2, validator3)
	validators := chain.GetValidators()

	if len(validators) != 3 {
		t.Errorf("Expected 3 validators, got %d", len(validators))
	}

	expectedNames := []string{"first", "second", "third"}
	for i, validator := range validators {
		if validator.Name() != expectedNames[i] {
			t.Errorf("Expected validator %d name %q, got %q", i, expectedNames[i], validator.Name())
		}
	}
}

func TestSecurityRequest_StructFields(t *testing.T) {
	req := SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		BodySize:      1024,
		HeaderSize:    512,
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		IsHealthCheck: false,
	}

	if req.ClientID != "192.168.1.100" {
		t.Errorf("Expected ClientID '192.168.1.100', got %q", req.ClientID)
	}
	if req.Endpoint != "/api/test" {
		t.Errorf("Expected Endpoint '/api/test', got %q", req.Endpoint)
	}
	if req.Method != "POST" {
		t.Errorf("Expected Method 'POST', got %q", req.Method)
	}
	if req.BodySize != 1024 {
		t.Errorf("Expected BodySize 1024, got %d", req.BodySize)
	}
	if req.HeaderSize != 512 {
		t.Errorf("Expected HeaderSize 512, got %d", req.HeaderSize)
	}
	if req.IsHealthCheck != false {
		t.Errorf("Expected IsHealthCheck false, got %v", req.IsHealthCheck)
	}
	if len(req.Headers) != 1 {
		t.Errorf("Expected 1 header, got %d", len(req.Headers))
	}
}

func TestSecurityResult_StructFields(t *testing.T) {
	resetTime := time.Now().Add(time.Minute)

	result := SecurityResult{
		Allowed:    false,
		Reason:     "Rate limit exceeded",
		RetryAfter: 60,
		RateLimit:  100,
		Remaining:  0,
		ResetTime:  resetTime,
	}

	if result.Allowed != false {
		t.Errorf("Expected Allowed false, got %v", result.Allowed)
	}
	if result.Reason != "Rate limit exceeded" {
		t.Errorf("Expected Reason 'Rate limit exceeded', got %q", result.Reason)
	}
	if result.RetryAfter != 60 {
		t.Errorf("Expected RetryAfter 60, got %d", result.RetryAfter)
	}
	if result.RateLimit != 100 {
		t.Errorf("Expected RateLimit 100, got %d", result.RateLimit)
	}
	if result.Remaining != 0 {
		t.Errorf("Expected Remaining 0, got %d", result.Remaining)
	}
	if !result.ResetTime.Equal(resetTime) {
		t.Errorf("Expected ResetTime %v, got %v", resetTime, result.ResetTime)
	}
}

func TestSecurityViolation_StructFields(t *testing.T) {
	timestamp := time.Now()

	violation := SecurityViolation{
		ClientID:      "192.168.1.100",
		ViolationType: "rate_limit",
		Endpoint:      "/api/test",
		Size:          1024,
		Timestamp:     timestamp,
	}

	if violation.ClientID != "192.168.1.100" {
		t.Errorf("Expected ClientID '192.168.1.100', got %q", violation.ClientID)
	}
	if violation.ViolationType != "rate_limit" {
		t.Errorf("Expected ViolationType 'rate_limit', got %q", violation.ViolationType)
	}
	if violation.Endpoint != "/api/test" {
		t.Errorf("Expected Endpoint '/api/test', got %q", violation.Endpoint)
	}
	if violation.Size != 1024 {
		t.Errorf("Expected Size 1024, got %d", violation.Size)
	}
	if !violation.Timestamp.Equal(timestamp) {
		t.Errorf("Expected Timestamp %v, got %v", timestamp, violation.Timestamp)
	}
}

func TestSecurityMetrics_StructFields(t *testing.T) {
	metrics := SecurityMetrics{
		RateLimitViolations:  10,
		SizeLimitViolations:  5,
		UniqueRateLimitedIPs: 3,
	}

	if metrics.RateLimitViolations != 10 {
		t.Errorf("Expected RateLimitViolations 10, got %d", metrics.RateLimitViolations)
	}
	if metrics.SizeLimitViolations != 5 {
		t.Errorf("Expected SizeLimitViolations 5, got %d", metrics.SizeLimitViolations)
	}
	if metrics.UniqueRateLimitedIPs != 3 {
		t.Errorf("Expected UniqueRateLimitedIPs 3, got %d", metrics.UniqueRateLimitedIPs)
	}
}
