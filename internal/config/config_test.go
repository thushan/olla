package config

import (
	"os"
	"runtime"
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

	// Test endpoint type
	if cfg.Discovery.Static.Endpoints[0].Type != "ollama" {
		t.Errorf("Expected default endpoint type 'ollama', got %s", cfg.Discovery.Static.Endpoints[0].Type)
	}

	// Test logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format 'json', got %s", cfg.Logging.Format)
	}

	// Test proxy defaults
	if cfg.Proxy.Engine != DefaultProxyEngine {
		t.Errorf("Expected proxy engine '%s', got %s", DefaultProxyEngine, cfg.Proxy.Engine)
	}
	if cfg.Proxy.LoadBalancer != DefaultLoadBalancer {
		t.Errorf("Expected load balancer '%s', got %s", DefaultLoadBalancer, cfg.Proxy.LoadBalancer)
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
	if cfg.Server.Host != DefaultHost && cfg.Server.Host != DefaultAllHost {
		t.Errorf("Expected default host %s, got %s", DefaultHost, cfg.Server.Host)
	}
}

func TestLoadConfig_WithEnvironmentVariables(t *testing.T) {
	// Set test environment variables
	testEnvVars := map[string]string{
		"OLLA_SERVER_PORT":            "8080",
		"OLLA_SERVER_HOST":            "0.0.0.0",
		"OLLA_PROXY_LOAD_BALANCER":    "round-robin",
		"OLLA_LOGGING_LEVEL":          "debug",
		"OLLA_SHOW_NERD_STATS":        "true",
		"OLLA_PROXY_RESPONSE_TIMEOUT": "15m",
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
						Type:           "ollama",
						Priority:       ptrInt(100),
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
		if endpoint.Priority != nil && *endpoint.Priority < 0 {
			t.Error("Priority should be non-negative")
		}
		if endpoint.Type == "" {
			t.Error("Endpoint should have a type specified")
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

func TestParseByteSize(t *testing.T) {
	testCases := []struct {
		input    string
		expected int64
		hasError bool
	}{
		// Valid cases
		{"100", 100, false},
		{"1024", 1024, false},
		{"1KB", 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"100MB", 100 * 1024 * 1024, false},
		{"2.5GB", int64(2.5 * 1024 * 1024 * 1024), false},
		{"0.5KB", 512, false},

		// Case insensitive
		{"100mb", 100 * 1024 * 1024, false},
		{"1gb", 1024 * 1024 * 1024, false},
		{"50KB", 50 * 1024, false},

		// With spaces (RAMInBytes handles this)
		{"100MB", 100 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},

		// Just B suffix
		{"1024B", 1024, false},

		// RAMInBytes also supports these formats
		{"1k", 1024, false},
		{"1m", 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},

		// Invalid cases
		{"", 0, true},
		{"invalid", 0, true},
		{"100XB", 0, true},
		{"-100MB", 0, true},
		{"MB100", 0, true},
		{"100 MB", 100 * 1024 * 1024, false}, // RAMInBytes allows spaces
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := parseByteSize(tc.input)

			if tc.hasError {
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tc.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %q: %v", tc.input, err)
				}
				if result != tc.expected {
					t.Errorf("Expected %d for input %q, got %d", tc.expected, tc.input, result)
				}
			}
		})
	}
}

func TestLoadConfig_WithRequestLimits(t *testing.T) {
	// Test environment variables for request limits
	testEnvVars := map[string]string{
		"OLLA_SERVER_MAX_BODY_SIZE":   "50MB",
		"OLLA_SERVER_MAX_HEADER_SIZE": "2MB",
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
		t.Fatalf("Load with request limit env vars failed: %v", err)
	}

	expectedBodySize := int64(50 * 1024 * 1024)
	expectedHeaderSize := int64(2 * 1024 * 1024)

	if cfg.Server.RequestLimits.MaxBodySize != expectedBodySize {
		t.Errorf("Expected body size %d from env var, got %d", expectedBodySize, cfg.Server.RequestLimits.MaxBodySize)
	}
	if cfg.Server.RequestLimits.MaxHeaderSize != expectedHeaderSize {
		t.Errorf("Expected header size %d from env var, got %d", expectedHeaderSize, cfg.Server.RequestLimits.MaxHeaderSize)
	}
}

