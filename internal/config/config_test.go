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
