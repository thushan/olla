package unifier

import (
	"time"
)

type Config struct {
	ModelTTL                     time.Duration        `json:"model_ttl" yaml:"model_ttl"`
	CleanupInterval              time.Duration        `json:"cleanup_interval" yaml:"cleanup_interval"`
	DiscoveryRetryPolicy         RetryPolicy          `json:"discovery_retry_policy" yaml:"discovery_retry_policy"`
	CircuitBreaker               CircuitBreakerConfig `json:"circuit_breaker" yaml:"circuit_breaker"`
	EnableBackgroundCleanup      bool                 `json:"enable_background_cleanup" yaml:"enable_background_cleanup"`
	EndpointHealthCheckInterval  time.Duration        `json:"endpoint_health_check_interval" yaml:"endpoint_health_check_interval"`
	MaxConsecutiveFailures       int                  `json:"max_consecutive_failures" yaml:"max_consecutive_failures"`
	EnableStateTransitionLogging bool                 `json:"enable_state_transition_logging" yaml:"enable_state_transition_logging"`
}

type RetryPolicy struct {
	MaxAttempts       int           `json:"max_attempts" yaml:"max_attempts"`
	InitialBackoff    time.Duration `json:"initial_backoff" yaml:"initial_backoff"`
	MaxBackoff        time.Duration `json:"max_backoff" yaml:"max_backoff"`
	BackoffMultiplier float64       `json:"backoff_multiplier" yaml:"backoff_multiplier"`
}

type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled" yaml:"enabled"`
	FailureThreshold int           `json:"failure_threshold" yaml:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold" yaml:"success_threshold"`
	OpenDuration     time.Duration `json:"open_duration" yaml:"open_duration"`
	HalfOpenRequests int           `json:"half_open_requests" yaml:"half_open_requests"`
}

// DefaultConfig provides production-ready defaults tuned for typical LLM workloads
func DefaultConfig() Config {
	return Config{
		ModelTTL:                24 * time.Hour,
		CleanupInterval:         5 * time.Minute,
		EnableBackgroundCleanup: true,
		EndpointHealthCheckInterval: 30 * time.Second,
		MaxConsecutiveFailures:  3,
		EnableStateTransitionLogging: false,
		DiscoveryRetryPolicy: RetryPolicy{
			MaxAttempts:       3,
			InitialBackoff:    time.Second,
			MaxBackoff:        30 * time.Second,
			BackoffMultiplier: 2.0,
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 2,
			OpenDuration:     60 * time.Second,
			HalfOpenRequests: 3,
		},
	}
}

// Validate fixes invalid values rather than returning errors for operational stability
func (c *Config) Validate() error {
	if c.ModelTTL <= 0 {
		c.ModelTTL = 24 * time.Hour
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = 5 * time.Minute
	}
	if c.EndpointHealthCheckInterval <= 0 {
		c.EndpointHealthCheckInterval = 30 * time.Second
	}
	if c.MaxConsecutiveFailures <= 0 {
		c.MaxConsecutiveFailures = 3
	}
	if c.DiscoveryRetryPolicy.MaxAttempts <= 0 {
		c.DiscoveryRetryPolicy.MaxAttempts = 3
	}
	if c.DiscoveryRetryPolicy.InitialBackoff <= 0 {
		c.DiscoveryRetryPolicy.InitialBackoff = time.Second
	}
	if c.DiscoveryRetryPolicy.BackoffMultiplier <= 1 {
		c.DiscoveryRetryPolicy.BackoffMultiplier = 2.0
	}
	return nil
}