func TestLoadConfig_WithRateLimits(t *testing.T) {
	// Test environment variables for rate limits
	testEnvVars := map[string]string{
		"OLLA_SERVER_GLOBAL_RATE_LIMIT":     "500",
		"OLLA_SERVER_PER_IP_RATE_LIMIT":     "50",
		"OLLA_SERVER_RATE_BURST_SIZE":       "25",
		"OLLA_SERVER_HEALTH_RATE_LIMIT":     "2000",
		"OLLA_SERVER_RATE_CLEANUP_INTERVAL": "10m",
		"OLLA_SERVER_TRUST_PROXY_HEADERS":   "true",
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
		t.Fatalf("Load with rate limit env vars failed: %v", err)
	}

	// Verify rate limit overrides
	if cfg.Server.RateLimits.GlobalRequestsPerMinute != 500 {
		t.Errorf("Expected global rate limit 500, got %d", cfg.Server.RateLimits.GlobalRequestsPerMinute)
	}
	if cfg.Server.RateLimits.PerIPRequestsPerMinute != 50 {
		t.Errorf("Expected per-IP rate limit 50, got %d", cfg.Server.RateLimits.PerIPRequestsPerMinute)
	}
	if cfg.Server.RateLimits.BurstSize != 25 {
		t.Errorf("Expected burst size 25, got %d", cfg.Server.RateLimits.BurstSize)
	}
	if cfg.Server.RateLimits.HealthRequestsPerMinute != 2000 {
		t.Errorf("Expected health rate limit 2000, got %d", cfg.Server.RateLimits.HealthRequestsPerMinute)
	}
	if cfg.Server.RateLimits.CleanupInterval != 10*time.Minute {
		t.Errorf("Expected cleanup interval 10m, got %v", cfg.Server.RateLimits.CleanupInterval)
	}
	if !cfg.Server.RateLimits.TrustProxyHeaders {
		t.Error("Expected trust proxy headers true")
	}
}

func TestDefaultConfig_RateLimits(t *testing.T) {
	cfg := DefaultConfig()

	expectedGlobal := 1000
	expectedPerIP := 100
	expectedBurst := 50
	expectedHealth := 1000
	expectedCleanup := 5 * time.Minute

	if cfg.Server.RateLimits.GlobalRequestsPerMinute != expectedGlobal {
		t.Errorf("Expected global rate limit %d, got %d", expectedGlobal, cfg.Server.RateLimits.GlobalRequestsPerMinute)
	}
	if cfg.Server.RateLimits.PerIPRequestsPerMinute != expectedPerIP {
		t.Errorf("Expected per-IP rate limit %d, got %d", expectedPerIP, cfg.Server.RateLimits.PerIPRequestsPerMinute)
	}
	if cfg.Server.RateLimits.BurstSize != expectedBurst {
		t.Errorf("Expected burst size %d, got %d", expectedBurst, cfg.Server.RateLimits.BurstSize)
	}
	if cfg.Server.RateLimits.HealthRequestsPerMinute != expectedHealth {
		t.Errorf("Expected health rate limit %d, got %d", expectedHealth, cfg.Server.RateLimits.HealthRequestsPerMinute)
	}
	if cfg.Server.RateLimits.CleanupInterval != expectedCleanup {
		t.Errorf("Expected cleanup interval %v, got %v", expectedCleanup, cfg.Server.RateLimits.CleanupInterval)
	}
	if cfg.Server.RateLimits.TrustProxyHeaders {
		t.Error("Expected trust proxy headers false by default")
	}
}

func TestLoadConfig_WithTrustedProxyCIDRs(t *testing.T) {
	testEnvVars := map[string]string{
		"OLLA_SERVER_TRUSTED_PROXY_CIDRS": "10.0.0.0/8,172.16.0.0/12,192.168.1.0/24",
	}

	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}

	defer func() {
		for key := range testEnvVars {
			os.Unsetenv(key)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with trusted proxy CIDRs failed: %v", err)
	}

	expectedCIDRs := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.1.0/24"}
	if len(cfg.Server.RateLimits.TrustedProxyCIDRs) != len(expectedCIDRs) {
		t.Errorf("Expected %d CIDRs, got %d", len(expectedCIDRs), len(cfg.Server.RateLimits.TrustedProxyCIDRs))
	}

	for i, expected := range expectedCIDRs {
		if i >= len(cfg.Server.RateLimits.TrustedProxyCIDRs) {
			t.Errorf("Missing CIDR at index %d", i)
			continue
		}
		if cfg.Server.RateLimits.TrustedProxyCIDRs[i] != expected {
			t.Errorf("Expected CIDR %s at index %d, got %s", expected, i, cfg.Server.RateLimits.TrustedProxyCIDRs[i])
		}
	}
}

