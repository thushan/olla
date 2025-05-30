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

	// Test engineering defaults
	if cfg.Engineering.ShowNerdStats != false {
		t.Error("Expected ShowNerdStats to be false by default")
	}
}

func TestLoadConfig_WithoutFile(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != DefaultPort {
		t.Errorf("Expected default port %d, got %d", DefaultPort, cfg.Server.Port)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Expected default host %s, got %s", DefaultHost, cfg.Server.Host)
	}
}

func TestLoadConfig_WithEnvironmentVariables(t *testing.T) {
	// Set test environment variables
	testEnvVars := map[string]string{
		"OLLA_SERVER_PORT":             "8080",
		"OLLA_SERVER_HOST":             "0.0.0.0",
		"OLLA_PROXY_LOAD_BALANCER":     "round-robin",
		"OLLA_LOGGING_LEVEL":           "debug",
		"OLLA_SHOW_NERD_STATS":         "true",
		"OLLA_PROXY_RESPONSE_TIMEOUT":  "15m",
	}

	// Set env vars
	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}

	// Clean up after test
	defer func() {
		for key := range testEnvVars {
			os.Unsetenv(key)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with env vars failed: %v", err)
	}

	// Verify env var overrides
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080 from env var, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0 from env var, got %s", cfg.Server.Host)
	}
	if cfg.Proxy.LoadBalancer != "round-robin" {
		t.Errorf("Expected load balancer round-robin from env var, got %s", cfg.Proxy.LoadBalancer)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug from env var, got %s", cfg.Logging.Level)
	}
	if cfg.Engineering.ShowNerdStats != true {
		t.Error("Expected ShowNerdStats true from env var")
	}
	if cfg.Proxy.ResponseTimeout != 15*time.Minute {
		t.Errorf("Expected response timeout 15m from env var, got %v", cfg.Proxy.ResponseTimeout)
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
			name: "valid static discovery config",
			modify: func(c *Config) {
				c.Discovery.Type = "static"
				c.Discovery.Static.Endpoints = []EndpointConfig{
					{
						Name:           "test",
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "/health",
						ModelURL:       "/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				}
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.modify(cfg)

			// Basic validation
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

	// Test that duration fields are properly typed
	if cfg.Server.ReadTimeout.String() == "" {
		t.Error("ReadTimeout should be a valid duration")
	}
	if cfg.Server.WriteTimeout.String() == "" {
		t.Error("WriteTimeout should be a valid duration")
	}
	if cfg.Proxy.ConnectionTimeout.String() == "" {
		t.Error("ConnectionTimeout should be a valid duration")
	}

	// Test endpoint config types
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
	if cfg.Engineering.ShowNerdStats != false {
		t.Error("ShowNerdStats should be disabled by default")
	}
}

func TestEnvironmentVariableParsing(t *testing.T) {
	testCases := []struct {
		envVar   string
		envValue string
		checkFn  func(*Config) bool
	}{
		{
			"OLLA_SERVER_PORT",
			"9999",
			func(c *Config) bool { return c.Server.Port == 9999 },
		},
		{
			"OLLA_SERVER_READ_TIMEOUT",
			"45s",
			func(c *Config) bool { return c.Server.ReadTimeout == 45*time.Second },
		},
		{
			"OLLA_PROXY_RESPONSE_TIMEOUT",
			"20m",
			func(c *Config) bool { return c.Proxy.ResponseTimeout == 20*time.Minute },
		},
		{
			"OLLA_SHOW_NERD_STATS",
			"true",
			func(c *Config) bool { return c.Engineering.ShowNerdStats == true },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.envVar, func(t *testing.T) {
			os.Setenv(tc.envVar, tc.envValue)
			defer os.Unsetenv(tc.envVar)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			if !tc.checkFn(cfg) {
				t.Errorf("Environment variable %s=%s not applied correctly", tc.envVar, tc.envValue)
			}
		})
	}
}