package config

import "time"

// Config holds all configuration for the application
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Proxy       ProxyConfig       `yaml:"proxy"`
	Discovery   DiscoveryConfig   `yaml:"discovery"`
	Logging     LoggingConfig     `yaml:"logging"`
	Engineering EngineeringConfig `yaml:"engineering"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	ResponseTimeout   time.Duration `yaml:"response_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	MaxRetries        int           `yaml:"max_retries"`
	RetryBackoff      time.Duration `yaml:"retry_backoff"`
	LoadBalancer      string        `yaml:"load_balancer"`
	StreamBufferSize  int           `yaml:"stream_buffer_size"`
}

// DiscoveryConfig holds service discovery configuration
type DiscoveryConfig struct {
	Type            string                `yaml:"type"` // Only "static" is implemented
	RefreshInterval time.Duration         `yaml:"refresh_interval"`
	Static          StaticDiscoveryConfig `yaml:"static"`
}

// StaticDiscoveryConfig holds static endpoint configuration
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// EndpointConfig holds configuration for an Ollama endpoint
type EndpointConfig struct {
	Name           string        `yaml:"name"`
	URL            string        `yaml:"url"`
	Priority       int           `yaml:"priority"`
	HealthCheckURL string        `yaml:"health_check_url"`
	ModelURL       string        `yaml:"model_url"`
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