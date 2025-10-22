package config

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// Config holds all configuration for the application
type Config struct {
	Logging       LoggingConfig       `yaml:"logging"`
	Filename      string              `yaml:"-"`
	Translators   TranslatorsConfig   `yaml:"translators"`
	ModelRegistry ModelRegistryConfig `yaml:"model_registry"`
	Proxy         ProxyConfig         `yaml:"proxy"`
	Discovery     DiscoveryConfig     `yaml:"discovery"`
	Server        ServerConfig        `yaml:"server"`
	Engineering   EngineeringConfig   `yaml:"engineering"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string              `yaml:"host"`
	RateLimits      ServerRateLimits    `yaml:"rate_limits"`
	RequestLimits   ServerRequestLimits `yaml:"request_limits"`
	Port            int                 `yaml:"port"`
	ReadTimeout     time.Duration       `yaml:"read_timeout"`
	WriteTimeout    time.Duration       `yaml:"write_timeout"`
	IdleTimeout     time.Duration       `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration       `yaml:"shutdown_timeout"`
	RequestLogging  bool                `yaml:"request_logging"`
}

// GetAddress returns the server address in host:port format
func (s *ServerConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// ServerRequestLimits defines request size and validation limits
type ServerRequestLimits struct {
	MaxBodySize   int64 `yaml:"max_body_size"`
	MaxHeaderSize int64 `yaml:"max_header_size"`
}

// ServerRateLimits defines rate limiting configuration
type ServerRateLimits struct {
	TrustedProxyCIDRs       []string      `yaml:"trusted_proxy_cidrs"`
	TrustedProxyCIDRsParsed []*net.IPNet  // to avoid parsing every time :D
	GlobalRequestsPerMinute int           `yaml:"global_requests_per_minute"`
	PerIPRequestsPerMinute  int           `yaml:"per_ip_requests_per_minute"`
	BurstSize               int           `yaml:"burst_size"`
	HealthRequestsPerMinute int           `yaml:"health_requests_per_minute"`
	CleanupInterval         time.Duration `yaml:"cleanup_interval"`
	TrustProxyHeaders       bool          `yaml:"trust_proxy_headers"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	ProfileFilter     *domain.FilterConfig `yaml:"profile_filter,omitempty"`
	Engine            string               `yaml:"engine"`
	LoadBalancer      string               `yaml:"load_balancer"`
	Profile           string               `yaml:"profile"`
	ConnectionTimeout time.Duration        `yaml:"connection_timeout"`
	ResponseTimeout   time.Duration        `yaml:"response_timeout"`
	ReadTimeout       time.Duration        `yaml:"read_timeout"`
	RetryBackoff      time.Duration        `yaml:"retry_backoff"` // Deprecated: Use model_registry.routing_strategy instead. TODO: Removal: v0.1.0
	StreamBufferSize  int                  `yaml:"stream_buffer_size"`
	MaxRetries        int                  `yaml:"max_retries"` // Deprecated: Use model_registry.routing_strategy instead. TODO: Removal: v0.1.0
}

// DiscoveryConfig holds service discovery configuration
type DiscoveryConfig struct {
	Type            string                `yaml:"type"` // Only "static" is implemented
	Static          StaticDiscoveryConfig `yaml:"static"`
	RefreshInterval time.Duration         `yaml:"refresh_interval"`
	ModelDiscovery  ModelDiscoveryConfig  `yaml:"model_discovery"`
}

// ModelDiscoveryConfig holds model discvery specific settings
type ModelDiscoveryConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Interval          time.Duration `yaml:"interval"`
	Timeout           time.Duration `yaml:"timeout"`
	ConcurrentWorkers int           `yaml:"concurrent_workers"`
	RetryAttempts     int           `yaml:"retry_attempts"`
	RetryBackoff      time.Duration `yaml:"retry_backoff"`
}

// StaticDiscoveryConfig holds static endpoint configuration
type StaticDiscoveryConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// EndpointConfig holds configuration for an AI inference endpoint
type EndpointConfig struct {
	ModelFilter    *domain.FilterConfig `yaml:"model_filter,omitempty"`
	URL            string               `yaml:"url"`
	Name           string               `yaml:"name"`
	Type           string               `yaml:"type"`
	HealthCheckURL string               `yaml:"health_check_url"`
	ModelURL       string               `yaml:"model_url"`
	CheckInterval  time.Duration        `yaml:"check_interval"`
	CheckTimeout   time.Duration        `yaml:"check_timeout"`
	Priority       int                  `yaml:"priority"`
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

// ModelRegistryConfig holds model registry configuration
type ModelRegistryConfig struct {
	RoutingStrategy ModelRoutingStrategy `yaml:"routing_strategy"`
	Type            string               `yaml:"type"`
	Unification     UnificationConfig    `yaml:"unification"`
	EnableUnifier   bool                 `yaml:"enable_unifier"`
}

// ModelRoutingStrategy configures how models are routed when not all endpoints have them
type ModelRoutingStrategy struct {
	Type    string                      `yaml:"type"` // strict, optimistic, discovery
	Options ModelRoutingStrategyOptions `yaml:"options"`
}

// ModelRoutingStrategyOptions holds routing strategy configuration
type ModelRoutingStrategyOptions struct {
	FallbackBehavior       string        `yaml:"fallback_behavior"` // compatible_only, none, all
	DiscoveryTimeout       time.Duration `yaml:"discovery_timeout"`
	DiscoveryRefreshOnMiss bool          `yaml:"discovery_refresh_on_miss"`
}

// UnificationConfig holds model unification configuration
type UnificationConfig struct {
	CustomRules     []UnificationRuleConfig `yaml:"custom_rules"`
	CacheTTL        time.Duration           `yaml:"cache_ttl"`
	Enabled         bool                    `yaml:"enabled"`
	StaleThreshold  time.Duration           `yaml:"stale_threshold"`
	CleanupInterval time.Duration           `yaml:"cleanup_interval"`
}

// UnificationRuleConfig defines custom unification rules
type UnificationRuleConfig struct {
	FamilyOverrides map[string]string `yaml:"family_overrides"`
	NamePatterns    map[string]string `yaml:"name_patterns"`
	Platform        string            `yaml:"platform"`
}

// TranslatorsConfig holds translator-specific configuration
type TranslatorsConfig struct {
	Anthropic AnthropicTranslatorConfig `yaml:"anthropic"`
}

// Validate validates all translator configurations
// Provides defence-in-depth by ensuring all translator configs are valid
func (c *TranslatorsConfig) Validate() error {
	if err := c.Anthropic.Validate(); err != nil {
		return fmt.Errorf("anthropic translator config invalid: %w", err)
	}
	return nil
}

// MaxAnthropicMessageSize is the maximum allowed message size for Anthropic API requests (100 MiB)
const MaxAnthropicMessageSize = 100 << 20

// AnthropicTranslatorConfig holds configuration for the Anthropic translator
type AnthropicTranslatorConfig struct {
	Inspector      InspectorConfig `yaml:"inspector"`
	MaxMessageSize int64           `yaml:"max_message_size"`
	Enabled        bool            `yaml:"enabled"`
}

// InspectorConfig holds configuration for request/response inspection
type InspectorConfig struct {
	OutputDir     string `yaml:"output_dir"`
	SessionHeader string `yaml:"session_header"`
	Enabled       bool   `yaml:"enabled"`
}

// validHTTPHeaderPattern matches valid HTTP header names per RFC 7230
// Header names must be tokens (alphanumeric and !#$%&'*+-.^_`|~)
var validHTTPHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+\-.^_` + "`" + `|~]+$`)

// validateOutputPath checks if a path is dangerous system path
// Prevents writing to critical system directories
func validateOutputPath(cleanPath string) error {
	// Convert to absolute path for checking
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("invalid output_dir path: %w", err)
	}

	// Normalize for case-insensitive comparison on Windows
	absPathLower := strings.ToLower(absPath)
	cleanPathLower := strings.ToLower(cleanPath)

	// Unix-specific dangerous paths (only check on Unix-like systems)
	unixDangerousPaths := []string{
		"/etc",
		"/var",
		"/usr",
		"/bin",
		"/sbin",
		"/boot",
		"/sys",
		"/proc",
		"/dev",
		"/root",
	}

	// Windows-specific dangerous paths
	windowsDangerousPaths := []string{
		"c:\\windows",
		"c:\\program files",
		"c:\\program files (x86)",
	}

	// Universal dangerous paths (root directories)
	universalDangerousPaths := []string{
		"/",
		"\\",
		"c:\\",
	}

	// Check universal dangerous paths
	for _, dangerous := range universalDangerousPaths {
		dangerousLower := strings.ToLower(dangerous)
		if cleanPathLower == dangerousLower || absPathLower == dangerousLower {
			return fmt.Errorf("output_dir cannot be set to dangerous system path: %s", cleanPath)
		}
	}

	// Check Unix paths only if the path starts with / (Unix-style)
	if strings.HasPrefix(cleanPath, "/") {
		for _, dangerous := range unixDangerousPaths {
			dangerousWithSep := dangerous + "/"
			if cleanPath == dangerous || strings.HasPrefix(cleanPath, dangerousWithSep) {
				return fmt.Errorf("output_dir cannot be set to dangerous system path: %s", cleanPath)
			}
			// Also check absolute path
			if absPath == dangerous || strings.HasPrefix(absPath, dangerousWithSep) {
				return fmt.Errorf("output_dir resolves to dangerous system path: %s", absPath)
			}
		}
	}

	// Check Windows paths
	// Normalise both clean and absolute paths to Windows-style separators for matching
	cleanWin := strings.ReplaceAll(cleanPathLower, "/", "\\")
	absWin := strings.ReplaceAll(absPathLower, "/", "\\")
	for _, dangerous := range windowsDangerousPaths {
		dangerousLower := strings.ToLower(dangerous)
		dangerousWithSep := dangerousLower + "\\"
		if cleanWin == dangerousLower || strings.HasPrefix(cleanWin, dangerousWithSep) ||
			absWin == dangerousLower || strings.HasPrefix(absWin, dangerousWithSep) {
			return fmt.Errorf("output_dir cannot be set to dangerous system path: %s", cleanPath)
		}
	}

	return nil
}

