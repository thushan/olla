package config

import "time"

// Config holds all configuration for the application
type Config struct {
	Server    ServerConfig    `mapstructure:"server" yaml:"server"`
	Proxy     ProxyConfig     `mapstructure:"proxy" yaml:"proxy"`
	Discovery DiscoveryConfig `mapstructure:"discovery" yaml:"discovery"`
	Logging   LoggingConfig   `mapstructure:"logging" yaml:"logging"`
	Telemetry TelemetryConfig `mapstructure:"telemetry" yaml:"telemetry"`
	Security  SecurityConfig  `mapstructure:"security" yaml:"security"`
	Plugins   PluginsConfig   `mapstructure:"plugins" yaml:"plugins"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `mapstructure:"host" yaml:"host"`
	Port            int           `mapstructure:"port" yaml:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout" yaml:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" yaml:"shutdown_timeout"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	ConnectionTimeout    time.Duration `mapstructure:"connection_timeout" yaml:"connection_timeout"` // Time to establish connection and send request
	ResponseTimeout      time.Duration `mapstructure:"response_timeout" yaml:"response_timeout"`     // Time to wait for response (much longer for LLMs)
	ReadTimeout          time.Duration `mapstructure:"read_timeout" yaml:"read_timeout"`             // Max time between response chunks
	MaxRetries           int           `mapstructure:"max_retries" yaml:"max_retries"`
	RetryBackoff         time.Duration `mapstructure:"retry_backoff" yaml:"retry_backoff"`
	LoadBalancer         string        `mapstructure:"load_balancer" yaml:"load_balancer"`
	StreamBufferSize     int           `mapstructure:"stream_buffer_size" yaml:"stream_buffer_size"`
	EnableCircuitBreaker bool          `mapstructure:"enable_circuit_breaker" yaml:"enable_circuit_breaker"`
}

// DiscoveryConfig holds service discovery configuration
type DiscoveryConfig struct {
	Type            string                `mapstructure:"type" yaml:"type"` // static, consul, etcd
	RefreshInterval time.Duration         `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	Static          StaticDiscoveryConfig `mapstructure:"static" yaml:"static"`
	Consul          ConsulDiscoveryConfig `mapstructure:"consul" yaml:"consul"`
	Etcd            EtcdDiscoveryConfig   `mapstructure:"etcd" yaml:"etcd"`
}

// StaticDiscoveryConfig holds static endpoint configuration
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `mapstructure:"endpoints" yaml:"endpoints"`
}

// EndpointConfig holds configuration for an Ollama endpoint
type EndpointConfig struct {
	Name           string        `mapstructure:"name" yaml:"name"`
	URL            string        `mapstructure:"url" yaml:"url"`
	Priority       int           `mapstructure:"priority" yaml:"priority"`
	HealthCheckURL string        `mapstructure:"health_check_url" yaml:"health_check_url"`
	ModelURL       string        `mapstructure:"model_url" yaml:"model_url"`
	CheckInterval  time.Duration `mapstructure:"check_interval" yaml:"check_interval"`
	CheckTimeout   time.Duration `mapstructure:"check_timeout" yaml:"check_timeout"`
}

// ConsulDiscoveryConfig holds configuration for Consul service discovery
type ConsulDiscoveryConfig struct {
	Address     string `mapstructure:"address" yaml:"address"`
	ServiceName string `mapstructure:"service_name" yaml:"service_name"`
}

// EtcdDiscoveryConfig holds configuration for etcd service discovery
type EtcdDiscoveryConfig struct {
	Endpoints []string `mapstructure:"endpoints" yaml:"endpoints"`
	KeyPrefix string   `mapstructure:"key_prefix" yaml:"key_prefix"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
	Output string `mapstructure:"output" yaml:"output"`
}

// TelemetryConfig holds telemetry configuration
type TelemetryConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics" yaml:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing" yaml:"tracing"`
}

// MetricsConfig holds metrics exporter configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Address string `mapstructure:"address" yaml:"address"`
}

// TracingConfig holds tracing exporter configuration
type TracingConfig struct {
	Enabled    bool    `mapstructure:"enabled" yaml:"enabled"`
	Endpoint   string  `mapstructure:"endpoint" yaml:"endpoint"`
	SampleRate float64 `mapstructure:"sample_rate" yaml:"sample_rate"`
}

// SecurityConfig holds TLS/mTLS configuration
type SecurityConfig struct {
	TLS  TLSConfig  `mapstructure:"tls" yaml:"tls"`
	MTLS MTLSConfig `mapstructure:"mtls" yaml:"mtls"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	CertFile string `mapstructure:"cert_file" yaml:"cert_file"`
	KeyFile  string `mapstructure:"key_file" yaml:"key_file"`
}

// MTLSConfig holds mutual TLS configuration
type MTLSConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	CAFile  string `mapstructure:"ca_file" yaml:"ca_file"`
}

// PluginsConfig holds plugin configuration
type PluginsConfig struct {
	Directory string                 `mapstructure:"directory" yaml:"directory"`
	Enabled   []string               `mapstructure:"enabled" yaml:"enabled"`
	Config    map[string]interface{} `mapstructure:"config" yaml:"config"`
}