func TestDefaultConfig_TrustedProxyCIDRs(t *testing.T) {
	cfg := DefaultConfig()

	expectedCIDRs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	if len(cfg.Server.RateLimits.TrustedProxyCIDRs) != len(expectedCIDRs) {
		t.Errorf("Expected %d default CIDRs, got %d", len(expectedCIDRs), len(cfg.Server.RateLimits.TrustedProxyCIDRs))
	}

	for i, expected := range expectedCIDRs {
		if i >= len(cfg.Server.RateLimits.TrustedProxyCIDRs) {
			t.Errorf("Missing default CIDR at index %d", i)
			continue
		}
		if cfg.Server.RateLimits.TrustedProxyCIDRs[i] != expected {
			t.Errorf("Expected default CIDR %s at index %d, got %s", expected, i, cfg.Server.RateLimits.TrustedProxyCIDRs[i])
		}
	}
}

func TestAnthropicTranslatorConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		config      AnthropicTranslatorConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with 10MB limit",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 10 << 20, // 10MB
			},
			expectError: false,
		},
		{
			name: "valid config with 50MB limit",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 50 << 20, // 50MB
			},
			expectError: false,
		},
		{
			name: "valid config at upper bound (100MB)",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 100 << 20, // 100MB
			},
			expectError: false,
		},
		{
			name: "valid config with zero size (will use default in translator)",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 0,
			},
			expectError: false,
		},
		{
			name: "invalid config with negative size",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: -1,
			},
			expectError: true,
			errorMsg:    "max_message_size must be non-negative",
		},
		{
			name: "invalid config exceeding 100MB limit",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 101 << 20, // 101MB
			},
			expectError: true,
			errorMsg:    "max_message_size exceeds 100 MiB safety limit",
		},
		{
			name: "invalid config way over limit",
			config: AnthropicTranslatorConfig{
				Enabled:        true,
				MaxMessageSize: 500 << 20, // 500MB
			},
			expectError: true,
			errorMsg:    "max_message_size exceeds 100 MiB safety limit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, but got nil", tc.errorMsg)
				} else if !contains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

func TestLoadConfig_WithTranslatorConfig(t *testing.T) {
	// Test environment variables for translator config
	testEnvVars := map[string]string{
		"OLLA_TRANSLATORS_ANTHROPIC_ENABLED":          "true",
		"OLLA_TRANSLATORS_ANTHROPIC_MAX_MESSAGE_SIZE": "20971520", // 20MB
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
		t.Fatalf("Load with translator env vars failed: %v", err)
	}

	// Verify translator config overrides
	if !cfg.Translators.Anthropic.Enabled {
		t.Error("Expected Anthropic translator enabled from env var")
	}
	expectedSize := int64(20 << 20) // 20MB
	if cfg.Translators.Anthropic.MaxMessageSize != expectedSize {
		t.Errorf("Expected max message size %d from env var, got %d",
			expectedSize, cfg.Translators.Anthropic.MaxMessageSize)
	}
}

func TestLoadConfig_WithPassthroughEnabledEnvVar(t *testing.T) {
	// Test that OLLA_TRANSLATORS_ANTHROPIC_PASSTHROUGH_ENABLED overrides config
	testCases := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"disable passthrough via env var", "false", false},
		{"enable passthrough via env var", "true", true},
		{"disable passthrough via 0", "0", false},
		{"enable passthrough via 1", "1", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("OLLA_TRANSLATORS_ANTHROPIC_PASSTHROUGH_ENABLED", tc.envValue)
			defer os.Unsetenv("OLLA_TRANSLATORS_ANTHROPIC_PASSTHROUGH_ENABLED")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			if cfg.Translators.Anthropic.PassthroughEnabled != tc.expected {
				t.Errorf("Expected PassthroughEnabled=%v from env var %q, got %v",
					tc.expected, tc.envValue, cfg.Translators.Anthropic.PassthroughEnabled)
			}
		})
	}
}

func TestDefaultConfig_Translators(t *testing.T) {
	cfg := DefaultConfig()

	// Test Anthropic translator defaults
	if !cfg.Translators.Anthropic.Enabled {
		t.Error("Expected Anthropic translator enabled by default")
	}

	if !cfg.Translators.Anthropic.PassthroughEnabled {
		t.Error("Expected Anthropic translator passthrough enabled by default")
	}

	if cfg.Translators.Anthropic.Inspector.Enabled {
		t.Error("Expected Anthropic translator inspector disabled by default")
	}

	expectedSize := int64(10 << 20) // 10MB
	if cfg.Translators.Anthropic.MaxMessageSize != expectedSize {
		t.Errorf("Expected default max message size %d, got %d",
			expectedSize, cfg.Translators.Anthropic.MaxMessageSize)
	}
}

