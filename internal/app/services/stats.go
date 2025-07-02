package services

import (
	"context"

	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// StatsService provides centralised metrics collection for all system components.
// It initialises early in the startup sequence as most other services depend on it
// for instrumentation and observability.
type StatsService struct {
	collector ports.StatsCollector
	logger    logger.StyledLogger
}

// NewStatsService creates a new stats service
func NewStatsService(logger logger.StyledLogger) *StatsService {
	return &StatsService{
		logger: logger,
	}
}

// Name returns the service name
func (s *StatsService) Name() string {
	return "stats"
}

// Start initialises the stats collector
func (s *StatsService) Start(ctx context.Context) error {
	s.logger.Info("Initialising stats collector")

	s.collector = stats.NewCollector(s.logger)

	s.logger.Info("Stats collector initialised")
	return nil
}

// Stop gracefully shuts down the stats collector
func (s *StatsService) Stop(ctx context.Context) error {
	// Lock-free atomic implementation requires no explicit cleanup
	s.logger.Info(" Stats collector stopped")
	return nil
}

// Dependencies returns service dependencies
func (s *StatsService) Dependencies() []string {
	return []string{}
}

// GetCollector returns the underlying stats collector
func (s *StatsService) GetCollector() ports.StatsCollector {
	if s.collector == nil {
		panic("stats collector not initialised")
	}
	return s.collector
}
