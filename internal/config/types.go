package config

import "time"

// Config holds all configuration for the application
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Proxy     ProxyConfig     `mapstructure:"proxy"`
	Discovery DiscoveryConfig `mapstructure:"discovery"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Security  SecurityConfig  `mapstructure:"security"`
	Plugins   PluginsConfig   `mapstructure:"plugins"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout"` // Time to establish connection and send request
	ResponseTimeout   time.Duration `mapstructure:"response_timeout"`   // Time to wait for response (much longer for LLMs)
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`       // Max time between response chunks
	MaxRetries        int           `mapstructure:"max_retries"`
	RetryBackoff      time.Duration `mapstructure:"retry_backoff"`
	LoadBalancer      string        `mapstructure:"load_balancer"`
}

// DiscoveryConfig holds service discovery configuration
type DiscoveryConfig struct {
	Type            string                `mapstructure:"type"` // static, consul, etcd
	RefreshInterval time.Duration         `mapstructure:"refresh_interval"`
	Static          StaticDiscoveryConfig `mapstructure:"static"`
	Consul          ConsulDiscoveryConfig `mapstructure:"consul"`
	Etcd            EtcdDiscoveryConfig   `mapstructure:"etcd"`
}

// StaticDiscoveryConfig holds configuration for static service discovery
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `mapstructure:"endpoints"`
}

// EndpointConfig holds configuration for an Ollama endpoint
type EndpointConfig struct {
	Name           string        `mapstructure:"name"`
	URL            string        `mapstructure:"url"`
	Priority       int           `mapstructure:"priority"`
	HealthCheckURL string        `mapstructure:"health_check_url"`
	ModelURL       string        `mapstructure:"model_url"`
	CheckInterval  time.Duration `mapstructure:"check_interval"`
	CheckTimeout   time.Duration `mapstructure:"check_timeout"`
}

// ConsulDiscoveryConfig holds configuration for Consul service discovery
type ConsulDiscoveryConfig struct {
	Address     string `mapstructure:"address"`
	ServiceName string `mapstructure:"service_name"`
}

// EtcdDiscoveryConfig holds configuration for etcd service discovery
type EtcdDiscoveryConfig struct {
	Endpoints []string `mapstructure:"endpoints"`
	KeyPrefix string   `mapstructure:"key_prefix"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// TelemetryConfig holds telemetry configuration
type TelemetryConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Address string `mapstructure:"address"`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled    bool    `mapstructure:"enabled"`
	Endpoint   string  `mapstructure:"endpoint"`
	SampleRate float64 `mapstructure:"sample_rate"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	TLS  TLSConfig  `mapstructure:"tls"`
	MTLS MTLSConfig `mapstructure:"mtls"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// MTLSConfig holds mutual TLS configuration
type MTLSConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	CAFile  string `mapstructure:"ca_file"`
}

// PluginsConfig holds plugin configuration
type PluginsConfig struct {
	Directory string                 `mapstructure:"directory"`
	Enabled   []string               `mapstructure:"enabled"`
	Config    map[string]interface{} `mapstructure:"config"`
}
