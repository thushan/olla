package services

import (
	"context"
	"fmt"

	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// SecurityService provides request validation, rate limiting, and security policy
// enforcement. It creates a chain of inspectors that validate incoming requests
// before they reach the proxy layer.
type SecurityService struct {
	serverConfig   *config.ServerConfig
	statsCollector ports.StatsCollector
	statsService   *StatsService
	logger         logger.StyledLogger
	services       *security.Services
	adapters       *security.Adapters
}

// NewSecurityService creates a new security service
func NewSecurityService(serverConfig *config.ServerConfig, statsCollector ports.StatsCollector, logger logger.StyledLogger) *SecurityService {
	return &SecurityService{
		serverConfig:   serverConfig,
		statsCollector: statsCollector,
		logger:         logger,
	}
}

// Name returns the service name
func (s *SecurityService) Name() string {
	return "security"
}

// Start initialises security components
func (s *SecurityService) Start(ctx context.Context) error {
	s.logger.Info("Initialising security service")

	// Factory requires full config structure for historical reasons
	tempConfig := &config.Config{
		Server: *s.serverConfig,
	}

	if s.statsService != nil {
		collector, err := s.statsService.GetCollector()
		if err != nil {
			return fmt.Errorf("failed to get stats collector: %w", err)
		}
		s.statsCollector = collector
	}

	s.services, s.adapters = security.NewSecurityServices(tempConfig, s.statsCollector, s.logger)

	s.logger.Info("Security services initialised",
		"globalRateLimit", s.serverConfig.RateLimits.GlobalRequestsPerMinute,
		"perIPRateLimit", s.serverConfig.RateLimits.PerIPRequestsPerMinute,
		"maxBodySize", s.serverConfig.RequestLimits.MaxBodySize)

	return nil
}

// Stop gracefully shuts down security components
func (s *SecurityService) Stop(ctx context.Context) error {
	s.logger.Info(" Stopping security service")

	defer func() {
		s.logger.ResetLine()
		s.logger.InfoWithStatus("Stopping security service", "OK")
	}()

	if s.adapters != nil {
		s.adapters.Stop()
	}

	return nil
}

// Dependencies returns service dependencies
func (s *SecurityService) Dependencies() []string {
	return []string{"stats"}
}

// GetSecurityChain returns the security chain for middleware
func (s *SecurityService) GetSecurityChain() (*ports.SecurityChain, error) {
	if s.services == nil || s.services.Chain == nil {
		return nil, fmt.Errorf("security chain not initialised")
	}
	return s.services.Chain, nil
}

// GetAdapters returns the security adapters
func (s *SecurityService) GetAdapters() (*security.Adapters, error) {
	if s.adapters == nil {
		return nil, fmt.Errorf("security adapters not initialised")
	}
	return s.adapters, nil
}

// GetMetrics returns the security metrics service
func (s *SecurityService) GetMetrics() (ports.SecurityMetricsService, error) {
	if s.services == nil || s.services.Metrics == nil {
		return nil, fmt.Errorf("security metrics not initialised")
	}
	return s.services.Metrics, nil
}

// SetStatsService sets the stats service dependency
func (s *SecurityService) SetStatsService(statsService *StatsService) {
	s.statsService = statsService
}