// TestInspectorConfig_Validate tests inspector configuration validation
func TestInspectorConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		config      InspectorConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with custom path",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./inspector-logs",
				SessionHeader: "X-Session-ID",
			},
			expectError: false,
		},
		{
			name: "valid config with empty path (gets default)",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "",
			},
			expectError: false,
		},
		{
			name: "valid config with custom header",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./logs",
				SessionHeader: "X-Custom-Session",
			},
			expectError: false,
		},
		{
			name: "disabled config skips validation",
			config: InspectorConfig{
				Enabled:       false,
				OutputDir:     "/etc", // Would be invalid if enabled
				SessionHeader: "invalid header!",
			},
			expectError: false,
		},
		// Note: Unix path tests are skipped on Windows as they're not dangerous there
		{
			name: "invalid config with root path",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "/",
			},
			expectError: true,
			errorMsg:    "dangerous system path",
		},
		{
			name: "invalid config with Windows system path",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "C:\\Windows",
			},
			expectError: true,
			errorMsg:    "dangerous system path",
		},
		{
			name: "invalid config with invalid header (spaces)",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./logs",
				SessionHeader: "Invalid Header",
			},
			expectError: true,
			errorMsg:    "invalid characters",
		},
		{
			name: "invalid config with invalid header (colon)",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./logs",
				SessionHeader: "X-Session:ID",
			},
			expectError: true,
			errorMsg:    "invalid characters",
		},
		{
			name: "invalid config with invalid header (special chars)",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./logs",
				SessionHeader: "X-Session@ID",
			},
			expectError: true,
			errorMsg:    "invalid characters",
		},
		{
			name: "valid config with dashes and underscores in header",
			config: InspectorConfig{
				Enabled:       true,
				OutputDir:     "./logs",
				SessionHeader: "X-Custom_Session-ID",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, but got nil", tc.errorMsg)
				} else if !contains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

// TestInspectorConfig_DefaultValues tests that validation sets sensible defaults
func TestInspectorConfig_DefaultValues(t *testing.T) {
	config := InspectorConfig{
		Enabled: true,
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check defaults were set
	if config.OutputDir == "" {
		t.Error("Expected OutputDir to be set to default")
	}
	if config.OutputDir != "./inspector-logs" {
		t.Errorf("Expected default OutputDir './inspector-logs', got %s", config.OutputDir)
	}
	if config.SessionHeader == "" {
		t.Error("Expected SessionHeader to be set to default")
	}
	if config.SessionHeader != "X-Session-ID" {
		t.Errorf("Expected default SessionHeader 'X-Session-ID', got %s", config.SessionHeader)
	}
}

// TestInspectorConfig_UnixPaths tests Unix-specific dangerous paths
func TestInspectorConfig_UnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix path tests on Windows")
	}

	unixTests := []struct {
		name        string
		config      InspectorConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "unix: invalid config with /etc path",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "/etc",
			},
			expectError: true,
			errorMsg:    "dangerous system path",
		},
		{
			name: "unix: invalid config with /var path",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "/var",
			},
			expectError: true,
			errorMsg:    "dangerous system path",
		},
		{
			name: "unix: invalid config with /usr path",
			config: InspectorConfig{
				Enabled:   true,
				OutputDir: "/usr/local/olla",
			},
			expectError: true,
			errorMsg:    "dangerous system path",
		},
	}

	for _, tc := range unixTests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, but got nil", tc.errorMsg)
				} else if !contains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoadConfig_WithModelAliases(t *testing.T) {
	configYAML := `
model_aliases:
  gpt-oss-120b:
    - "gpt-oss:120b"
    - gpt-oss-120b-MLX
    - gguf_gpt_oss_120b.gguf
  my-llama:
    - "llama3.1:8b"
    - llama-3.1-8b
`
	tmpFile, err := os.CreateTemp(t.TempDir(), "olla-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configYAML); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ModelAliases == nil {
		t.Fatal("expected ModelAliases to be set")
	}

	if len(cfg.ModelAliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(cfg.ModelAliases))
	}

	gptModels := cfg.ModelAliases["gpt-oss-120b"]
	if len(gptModels) != 3 {
		t.Errorf("expected 3 models for gpt-oss-120b alias, got %d", len(gptModels))
	}
	if gptModels[0] != "gpt-oss:120b" {
		t.Errorf("expected first model to be gpt-oss:120b, got %s", gptModels[0])
	}

	llamaModels := cfg.ModelAliases["my-llama"]
	if len(llamaModels) != 2 {
		t.Errorf("expected 2 models for my-llama alias, got %d", len(llamaModels))
	}
}

func TestDefaultConfig_NoModelAliases(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ModelAliases != nil {
		t.Error("expected nil ModelAliases in default config")
	}
}

func TestValidateModelAliases_NoAliases(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.ValidateModelAliases(); err != nil {
		t.Errorf("expected no error for empty aliases, got: %v", err)
	}
}

