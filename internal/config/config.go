package config

import (
	"fmt"
	"os"
	"strconv"
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
			WriteTimeout:    0, // No timeout for proxies
			ShutdownTimeout: 10 * time.Second,
		},
		Proxy: ProxyConfig{
			ConnectionTimeout: 30 * time.Second,
			ResponseTimeout:   10 * time.Minute,
			ReadTimeout:       120 * time.Second,
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
			LoadBalancer:      "priority",
			StreamBufferSize:  8 * 1024, // 8KB
		},
		Discovery: DiscoveryConfig{
			Type:            "static",
			RefreshInterval: 30 * time.Second,
			Static: StaticDiscoveryConfig{
				Endpoints: []EndpointConfig{
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
		Engineering: EngineeringConfig{
			ShowNerdStats: false,
		},
	}
}

func Load() (*Config, error) {
	config := DefaultConfig()

	// Simple config file loading - check a few standard locations
	configPaths := []string{"config.yaml", "config/config.yaml", "default.yaml"}
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

	// Apply essential environment overrides only
	applyEnvOverrides(config)
	return config, nil
}

// Simplified env overrides - only the ones actually used
func applyEnvOverrides(config *Config) {
	// Server config
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

	// Proxy config
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
	if val := os.Getenv("OLLA_PROXY_LOAD_BALANCER"); val != "" {
		config.Proxy.LoadBalancer = val
	}

	// Logging config
	if val := os.Getenv("OLLA_LOGGING_LEVEL"); val != "" {
		config.Logging.Level = val
	}
	if val := os.Getenv("OLLA_LOGGING_FORMAT"); val != "" {
		config.Logging.Format = val
	}

	// Engineering config
	if val := os.Getenv("OLLA_SHOW_NERD_STATS"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Engineering.ShowNerdStats = enabled
		}
	}
}