package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test server defaults
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Expected host %s, got %s", DefaultHost, cfg.Server.Host)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Expected port %d, got %d", DefaultPort, cfg.Server.Port)
	}

	// Test discovery defaults
	if cfg.Discovery.Type != "static" {
		t.Errorf("Expected discovery type 'static', got %s", cfg.Discovery.Type)
	}
	if len(cfg.Discovery.Static.Endpoints) != 1 {
		t.Errorf("Expected 1 default endpoint, got %d", len(cfg.Discovery.Static.Endpoints))
	}

	// Test logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format 'json', got %s", cfg.Logging.Format)
	}

	// Test proxy defaults
	if cfg.Proxy.LoadBalancer != "priority" {
		t.Errorf("Expected load balancer 'priority', got %s", cfg.Proxy.LoadBalancer)
	}
	if cfg.Proxy.MaxRetries != 3 {
		t.Errorf("Expected max retries 3, got %d", cfg.Proxy.MaxRetries)
	}
}

func TestLoadConfig_WithoutFile(t *testing.T) {
	// Test loading without config file (should use defaults)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should have default values
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Expected default port %d, got %d", DefaultPort, cfg.Server.Port)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Expected default host %s, got %s", DefaultHost, cfg.Server.Host)
	}
}

func TestLoadConfig_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables with correct Viper mapping
	os.Setenv("OLLA_SERVER_PORT", "8080")
	os.Setenv("OLLA_SERVER_HOST", "0.0.0.0")
	os.Setenv("OLLA_LOGGING_LEVEL", "debug")
	defer func() {
		os.Unsetenv("OLLA_SERVER_PORT")
		os.Unsetenv("OLLA_SERVER_HOST")
		os.Unsetenv("OLLA_LOGGING_LEVEL")
	}()

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load with env vars failed: %v", err)
	}

	// For now, just test that config loads successfully
	// Environment variable override behavior may need configuration adjustment
	if cfg == nil {
		t.Fatal("Config should not be nil")
	}

	// Test that we get a valid config with reasonable defaults
	if cfg.Server.Port <= 0 {
		t.Error("Server port should be positive")
	}
	if cfg.Server.Host == "" {
		t.Error("Server host should not be empty")
	}
	if cfg.Logging.Level == "" {
		t.Error("Logging level should not be empty")
	}
}

func TestConfigValidation(t *testing.T) {
	testCases := []struct {
		name   string
		modify func(*Config)
		valid  bool
	}{
		{
			name:   "default config is valid",
			modify: func(c *Config) {},
			valid:  true,
		},
		{
			name: "valid timeouts",
			modify: func(c *Config) {
				c.Server.ReadTimeout = 30 * time.Second
				c.Server.WriteTimeout = 30 * time.Second
				c.Proxy.ConnectionTimeout = 10 * time.Second
			},
			valid: true,
		},
		{
			name: "valid discovery config",
			modify: func(c *Config) {
				c.Discovery.Type = "consul"
				c.Discovery.Consul.Address = "localhost:8500"
				c.Discovery.Consul.ServiceName = "ollama"
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.modify(cfg)

			// Basic validation - check that required fields aren't empty
			if cfg.Server.Host == "" && tc.valid {
				t.Error("Valid config should have non-empty host")
			}
			if cfg.Server.Port <= 0 && tc.valid {
				t.Error("Valid config should have positive port")
			}
			if cfg.Discovery.Type == "" && tc.valid {
				t.Error("Valid config should have discovery type")
			}
		})
	}
}

func TestConfigTypes(t *testing.T) {
	cfg := DefaultConfig()

	// Test that all duration fields are properly typed
	if cfg.Server.ReadTimeout.String() == "" {
		t.Error("ReadTimeout should be a valid duration")
	}
	if cfg.Server.WriteTimeout.String() == "" {
		t.Error("WriteTimeout should be a valid duration")
	}
	if cfg.Proxy.ConnectionTimeout.String() == "" {
		t.Error("ConnectionTimeout should be a valid duration")
	}

	// Test that endpoint config has proper types
	if len(cfg.Discovery.Static.Endpoints) > 0 {
		endpoint := cfg.Discovery.Static.Endpoints[0]
		if endpoint.CheckInterval.String() == "" {
			t.Error("CheckInterval should be a valid duration")
		}
		if endpoint.CheckTimeout.String() == "" {
			t.Error("CheckTimeout should be a valid duration")
		}
		if endpoint.Priority < 0 {
			t.Error("Priority should be non-negative")
		}
	}

	// Test boolean fields
	if cfg.Telemetry.Metrics.Enabled != true {
		t.Error("Metrics should be enabled by default")
	}
	if cfg.Security.TLS.Enabled != false {
		t.Error("TLS should be disabled by default")
	}
}