func TestValidateModelAliases_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b-MLX", "gguf_gpt_oss_120b.gguf"},
		"my-llama":     {"llama3.1:8b", "llama-3.1-8b"},
	}

	if err := cfg.ValidateModelAliases(); err != nil {
		t.Errorf("expected no error for valid config, got: %v", err)
	}
}

func TestValidateModelAliases_EmptyModelList(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"empty-alias": {},
	}

	err := cfg.ValidateModelAliases()
	if err == nil {
		t.Error("expected error for empty model list")
	}
	if !stringContains(err.Error(), "no actual models configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateModelAliases_EmptyModelName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"my-alias": {"model-a", "", "model-c"},
	}

	err := cfg.ValidateModelAliases()
	if err == nil {
		t.Error("expected error for empty model name")
	}
	if !stringContains(err.Error(), "empty model name at position 1") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateModelAliases_WhitespaceInAliasName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		" leading-space": {"model-a"},
	}

	err := cfg.ValidateModelAliases()
	if err == nil {
		t.Error("expected error for whitespace in alias name")
	}
	if !stringContains(err.Error(), "leading/trailing whitespace") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateModelAliases_TrailingWhitespace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"trailing-space ": {"model-a"},
	}

	err := cfg.ValidateModelAliases()
	if err == nil {
		t.Error("expected error for trailing whitespace in alias name")
	}
	if !stringContains(err.Error(), "leading/trailing whitespace") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateModelAliases_DuplicateModelNames(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"my-alias": {"model-a", "model-b", "model-a"},
	}

	// Duplicates should produce a warning but NOT an error
	err := cfg.ValidateModelAliases()
	if err != nil {
		t.Errorf("expected no error for duplicate model names (should only warn), got: %v", err)
	}
}

func TestValidateModelAliases_SelfReferencingAlias(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b", "gpt-oss-120b"},
	}

	// Self-referencing is valid (alias name is also an actual model name)
	err := cfg.ValidateModelAliases()
	if err != nil {
		t.Errorf("expected no error for self-referencing alias, got: %v", err)
	}
}

// TestDefaultConfig_ModelDiscovery verifies the ModelDiscovery block is
// populated with safe, non-zero defaults so the ticker and errgroup won't panic
// on a fresh install.
func TestDefaultConfig_ModelDiscovery(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	md := cfg.Discovery.ModelDiscovery

	if !md.Enabled {
		t.Error("Expected ModelDiscovery.Enabled to be true by default")
	}
	if md.Interval != 5*time.Minute {
		t.Errorf("Expected Interval 5m, got %v", md.Interval)
	}
	if md.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", md.Timeout)
	}
	if md.ConcurrentWorkers != 5 {
		t.Errorf("Expected ConcurrentWorkers 5, got %d", md.ConcurrentWorkers)
	}
	if md.RetryAttempts != 3 {
		t.Errorf("Expected RetryAttempts 3, got %d", md.RetryAttempts)
	}
	if md.RetryBackoff != 1*time.Second {
		t.Errorf("Expected RetryBackoff 1s, got %v", md.RetryBackoff)
	}
}

// TestConfigValidate_DefaultConfigIsValid confirms that an out-of-the-box
// DefaultConfig passes Validate() without modification.
func TestConfigValidate_DefaultConfigIsValid(t *testing.T) {
	t.Parallel()

	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() returned unexpected error: %v", err)
	}
}

