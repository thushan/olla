package config

import (
	"fmt"
	"net"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Filename      string              `yaml:"-"`
	Logging       LoggingConfig       `yaml:"logging"`
	ModelRegistry ModelRegistryConfig `yaml:"model_registry"`
	Proxy         ProxyConfig         `yaml:"proxy"`
	Discovery     DiscoveryConfig     `yaml:"discovery"`
	Server        ServerConfig        `yaml:"server"`
	Engineering   EngineeringConfig   `yaml:"engineering"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string              `yaml:"host"`
	RateLimits      ServerRateLimits    `yaml:"rate_limits"`
	RequestLimits   ServerRequestLimits `yaml:"request_limits"`
	Port            int                 `yaml:"port"`
	ReadTimeout     time.Duration       `yaml:"read_timeout"`
	WriteTimeout    time.Duration       `yaml:"write_timeout"`
	IdleTimeout     time.Duration       `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration       `yaml:"shutdown_timeout"`
	RequestLogging  bool                `yaml:"request_logging"`
}

// GetAddress returns the server address in host:port format
func (s *ServerConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// ServerRequestLimits defines request size and validation limits
type ServerRequestLimits struct {
	MaxBodySize   int64 `yaml:"max_body_size"`
	MaxHeaderSize int64 `yaml:"max_header_size"`
}

// ServerRateLimits defines rate limiting configuration
type ServerRateLimits struct {
	TrustedProxyCIDRs       []string      `yaml:"trusted_proxy_cidrs"`
	TrustedProxyCIDRsParsed []*net.IPNet  // to avoid parsing every time :D
	GlobalRequestsPerMinute int           `yaml:"global_requests_per_minute"`
	PerIPRequestsPerMinute  int           `yaml:"per_ip_requests_per_minute"`
	BurstSize               int           `yaml:"burst_size"`
	HealthRequestsPerMinute int           `yaml:"health_requests_per_minute"`
	CleanupInterval         time.Duration `yaml:"cleanup_interval"`
	TrustProxyHeaders       bool          `yaml:"trust_proxy_headers"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	Engine            string        `yaml:"engine"`
	LoadBalancer      string        `yaml:"load_balancer"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	ResponseTimeout   time.Duration `yaml:"response_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	MaxRetries        int           `yaml:"max_retries"`
	RetryBackoff      time.Duration `yaml:"retry_backoff"`
	StreamBufferSize  int           `yaml:"stream_buffer_size"`
}

// DiscoveryConfig holds service discovery configuration
type DiscoveryConfig struct {
	Type            string                `yaml:"type"` // Only "static" is implemented
	Static          StaticDiscoveryConfig `yaml:"static"`
	RefreshInterval time.Duration         `yaml:"refresh_interval"`
	ModelDiscovery  ModelDiscoveryConfig  `yaml:"model_discovery"`
}

// ModelDiscoveryConfig holds model discvery specific settings
type ModelDiscoveryConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Interval          time.Duration `yaml:"interval"`
	Timeout           time.Duration `yaml:"timeout"`
	ConcurrentWorkers int           `yaml:"concurrent_workers"`
	RetryAttempts     int           `yaml:"retry_attempts"`
	RetryBackoff      time.Duration `yaml:"retry_backoff"`
}

// StaticDiscoveryConfig holds static endpoint configuration
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// EndpointConfig holds configuration for an AI inference endpoint
type EndpointConfig struct {
	URL            string        `yaml:"url"`
	Name           string        `yaml:"name"`
	Type           string        `yaml:"type"`
	HealthCheckURL string        `yaml:"health_check_url"`
	ModelURL       string        `yaml:"model_url"`
	Priority       int           `yaml:"priority"`
	CheckInterval  time.Duration `yaml:"check_interval"`
	CheckTimeout   time.Duration `yaml:"check_timeout"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// EngineeringConfig holds development/debugging configuration
type EngineeringConfig struct {
	ShowNerdStats bool `yaml:"show_nerdstats"`
}

// ModelRegistryConfig holds model registry configuration
type ModelRegistryConfig struct {
	Type          string            `yaml:"type"`
	Unification   UnificationConfig `yaml:"unification"`
	EnableUnifier bool              `yaml:"enable_unifier"`
}

// UnificationConfig holds model unification configuration
type UnificationConfig struct {
	CustomRules     []UnificationRuleConfig `yaml:"custom_rules"`
	CacheTTL        time.Duration           `yaml:"cache_ttl"`
	Enabled         bool                    `yaml:"enabled"`
	StaleThreshold  time.Duration           `yaml:"stale_threshold"`
	CleanupInterval time.Duration           `yaml:"cleanup_interval"`
}

// UnificationRuleConfig defines custom unification rules
type UnificationRuleConfig struct {
	FamilyOverrides map[string]string `yaml:"family_overrides"`
	NamePatterns    map[string]string `yaml:"name_patterns"`
	Platform        string            `yaml:"platform"`
}
