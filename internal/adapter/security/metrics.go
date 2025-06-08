package security

import (
	"context"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

type MetricsAdapter struct {
	statsCollector ports.StatsCollector
	logger         *logger.StyledLogger
}

// NewSecurityMetricsAdapter concise way to capture security metrics for now
func NewSecurityMetricsAdapter(statsCollector ports.StatsCollector, logger *logger.StyledLogger) *MetricsAdapter {
	return &MetricsAdapter{
		statsCollector: statsCollector,
		logger:         logger,
	}
}

func (sma *MetricsAdapter) RecordViolation(ctx context.Context, violation ports.SecurityViolation) error {
	sma.statsCollector.RecordSecurityViolation(violation)

	if violation.ViolationType == constants.ViolationSizeLimit && violation.Size > 50*1024*1024 {
		sma.logger.Warn("Large request blocked",
			"client_id", violation.ClientID,
			"size", violation.Size,
			"endpoint", violation.Endpoint)
	}

	return nil
}

func (sma *MetricsAdapter) GetMetrics(ctx context.Context) (ports.SecurityMetrics, error) {
	stats := sma.statsCollector.GetSecurityStats()

	return ports.SecurityMetrics{
		RateLimitViolations:  stats.RateLimitViolations,
		SizeLimitViolations:  stats.SizeLimitViolations,
		UniqueRateLimitedIPs: stats.UniqueRateLimitedIPs,
	}, nil
}
