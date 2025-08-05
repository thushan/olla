package config

import (
	"fmt"
	"github.com/thushan/olla/internal/core/constants"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/thushan/olla/internal/util"

	"github.com/docker/go-units"
	"gopkg.in/yaml.v3"
)

const (
	DefaultPort              = 40114
	DefaultHost              = "localhost"
	DefaultProxyProfile      = constants.ConfigurationProxyProfileAuto
	DefaultProxyEngine       = "sherpa"
	DefaultLoadBalancer      = "priority"
	DefaultModelRegistryType = "memory"
	DefaultDiscoveryType     = "static"
)

var DefaultLocalNetworkTrustedCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            DefaultHost,
			Port:            DefaultPort,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    0, // No timeout for proxies
			ShutdownTimeout: 10 * time.Second,
			RequestLogging:  true,
			RequestLimits: ServerRequestLimits{
				MaxBodySize:   100 * 1024 * 1024,
				MaxHeaderSize: 1024 * 1024,
			},
			RateLimits: ServerRateLimits{
				GlobalRequestsPerMinute: 1000,
				PerIPRequestsPerMinute:  100,
				BurstSize:               50,
				HealthRequestsPerMinute: 1000,
				CleanupInterval:         5 * time.Minute,
				TrustProxyHeaders:       false,
				TrustedProxyCIDRs:       DefaultLocalNetworkTrustedCIDRs,
				TrustedProxyCIDRsParsed: nil, // Will be parsed later
			},
		},
		Proxy: ProxyConfig{
			Engine:            DefaultProxyEngine,
			LoadBalancer:      DefaultLoadBalancer,
			Profile:           DefaultProxyProfile,
			StreamBufferSize:  8 * 1024, // 8KB
			ConnectionTimeout: 30 * time.Second,
			ResponseTimeout:   10 * time.Minute,
			ReadTimeout:       120 * time.Second,
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
		},
		Discovery: DiscoveryConfig{
			Type:            DefaultDiscoveryType,
			RefreshInterval: 30 * time.Second,
			Static: StaticDiscoveryConfig{
				Endpoints: []EndpointConfig{
					{
						URL:            "http://localhost:11434",
						Name:           "localhost",
						Type:           "ollama",
						Priority:       100,
						HealthCheckURL: "/health",
						ModelURL:       "/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				},
			},
		},
		ModelRegistry: ModelRegistryConfig{
			Type:          DefaultModelRegistryType,
			EnableUnifier: true,
			Unification: UnificationConfig{
				Enabled:  true,
				CacheTTL: 10 * time.Minute,
				CustomRules: []UnificationRuleConfig{
					{
						Platform: "ollama",
						FamilyOverrides: map[string]string{
							"phi4:*": "phi",
						},
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

func Load(flagConfigFile ...string) (*Config, error) {
	config := DefaultConfig()

	// Simple config file loading - check a few standard locations
	configPaths := []string{"config/config.local.yaml", "config/config.yaml", "config.yaml", "default.yaml"}

	// Priority: flag > environment variable > default paths
	if len(flagConfigFile) > 0 && flagConfigFile[0] != "" {
		configPaths = []string{flagConfigFile[0]}
	} else if configFile := os.Getenv("OLLA_CONFIG_FILE"); configFile != "" {
		configPaths = []string{configFile}
	}

	var configLoaded bool
	var configFilename string

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", path, err)
			}
			configLoaded = true
			configFilename = path
			// Cache any settings so we don't reparse etc
			ApplyConfigCaches(config)
			break
		}
	}

	if !configLoaded {
		if len(flagConfigFile) > 0 && flagConfigFile[0] != "" {
			return nil, fmt.Errorf("specified config file not found: %s", flagConfigFile[0])
		} else if configFile := os.Getenv("OLLA_CONFIG_FILE"); configFile != "" {
			return nil, fmt.Errorf("specified config file not found: %s", configFile)
		}
	}

	// Apply essential environment overrides only
	applyEnvOverrides(config)
	config.Filename = configFilename
	return config, nil
}

func ApplyConfigCaches(config *Config) {
	if val := config.Server.RateLimits.TrustedProxyCIDRs; len(val) > 0 {
		if trustedCIDRs, err := util.ParseTrustedCIDRs(val); err == nil {
			config.Server.RateLimits.TrustedProxyCIDRs = val
			config.Server.RateLimits.TrustedProxyCIDRsParsed = trustedCIDRs
		} else {
			config.Server.RateLimits.TrustedProxyCIDRsParsed = nil // fallback to empty if parsing fails
		}
	}

	CheckFallbackCIDRs(config)
}

func CheckFallbackCIDRs(config *Config) {
	if config.Server.RateLimits.TrustedProxyCIDRsParsed == nil {
		if localCIDRs, err := util.ParseTrustedCIDRs(DefaultLocalNetworkTrustedCIDRs); err == nil {
			config.Server.RateLimits.TrustedProxyCIDRs = DefaultLocalNetworkTrustedCIDRs
			config.Server.RateLimits.TrustedProxyCIDRsParsed = localCIDRs
		} else {
			slog.Error("BUGCHECK: Failed to parse trusted local proxy CIDRs, please file a bug report")
		}
	}
}

//nolint:gocognit // flat env parsing logic, intentionally verbose
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
	if val := os.Getenv("OLLA_SERVER_MAX_BODY_SIZE"); val != "" {
		if size, err := parseByteSize(val); err == nil {
			config.Server.RequestLimits.MaxBodySize = size
		}
	}
	if val := os.Getenv("OLLA_SERVER_MAX_HEADER_SIZE"); val != "" {
		if size, err := parseByteSize(val); err == nil {
			config.Server.RequestLimits.MaxHeaderSize = size
		}
	}

	if val := os.Getenv("OLLA_SERVER_GLOBAL_RATE_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Server.RateLimits.GlobalRequestsPerMinute = limit
		}
	}
	if val := os.Getenv("OLLA_SERVER_PER_IP_RATE_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Server.RateLimits.PerIPRequestsPerMinute = limit
		}
	}
	if val := os.Getenv("OLLA_SERVER_RATE_BURST_SIZE"); val != "" {
		if burst, err := strconv.Atoi(val); err == nil {
			config.Server.RateLimits.BurstSize = burst
		}
	}
	if val := os.Getenv("OLLA_SERVER_HEALTH_RATE_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Server.RateLimits.HealthRequestsPerMinute = limit
		}
	}
	if val := os.Getenv("OLLA_SERVER_RATE_CLEANUP_INTERVAL"); val != "" {
		if interval, err := time.ParseDuration(val); err == nil {
			config.Server.RateLimits.CleanupInterval = interval
		}
	}
	if val := os.Getenv("OLLA_SERVER_TRUST_PROXY_HEADERS"); val != "" {
		if trust, err := strconv.ParseBool(val); err == nil {
			config.Server.RateLimits.TrustProxyHeaders = trust
		}
	}
	if val := os.Getenv("OLLA_SERVER_TRUSTED_PROXY_CIDRS"); val != "" {
		cidrs := strings.Split(val, ",")
		if trustedCIDRs, err := util.ParseTrustedCIDRs(cidrs); err == nil {
			config.Server.RateLimits.TrustedProxyCIDRs = cidrs
			config.Server.RateLimits.TrustedProxyCIDRsParsed = trustedCIDRs
		} else {
			CheckFallbackCIDRs(config)
		}
	}

	if val := os.Getenv("OLLA_PROXY_ENGINE"); val != "" {
		config.Proxy.Engine = val
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
	if val := os.Getenv("OLLA_PROXY_LOAD_BALANCER"); val != "" {
		config.Proxy.LoadBalancer = val
	}

	if val := os.Getenv("OLLA_LOGGING_LEVEL"); val != "" {
		config.Logging.Level = val
	}
	if val := os.Getenv("OLLA_LOGGING_FORMAT"); val != "" {
		config.Logging.Format = val
	}

	if val := os.Getenv("OLLA_SHOW_NERD_STATS"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Engineering.ShowNerdStats = enabled
		}
	}
	if val := os.Getenv("OLLA_MODEL_REGISTRY_TYPE"); val != "" {
		config.ModelRegistry.Type = val
	}

	// Model unification configuration
	if val := os.Getenv("OLLA_MODEL_UNIFIER_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.ModelRegistry.EnableUnifier = enabled
			config.ModelRegistry.Unification.Enabled = enabled
		}
	}
	if val := os.Getenv("OLLA_MODEL_UNIFIER_CACHE_TTL"); val != "" {
		if ttl, err := time.ParseDuration(val); err == nil {
			config.ModelRegistry.Unification.CacheTTL = ttl
		}
	}
}

// parseByteSize parses human-readable byte sizes like "100MB", "1GB"
// Uses binary units (1KB = 1024 bytes) for consistency with memory/storage
func parseByteSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty byte size")
	}

	s = strings.TrimSpace(s)

	size, err := units.RAMInBytes(s)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size format: %s", s)
	}

	return size, nil
}