// TestConfigValidate_RejectsEmptyFields covers each field that Validate()
// checks individually so a regression in any single guard is caught cleanly.
func TestConfigValidate_RejectsEmptyFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		modify      func(*Config)
		errContains string
	}{
		{
			name:        "empty discovery.type",
			modify:      func(c *Config) { c.Discovery.Type = "" },
			errContains: "discovery.type",
		},
		{
			name:        "empty proxy.engine",
			modify:      func(c *Config) { c.Proxy.Engine = "" },
			errContains: "proxy.engine",
		},
		{
			name:        "empty proxy.load_balancer",
			modify:      func(c *Config) { c.Proxy.LoadBalancer = "" },
			errContains: "proxy.load_balancer",
		},
		{
			name:        "server.port zero",
			modify:      func(c *Config) { c.Server.Port = 0 },
			errContains: "server.port",
		},
		{
			name:        "server.port negative",
			modify:      func(c *Config) { c.Server.Port = -1 },
			errContains: "server.port",
		},
		{
			name:        "server.port above 65535",
			modify:      func(c *Config) { c.Server.Port = 99999 },
			errContains: "server.port",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			tc.modify(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Expected error containing %q, got nil", tc.errContains)
			}
			if !contains(err.Error(), tc.errContains) {
				t.Errorf("Expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}

// TestConfigValidate_ModelDiscoveryEnabled checks that Validate() rejects
// zero values for interval, workers, and timeout when model discovery is on,
// since those would cause a ticker panic or immediate context expiry at runtime.
func TestConfigValidate_ModelDiscoveryEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		modify      func(*ModelDiscoveryConfig)
		errContains string
	}{
		{
			name:        "zero interval",
			modify:      func(md *ModelDiscoveryConfig) { md.Interval = 0 },
			errContains: "interval",
		},
		{
			name:        "zero concurrent_workers",
			modify:      func(md *ModelDiscoveryConfig) { md.ConcurrentWorkers = 0 },
			errContains: "concurrent_workers",
		},
		{
			name:        "zero timeout",
			modify:      func(md *ModelDiscoveryConfig) { md.Timeout = 0 },
			errContains: "timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.Discovery.ModelDiscovery.Enabled = true
			tc.modify(&cfg.Discovery.ModelDiscovery)

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Expected error containing %q, got nil", tc.errContains)
			}
			if !contains(err.Error(), tc.errContains) {
				t.Errorf("Expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}

// TestConfigValidate_ModelDiscoveryDisabled confirms that zero values for
// interval, workers, and timeout are accepted when model discovery is off —
// operators may disable discovery entirely in production.
func TestConfigValidate_ModelDiscoveryDisabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Discovery.ModelDiscovery = ModelDiscoveryConfig{
		Enabled:           false,
		Interval:          0,
		Timeout:           0,
		ConcurrentWorkers: 0,
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected no error when model discovery is disabled with zero values, got: %v", err)
	}
}

// TestConfigValidate_WriteTimeoutZeroAllowed confirms that WriteTimeout == 0
// is intentionally accepted. The default is zero to support long-running
// streaming responses, and Validate() must not block that use case.
func TestConfigValidate_WriteTimeoutZeroAllowed(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Server.WriteTimeout = 0

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected no error for WriteTimeout == 0 (valid streaming config), got: %v", err)
	}
}

// TestDefaultConfig_CorsDefaults verifies the out-of-the-box CORS config:
// disabled, wildcard origins, safe methods, no credentials, 5-min preflight cache.
func TestDefaultConfig_CorsDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cors := cfg.Server.Cors

	if cors.Enabled {
		t.Error("expected CORS disabled by default")
	}
	if cors.AllowCredentials {
		t.Error("expected AllowCredentials false by default")
	}
	if cors.MaxAge != 300 {
		t.Errorf("expected MaxAge 300, got %d", cors.MaxAge)
	}
	if len(cors.AllowedOrigins) != 1 || cors.AllowedOrigins[0] != "*" {
		t.Errorf("expected AllowedOrigins=[\"*\"], got %v", cors.AllowedOrigins)
	}
	wantMethods := []string{"GET", "POST", "OPTIONS"}
	if len(cors.AllowedMethods) != len(wantMethods) {
		t.Errorf("expected %d AllowedMethods, got %d", len(wantMethods), len(cors.AllowedMethods))
	}
	for i, m := range wantMethods {
		if i < len(cors.AllowedMethods) && cors.AllowedMethods[i] != m {
			t.Errorf("AllowedMethods[%d]: expected %s, got %s", i, m, cors.AllowedMethods[i])
		}
	}
	if len(cors.AllowedHeaders) != 1 || cors.AllowedHeaders[0] != "*" {
		t.Errorf("expected AllowedHeaders=[\"*\"], got %v", cors.AllowedHeaders)
	}
	if len(cors.ExposedHeaders) != 0 {
		t.Errorf("expected ExposedHeaders empty by default, got %v", cors.ExposedHeaders)
	}
}

// TestCorsConfig_YAMLParsing confirms a full cors block round-trips correctly through YAML.
func TestCorsConfig_YAMLParsing(t *testing.T) {
	configYAML := `
server:
  host: localhost
  port: 40114
  cors:
    enabled: true
    allowed_origins:
      - "http://localhost:3000"
      - "http://localhost:8080"
    allowed_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "OPTIONS"
    allowed_headers:
      - "Content-Type"
      - "Authorization"
    exposed_headers:
      - "X-Olla-Request-ID"
    allow_credentials: true
    max_age: 600
`
	import_tmp, err := os.CreateTemp(t.TempDir(), "olla-cors-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(import_tmp.Name())
	if _, err := import_tmp.WriteString(configYAML); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	import_tmp.Close()

	cfg, err := Load(import_tmp.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cors := cfg.Server.Cors
	if !cors.Enabled {
		t.Error("expected CORS enabled")
	}
	if !cors.AllowCredentials {
		t.Error("expected AllowCredentials true")
	}
	if cors.MaxAge != 600 {
		t.Errorf("expected MaxAge 600, got %d", cors.MaxAge)
	}
	if len(cors.AllowedOrigins) != 2 {
		t.Errorf("expected 2 allowed origins, got %d", len(cors.AllowedOrigins))
	}
	if cors.AllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("expected first origin http://localhost:3000, got %s", cors.AllowedOrigins[0])
	}
	if len(cors.AllowedMethods) != 4 {
		t.Errorf("expected 4 allowed methods, got %d", len(cors.AllowedMethods))
	}
	if len(cors.AllowedHeaders) != 2 {
		t.Errorf("expected 2 allowed headers, got %d", len(cors.AllowedHeaders))
	}
	if len(cors.ExposedHeaders) != 1 || cors.ExposedHeaders[0] != "X-Olla-Request-ID" {
		t.Errorf("expected ExposedHeaders=[\"X-Olla-Request-ID\"], got %v", cors.ExposedHeaders)
	}
}

// TestCorsConfig_EnvOverrides checks that each OLLA_SERVER_CORS_* variable is parsed and applied.
// Not parallel: env vars are process-global; parallel subtests would bleed into each other.
func TestCorsConfig_EnvOverrides(t *testing.T) {
	testCases := []struct {
		name    string
		envVars map[string]string
		checkFn func(*Config) bool
		desc    string
	}{
		{
			name:    "CORS enabled",
			envVars: map[string]string{"OLLA_SERVER_CORS_ENABLED": "true"},
			checkFn: func(c *Config) bool { return c.Server.Cors.Enabled },
			desc:    "Cors.Enabled should be true",
		},
		{
			name:    "CORS disabled via 0",
			envVars: map[string]string{"OLLA_SERVER_CORS_ENABLED": "0"},
			checkFn: func(c *Config) bool { return !c.Server.Cors.Enabled },
			desc:    "Cors.Enabled should be false",
		},
		{
			name:    "allowed origins comma-separated",
			envVars: map[string]string{"OLLA_SERVER_CORS_ALLOWED_ORIGINS": "http://a.com,http://b.com"},
			checkFn: func(c *Config) bool {
				return len(c.Server.Cors.AllowedOrigins) == 2 &&
					c.Server.Cors.AllowedOrigins[0] == "http://a.com" &&
					c.Server.Cors.AllowedOrigins[1] == "http://b.com"
			},
			desc: "AllowedOrigins should have 2 entries",
		},
		{
			name:    "allowed methods comma-separated",
			envVars: map[string]string{"OLLA_SERVER_CORS_ALLOWED_METHODS": "GET,POST,DELETE"},
			checkFn: func(c *Config) bool { return len(c.Server.Cors.AllowedMethods) == 3 },
			desc:    "AllowedMethods should have 3 entries",
		},
		{
			name:    "allowed headers comma-separated",
			envVars: map[string]string{"OLLA_SERVER_CORS_ALLOWED_HEADERS": "Content-Type,Authorization"},
			checkFn: func(c *Config) bool { return len(c.Server.Cors.AllowedHeaders) == 2 },
			desc:    "AllowedHeaders should have 2 entries",
		},
		{
			name:    "exposed headers comma-separated",
			envVars: map[string]string{"OLLA_SERVER_CORS_EXPOSED_HEADERS": "X-Olla-Request-ID,X-Olla-Model"},
			checkFn: func(c *Config) bool { return len(c.Server.Cors.ExposedHeaders) == 2 },
			desc:    "ExposedHeaders should have 2 entries",
		},
		{
			name: "allow credentials true",
			envVars: map[string]string{
				"OLLA_SERVER_CORS_ALLOW_CREDENTIALS": "true",
				"OLLA_SERVER_CORS_ENABLED":           "true",
				"OLLA_SERVER_CORS_ALLOWED_ORIGINS":   "http://trusted.example.com",
			},
			checkFn: func(c *Config) bool { return c.Server.Cors.AllowCredentials },
			desc:    "AllowCredentials should be true",
		},
		{
			name:    "max age int",
			envVars: map[string]string{"OLLA_SERVER_CORS_MAX_AGE": "3600"},
			checkFn: func(c *Config) bool { return c.Server.Cors.MaxAge == 3600 },
			desc:    "MaxAge should be 3600",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				os.Setenv(k, v)
			}
			defer func() {
				for k := range tc.envVars {
					os.Unsetenv(k)
				}
			}()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			if !tc.checkFn(cfg) {
				t.Errorf("%s: check failed for env %v", tc.desc, tc.envVars)
			}
		})
	}
}

// TestCorsConfig_Validate covers the two error paths and the happy path.
func TestCorsConfig_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		cors        CorsConfig
		errContains string
	}{
		{
			name: "disabled config skips validation entirely",
			cors: CorsConfig{
				Enabled:          false,
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true, // would be invalid if enabled
				MaxAge:           -1,   // would be invalid if enabled
			},
		},
		{
			name: "valid enabled config with explicit origins and credentials",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"http://localhost:3000"},
				AllowedMethods:   []string{"GET", "POST"},
				AllowCredentials: true,
				MaxAge:           300,
			},
		},
		{
			name: "valid enabled config with wildcard and no credentials",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"*"},
				AllowCredentials: false,
				MaxAge:           600,
			},
		},
		{
			name: "credentials with wildcard origin is rejected",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
				MaxAge:           300,
			},
			errContains: "allow_credentials",
		},
		{
			name: "negative max_age is rejected",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"http://example.com"},
				AllowCredentials: false,
				MaxAge:           -1,
			},
			errContains: "max_age",
		},
		{
			name: "zero max_age is accepted",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"http://example.com"},
				AllowCredentials: false,
				MaxAge:           0,
			},
		},
		{
			name: "credentials with mixed origins containing wildcard is rejected",
			cors: CorsConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"http://safe.example.com", "*"},
				AllowCredentials: true,
				MaxAge:           300,
			},
			errContains: "allow_credentials",
		},
		{
			// rs/cors treats a nil slice as allow-all; reject it so the operator
			// must be explicit rather than accidentally opening up all origins.
			name: "nil allowed_origins when enabled is rejected",
			cors: CorsConfig{
				Enabled:        true,
				AllowedOrigins: nil,
			},
			errContains: "allowed_origins",
		},
		{
			// Same as nil — an empty slice is indistinguishable to rs/cors and
			// equally surprising to an operator who left the key blank.
			name: "empty allowed_origins slice when enabled is rejected",
			cors: CorsConfig{
				Enabled:        true,
				AllowedOrigins: []string{},
			},
			errContains: "allowed_origins",
		},
		{
			// Disabled CORS skips validation, so a nil origins list is fine.
			name: "nil allowed_origins when disabled is accepted",
			cors: CorsConfig{
				Enabled:        false,
				AllowedOrigins: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cors.Validate()

			if tc.errContains == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if !contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			}
		})
	}
}

