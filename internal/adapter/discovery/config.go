package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/config"
	"net/url"
)

func validateEndpointConfig(cfg config.EndpointConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	if cfg.HealthCheckURL == "" {
		return fmt.Errorf("health check URL cannot be empty")
	}

	if cfg.ModelURL == "" {
		return fmt.Errorf("model URL cannot be empty")
	}

	if cfg.CheckInterval < MinHealthCheckInterval {
		return fmt.Errorf("check_interval too short: minimum %v, got %v", MinHealthCheckInterval, cfg.CheckInterval)
	}

	if cfg.CheckTimeout >= cfg.CheckInterval {
		return fmt.Errorf("check_timeout (%v) must be less than check_interval (%v)", cfg.CheckTimeout, cfg.CheckInterval)
	}

	if cfg.CheckTimeout > MaxHealthCheckTimeout {
		return fmt.Errorf("check_timeout too long: maximum %v, got %v", MaxHealthCheckTimeout, cfg.CheckTimeout)
	}

	if cfg.Priority < 0 {
		return fmt.Errorf("priority must be non-negative, got %d", cfg.Priority)
	}

	// Validate URLs are parseable, we need to ensure they are valid URLs
	// now so we can merge later without worrying about URL parsing errors
	if _, err := url.Parse(cfg.URL); err != nil {
		return fmt.Errorf("invalid endpoint URL %q: %w", cfg.URL, err)
	}

	if _, err := url.Parse(cfg.HealthCheckURL); err != nil {
		return fmt.Errorf("invalid health check URL %q: %w", cfg.HealthCheckURL, err)
	}

	if _, err := url.Parse(cfg.ModelURL); err != nil {
		return fmt.Errorf("invalid model URL %q: %w", cfg.ModelURL, err)
	}
	return nil
}

func (s *StaticDiscoveryService) SetConfig(config *config.Config) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.config = config
}

func (s *StaticDiscoveryService) getConfig() *config.Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}

func (s *StaticDiscoveryService) ReloadConfig() {
	go func() {
		s.logger.Info("Config file changed, reloading endpoints...")
		if err := s.RefreshEndpoints(context.Background()); err != nil {
			s.logger.Error("Failed to reload endpoints from config", "error", err)
		}
	}()
}
