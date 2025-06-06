package security

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	ViolationRateLimit = "rate_limit"
	ViolationSizeLimit = "size_limit"
)

type MetricsAdapter struct {
	uniqueRateLimitedIPs map[string]time.Time
	logger               *logger.StyledLogger
	rateLimitViolations  int64
	sizeLimitViolations  int64
	mu                   sync.RWMutex
}

// NewSecurityMetricsAdapter concise way to capture security metrics for now
func NewSecurityMetricsAdapter(logger *logger.StyledLogger) *MetricsAdapter {
	return &MetricsAdapter{
		uniqueRateLimitedIPs: make(map[string]time.Time),
		logger:               logger,
	}
}

func (sma *MetricsAdapter) RecordViolation(ctx context.Context, violation ports.SecurityViolation) error {
	switch violation.ViolationType {
	case ViolationRateLimit:
		atomic.AddInt64(&sma.rateLimitViolations, 1)
		sma.recordRateLimitedIP(violation.ClientID)
	case ViolationSizeLimit:
		atomic.AddInt64(&sma.sizeLimitViolations, 1)
		if violation.Size > 50*1024*1024 {
			sma.logger.Warn("Large request blocked",
				"client_id", violation.ClientID,
				"size", violation.Size,
				"endpoint", violation.Endpoint)
		}
	}

	return nil
}

func (sma *MetricsAdapter) GetMetrics(ctx context.Context) (ports.SecurityMetrics, error) {
	sma.mu.RLock()
	uniqueRateLimitedCount := len(sma.uniqueRateLimitedIPs)
	sma.mu.RUnlock()

	return ports.SecurityMetrics{
		RateLimitViolations:  atomic.LoadInt64(&sma.rateLimitViolations),
		SizeLimitViolations:  atomic.LoadInt64(&sma.sizeLimitViolations),
		UniqueRateLimitedIPs: uniqueRateLimitedCount,
	}, nil
}

func (sma *MetricsAdapter) recordRateLimitedIP(clientIP string) {
	sma.mu.Lock()
	defer sma.mu.Unlock()

	sma.uniqueRateLimitedIPs[clientIP] = time.Now()

	if len(sma.uniqueRateLimitedIPs) > 5 {
		sma.logger.Warn("Multiple IPs rate limited",
			"unique_ips", len(sma.uniqueRateLimitedIPs),
			"latest_ip", clientIP)
	}

	cutoff := time.Now().Add(-time.Hour)
	for ip, timestamp := range sma.uniqueRateLimitedIPs {
		if timestamp.Before(cutoff) {
			delete(sma.uniqueRateLimitedIPs, ip)
		}
	}
}