// TestConfigValidate_CorsInvalidPropagates confirms that a bad CORS config causes
// Config.Validate() to return an error (i.e. it's wired into the startup gate).
func TestConfigValidate_CorsInvalidPropagates(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Server.Cors.Enabled = true
	cfg.Server.Cors.AllowedOrigins = []string{"*"}
	cfg.Server.Cors.AllowCredentials = true

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for credentials+wildcard, got nil")
	}
	if !contains(err.Error(), "allow_credentials") {
		t.Errorf("expected error to mention allow_credentials, got: %v", err)
	}
}

// TestEnvOverrides_NewTunables verifies that the four tunables added on the
// feature/config-tunables branch are each individually honoured by
// applyEnvOverrides, so the docs claim (all settings support OLLA_ prefix) holds.
// Not parallel: env vars are process-global; sequential subtests prevent bleed.
func TestEnvOverrides_NewTunables(t *testing.T) {
	testCases := []struct {
		envVar  string
		value   string
		checkFn func(*Config) bool
	}{
		{
			envVar:  "OLLA_SERVER_READ_HEADER_TIMEOUT",
			value:   "15s",
			checkFn: func(c *Config) bool { return c.Server.ReadHeaderTimeout == 15*time.Second },
		},
		{
			envVar:  "OLLA_PROXY_CONNECTION_KEEP_ALIVE",
			value:   "90s",
			checkFn: func(c *Config) bool { return c.Proxy.ConnectionKeepAlive == 90*time.Second },
		},
		{
			envVar:  "OLLA_PROXY_RESPONSE_HEADER_TIMEOUT",
			value:   "180s",
			checkFn: func(c *Config) bool { return c.Proxy.ResponseHeaderTimeout == 180*time.Second },
		},
		{
			envVar:  "OLLA_PROXY_TLS_HANDSHAKE_TIMEOUT",
			value:   "20s",
			checkFn: func(c *Config) bool { return c.Proxy.TLSHandshakeTimeout == 20*time.Second },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.envVar, func(t *testing.T) {
			os.Setenv(tc.envVar, tc.value)
			defer os.Unsetenv(tc.envVar)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			if !tc.checkFn(cfg) {
				t.Errorf("%s=%s was not applied: got %+v", tc.envVar, tc.value, cfg.Proxy)
			}
		})
	}
}
