package proxy

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/ports"
)

type Configuration struct {
	ProxyPrefix         string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	StreamBufferSize    int

	// Olla-specific fields for advanced connection pooling
	IdleConnTimeout time.Duration
	MaxIdleConns    int
	MaxConnsPerHost int
}

func (c *Configuration) GetProxyPrefix() string {
	if c.ProxyPrefix == "" {
		return constants.ProxyPathPrefix
	}
	return c.ProxyPrefix
}

func (c *Configuration) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return DefaultTimeout
	}
	return c.ConnectionTimeout
}

func (c *Configuration) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return DefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

func (c *Configuration) GetResponseTimeout() time.Duration {
	if c.ResponseTimeout == 0 {
		return 0 // no timeout by default
	}
	return c.ResponseTimeout
}

func (c *Configuration) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return c.ReadTimeout
}

func (c *Configuration) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return DefaultStreamBufferSize
	}
	return c.StreamBufferSize
}

func (c *Configuration) GetIdleConnTimeout() time.Duration {
	if c.IdleConnTimeout == 0 {
		return OllaDefaultIdleConnTimeout
	}
	return c.IdleConnTimeout
}

func (c *Configuration) GetMaxIdleConns() int {
	if c.MaxIdleConns == 0 {
		return OllaDefaultMaxIdleConns
	}
	return c.MaxIdleConns
}

func (c *Configuration) GetMaxConnsPerHost() int {
	if c.MaxConnsPerHost == 0 {
		return OllaDefaultMaxConnsPerHost
	}
	return c.MaxConnsPerHost
}

// Compile-time check to ensure both configurations implement the interface
// neat little hack from https://stackoverflow.com/a/10499051
var (
	_ ports.ProxyConfiguration = (*Configuration)(nil)
)
