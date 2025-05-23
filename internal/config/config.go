package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Proxy: ProxyConfig{
			ConnectionTimeout: 30 * time.Second,  // Quick connection/request timeout
			ResponseTimeout:   10 * time.Minute,  // Long response timeout for LLMs
			ReadTimeout:       120 * time.Second, // 2 minutes between response chunks
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
			LoadBalancer:      "priority",
		},
		Discovery: DiscoveryConfig{
			Type:            "static",
			RefreshInterval: 30 * time.Second,
			Static: StaticDiscoveryConfig{
				Endpoints: []EndpointConfig{
					{
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "http://localhost:11434/health",
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
	}
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	config := DefaultConfig()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Environment variables
	viper.SetEnvPrefix("OLLA")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		// It's okay if config file doesn't exist
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// If config file not found, check if we have OLLA_CONFIG_FILE env var
		if configFile := os.Getenv("OLLA_CONFIG_FILE"); configFile != "" {
			viper.SetConfigFile(configFile)
			if err := viper.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("error reading config file %s: %w", configFile, err)
			}
		}
	}

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	// Enable hot reloading of config
	viper.WatchConfig()

	return config, nil
}
