package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort = 19841
	DefaultHost = "localhost"
)

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            DefaultHost,
			Port:            DefaultPort,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    0, /* We don't want to timeout for proxies */
			ShutdownTimeout: 10 * time.Second,
		},
		Proxy: ProxyConfig{
			ConnectionTimeout: 30 * time.Second,
			ResponseTimeout:   10 * time.Minute,
			ReadTimeout:       120 * time.Second,
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
			LoadBalancer:      "priority",
			StreamBufferSize:     8 * 1024, // 8KB
			EnableCircuitBreaker: true,
		},
		Discovery: DiscoveryConfig{
			Type:            "static",
			RefreshInterval: 30 * time.Second,
			Static: StaticDiscoveryConfig{
				Endpoints: []EndpointConfig{
					/** Assume user has a default endpoint for Ollama API **/
					{
						Name:           "localhost",
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "/health",
						ModelURL:       "/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				},
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Telemetry: TelemetryConfig{
			Metrics: MetricsConfig{
				Enabled: true,
				Address: ":9090",
			},
			Tracing: TracingConfig{
				Enabled:    false,
				Endpoint:   "localhost:4317",
				SampleRate: 0.1,
			},
		},
		Security: SecurityConfig{
			TLS: TLSConfig{
				Enabled:  false,
				CertFile: "cert.pem",
				KeyFile:  "key.pem",
			},
			MTLS: MTLSConfig{
				Enabled: false,
				CAFile:  "ca.pem",
			},
		},
		Plugins: PluginsConfig{
			Directory: "./plugins",
			Enabled:   []string{},
			Config:    map[string]interface{}{},
		},
		Engineering: EngineeringConfig{
			ShowNerdStats: false,
		},
	}
}

func Load() (*Config, error) {
	config := DefaultConfig()

	// Prioritise users ./config.yaml or ./config/config.yaml before falling back to default.yaml
	configPaths := []string{"config.yaml", "config/config.yaml" /* Development */, "default.yaml"}
	if configFile := os.Getenv("OLLA_CONFIG_FILE"); configFile != "" {
		configPaths = []string{configFile}
	}

	var configLoaded bool
	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", path, err)
			}
			configLoaded = true
			break
		}
	}

	if !configLoaded && os.Getenv("OLLA_CONFIG_FILE") != "" {
		return nil, fmt.Errorf("specified config file not found: %s", os.Getenv("OLLA_CONFIG_FILE"))
	}

	applyEnvOverrides(config)
	return config, nil
}

func applyEnvOverrides(config *Config) {
	if val := os.Getenv("OLLA_SERVER_HOST"); val != "" {
		config.Server.Host = val
	}
	if val := os.Getenv("OLLA_SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Server.Port = port
		}
	}
	if val := os.Getenv("OLLA_SERVER_READ_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Server.ReadTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_SERVER_WRITE_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Server.WriteTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_SERVER_SHUTDOWN_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Server.ShutdownTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_PROXY_CONNECTION_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Proxy.ConnectionTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_PROXY_RESPONSE_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Proxy.ResponseTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_PROXY_READ_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Proxy.ReadTimeout = duration
		}
	}
	if val := os.Getenv("OLLA_PROXY_MAX_RETRIES"); val != "" {
		if retries, err := strconv.Atoi(val); err == nil {
			config.Proxy.MaxRetries = retries
		}
	}
	if val := os.Getenv("OLLA_PROXY_RETRY_BACKOFF"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Proxy.RetryBackoff = duration
		}
	}
	if val := os.Getenv("OLLA_PROXY_LOAD_BALANCER"); val != "" {
		config.Proxy.LoadBalancer = val
	}
	if val := os.Getenv("OLLA_DISCOVERY_TYPE"); val != "" {
		config.Discovery.Type = val
	}
	if val := os.Getenv("OLLA_DISCOVERY_REFRESH_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Discovery.RefreshInterval = duration
		}
	}
	if val := os.Getenv("OLLA_LOGGING_LEVEL"); val != "" {
		config.Logging.Level = val
	}
	if val := os.Getenv("OLLA_LOGGING_FORMAT"); val != "" {
		config.Logging.Format = val
	}
	if val := os.Getenv("OLLA_LOGGING_OUTPUT"); val != "" {
		config.Logging.Output = val
	}
	if val := os.Getenv("OLLA_TELEMETRY_METRICS_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Telemetry.Metrics.Enabled = enabled
		}
	}
	if val := os.Getenv("OLLA_TELEMETRY_METRICS_ADDRESS"); val != "" {
		config.Telemetry.Metrics.Address = val
	}
	if val := os.Getenv("OLLA_TELEMETRY_TRACING_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Telemetry.Tracing.Enabled = enabled
		}
	}
	if val := os.Getenv("OLLA_TELEMETRY_TRACING_ENDPOINT"); val != "" {
		config.Telemetry.Tracing.Endpoint = val
	}
	if val := os.Getenv("OLLA_TELEMETRY_TRACING_SAMPLE_RATE"); val != "" {
		if rate, err := strconv.ParseFloat(val, 64); err == nil {
			config.Telemetry.Tracing.SampleRate = rate
		}
	}
	if val := os.Getenv("OLLA_SECURITY_TLS_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Security.TLS.Enabled = enabled
		}
	}
	if val := os.Getenv("OLLA_SECURITY_TLS_CERT_FILE"); val != "" {
		config.Security.TLS.CertFile = val
	}
	if val := os.Getenv("OLLA_SECURITY_TLS_KEY_FILE"); val != "" {
		config.Security.TLS.KeyFile = val
	}
	if val := os.Getenv("OLLA_SECURITY_MTLS_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Security.MTLS.Enabled = enabled
		}
	}
	if val := os.Getenv("OLLA_SECURITY_MTLS_CA_FILE"); val != "" {
		config.Security.MTLS.CAFile = val
	}
	if val := os.Getenv("OLLA_PLUGINS_DIRECTORY"); val != "" {
		config.Plugins.Directory = val
	}
	if val := os.Getenv("OLLA_PLUGINS_ENABLED"); val != "" {
		config.Plugins.Enabled = strings.Split(val, ",")
	}
}
