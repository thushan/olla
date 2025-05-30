package config

import "time"

// Config holds all configuration for the application
type Config struct {
	Logging     LoggingConfig     `yaml:"logging"`
	Discovery   DiscoveryConfig   `yaml:"discovery"`
	Server      ServerConfig      `yaml:"server"`
	Proxy       ProxyConfig       `yaml:"proxy"`
	Engineering EngineeringConfig `yaml:"engineering"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string              `yaml:"host"`
	Port            int                 `yaml:"port"`
	ReadTimeout     time.Duration       `yaml:"read_timeout"`
	WriteTimeout    time.Duration       `yaml:"write_timeout"`
	ShutdownTimeout time.Duration       `yaml:"shutdown_timeout"`
	RequestLimits   ServerRequestLimits `yaml:"request_limits"`
	RateLimits      ServerRateLimits    `yaml:"rate_limits"`
}

// ServerRequestLimits defines request size and validation limits
type ServerRequestLimits struct {
	MaxBodySize   int64 `yaml:"max_body_size"`
	MaxHeaderSize int64 `yaml:"max_header_size"`
}

// ServerRateLimits defines rate limiting configuration
type ServerRateLimits struct {
	GlobalRequestsPerMinute    int           `yaml:"global_requests_per_minute"`
	PerIPRequestsPerMinute     int           `yaml:"per_ip_requests_per_minute"`
	BurstSize                  int           `yaml:"burst_size"`
	HealthRequestsPerMinute    int           `yaml:"health_requests_per_minute"`
	CleanupInterval            time.Duration `yaml:"cleanup_interval"`
	IPExtractionTrustProxy     bool          `yaml:"ip_extraction_trust_proxy"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
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
}

// StaticDiscoveryConfig holds static endpoint configuration
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// EndpointConfig holds configuration for an Ollama endpoint
type EndpointConfig struct {
	Name           string        `yaml:"name"`
	URL            string        `yaml:"url"`
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
