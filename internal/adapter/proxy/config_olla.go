package proxy

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
)

type OllaConfiguration struct {
	ProxyPrefix         string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	IdleConnTimeout     time.Duration
	StreamBufferSize    int
	MaxIdleConns        int
	MaxConnsPerHost     int
}

func (c *OllaConfiguration) GetProxyPrefix() string {
	if c.ProxyPrefix == "" {
		return constants.ProxyPathPrefix
	}
	return c.ProxyPrefix
}

func (c *OllaConfiguration) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return DefaultTimeout
	}
	return c.ConnectionTimeout
}

func (c *OllaConfiguration) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return DefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

func (c *OllaConfiguration) GetResponseTimeout() time.Duration {
	if c.ResponseTimeout == 0 {
		return 0 // no timeout by default
	}
	return c.ResponseTimeout
}

func (c *OllaConfiguration) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return c.ReadTimeout
}

func (c *OllaConfiguration) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return DefaultStreamBufferSize
	}
	return c.StreamBufferSize
}
