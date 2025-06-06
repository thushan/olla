package ports

import (
	"context"
	"time"
)

type SecurityRequest struct {
	Headers       map[string][]string
	ClientID      string
	Endpoint      string
	Method        string
	BodySize      int64
	HeaderSize    int64
	IsHealthCheck bool
}

type SecurityResult struct {
	ResetTime  time.Time
	Reason     string
	RetryAfter int
	RateLimit  int
	Remaining  int
	Allowed    bool
}

type SecurityViolation struct {
	Timestamp     time.Time
	ClientID      string
	ViolationType string
	Endpoint      string
	Size          int64
}

type SecurityMetrics struct {
	RateLimitViolations  int64
	SizeLimitViolations  int64
	UniqueRateLimitedIPs int
}

type SecurityValidator interface {
	Validate(ctx context.Context, req SecurityRequest) (SecurityResult, error)
	Name() string
}

type SecurityChain struct {
	validators []SecurityValidator
}

func NewSecurityChain(validators ...SecurityValidator) *SecurityChain {
	return &SecurityChain{
		validators: validators,
	}
}

func (sc *SecurityChain) Validate(ctx context.Context, req SecurityRequest) (SecurityResult, error) {
	for _, validator := range sc.validators {
		if result, err := validator.Validate(ctx, req); err != nil {
			return result, err
		} else if !result.Allowed {
			return result, nil
		}
	}
	return SecurityResult{Allowed: true}, nil
}

func (sc *SecurityChain) GetValidators() []SecurityValidator {
	return sc.validators
}

type SecurityMetricsService interface {
	RecordViolation(ctx context.Context, violation SecurityViolation) error
	GetMetrics(ctx context.Context) (SecurityMetrics, error)
}