// Validate validates the inspector configuration
// Prevents dangerous output paths and ensures session header is valid
func (c *InspectorConfig) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	// Validate output directory is not a dangerous path
	if c.OutputDir == "" {
		// Set sensible default
		c.OutputDir = "./inspector-logs"
	} else {
		// Clean the path to normalize it
		cleanPath := filepath.Clean(c.OutputDir)

		// Check for dangerous paths based on platform
		if err := validateOutputPath(cleanPath); err != nil {
			return err
		}

		c.OutputDir = cleanPath
	}

	// Validate session header is a valid HTTP header name
	if c.SessionHeader == "" {
		// Set default
		c.SessionHeader = "X-Session-ID"
	} else {
		// Remove any whitespace
		c.SessionHeader = strings.TrimSpace(c.SessionHeader)

		// Validate against HTTP header name rules (RFC 7230)
		if !validHTTPHeaderPattern.MatchString(c.SessionHeader) {
			return fmt.Errorf("session_header contains invalid characters (must be valid HTTP header name): %s", c.SessionHeader)
		}

		// Additional checks for common mistakes
		if strings.Contains(c.SessionHeader, " ") {
			return fmt.Errorf("session_header cannot contain spaces: %s", c.SessionHeader)
		}
		if strings.Contains(c.SessionHeader, ":") {
			return fmt.Errorf("session_header cannot contain colons: %s", c.SessionHeader)
		}
	}

	return nil
}

// Validate validates the Anthropic translator configuration
// Ensures message size is within safe bounds to prevent DoS and API errors
func (c *AnthropicTranslatorConfig) Validate() error {
	if c.MaxMessageSize < 0 {
		return fmt.Errorf("max_message_size must be non-negative, got %d", c.MaxMessageSize)
	}
	if c.MaxMessageSize > MaxAnthropicMessageSize {
		return fmt.Errorf("max_message_size exceeds 100 MiB safety limit, got %d", c.MaxMessageSize)
	}

	// Validate inspector configuration
	if err := c.Inspector.Validate(); err != nil {
		return fmt.Errorf("inspector config invalid: %w", err)
	}

	return nil
}
