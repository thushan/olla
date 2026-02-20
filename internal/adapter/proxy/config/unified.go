package config

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
)

// Default values for proxy settings
const (
	DefaultReadTimeout      = 60 * time.Second
	DefaultStreamBufferSize = 8 * 1024
	DefaultTimeout          = 60 * time.Second
	DefaultKeepAlive        = 60 * time.Second

	// Olla-specific defaults for high-performance
	OllaDefaultStreamBufferSize    = 64 * 1024 // Larger buffer for better streaming performance
	OllaDefaultMaxIdleConns        = 100
	OllaDefaultMaxConnsPerHost     = 50
	OllaDefaultMaxIdleConnsPerHost = 25 // Half of MaxConnsPerHost; idle slots rarely need to match total capacity
	OllaDefaultIdleConnTimeout     = 90 * time.Second
	// Olla uses 30s timeouts for faster failure detection in AI workloads
	OllaDefaultTimeout     = 30 * time.Second
	OllaDefaultKeepAlive   = 30 * time.Second
	OllaDefaultReadTimeout = 30 * time.Second
)

// ProxyConfig defines the interface for all proxy configurations
type ProxyConfig interface {
	// Core configuration getters
	GetProxyProfile() string
	GetProxyPrefix() string
	GetConnectionTimeout() time.Duration
	GetConnectionKeepAlive() time.Duration
	GetResponseTimeout() time.Duration
	GetReadTimeout() time.Duration
	GetStreamBufferSize() int

	// Validation
	Validate() error
}

// BaseProxyConfig contains common configuration fields for all proxy implementations
type BaseProxyConfig struct {
	ProxyPrefix         string
	Profile             string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	StreamBufferSize    int
}

// GetProxyProfile returns the proxy profile, defaulting to "auto" if not set
func (c *BaseProxyConfig) GetProxyProfile() string {
	if c.Profile == "" {
		return constants.ConfigurationProxyProfileAuto
	}
	return c.Profile
}

// GetProxyPrefix returns the proxy prefix, defaulting to "/olla/" if not set
func (c *BaseProxyConfig) GetProxyPrefix() string {
	if c.ProxyPrefix == "" {
		return "/olla/"
	}
	return c.ProxyPrefix
}

// GetConnectionTimeout returns the connection timeout, defaulting to DefaultTimeout
func (c *BaseProxyConfig) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return DefaultTimeout
	}
	return c.ConnectionTimeout
}

// GetConnectionKeepAlive returns the keep-alive duration, defaulting to DefaultKeepAlive
func (c *BaseProxyConfig) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return DefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

// GetResponseTimeout returns the response timeout
func (c *BaseProxyConfig) GetResponseTimeout() time.Duration {
	return c.ResponseTimeout
}

// GetReadTimeout returns the read timeout, defaulting to DefaultReadTimeout
func (c *BaseProxyConfig) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return c.ReadTimeout
}

// GetStreamBufferSize returns the stream buffer size, defaulting to DefaultStreamBufferSize
func (c *BaseProxyConfig) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return DefaultStreamBufferSize
	}
	return c.StreamBufferSize
}

// Validate performs basic validation on the configuration
func (c *BaseProxyConfig) Validate() error {
	return nil
}

// SherpaConfig extends BaseProxyConfig with Sherpa-specific configuration
type SherpaConfig struct {
	BaseProxyConfig
}

// OllaConfig extends BaseProxyConfig with Olla-specific configuration
type OllaConfig struct {
	BaseProxyConfig

	// Olla-specific fields for advanced connection pooling
	IdleConnTimeout     time.Duration
	MaxIdleConns        int
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
}

// GetStreamBufferSize returns the stream buffer size, defaulting to OllaDefaultStreamBufferSize for better performance
func (c *OllaConfig) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return OllaDefaultStreamBufferSize
	}
	return c.StreamBufferSize
}

// GetIdleConnTimeout returns the idle connection timeout, defaulting to OllaDefaultIdleConnTimeout
func (c *OllaConfig) GetIdleConnTimeout() time.Duration {
	if c.IdleConnTimeout == 0 {
		return OllaDefaultIdleConnTimeout
	}
	return c.IdleConnTimeout
}

// GetMaxIdleConns returns the maximum idle connections, defaulting to OllaDefaultMaxIdleConns
func (c *OllaConfig) GetMaxIdleConns() int {
	if c.MaxIdleConns == 0 {
		return OllaDefaultMaxIdleConns
	}
	return c.MaxIdleConns
}

// GetMaxConnsPerHost returns the maximum connections per host, defaulting to OllaDefaultMaxConnsPerHost
func (c *OllaConfig) GetMaxConnsPerHost() int {
	if c.MaxConnsPerHost == 0 {
		return OllaDefaultMaxConnsPerHost
	}
	return c.MaxConnsPerHost
}

// GetMaxIdleConnsPerHost returns the maximum idle connections per host, defaulting to OllaDefaultMaxIdleConnsPerHost
func (c *OllaConfig) GetMaxIdleConnsPerHost() int {
	if c.MaxIdleConnsPerHost == 0 {
		return OllaDefaultMaxIdleConnsPerHost
	}
	return c.MaxIdleConnsPerHost
}

// GetConnectionTimeout returns the connection timeout, defaulting to OllaDefaultTimeout (30s for faster failure detection)
func (c *OllaConfig) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return OllaDefaultTimeout
	}
	return c.ConnectionTimeout
}

// GetConnectionKeepAlive returns the keep-alive duration, defaulting to OllaDefaultKeepAlive (30s for AI workloads)
func (c *OllaConfig) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return OllaDefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

// GetReadTimeout returns the read timeout, defaulting to OllaDefaultReadTimeout (30s for faster error detection)
func (c *OllaConfig) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return OllaDefaultReadTimeout
	}
	return c.ReadTimeout
}
