package proxy

import (
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

type Configuration struct {
	ProxyPrefix         string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	StreamBufferSize    int
}

func (c *Configuration) GetProxyPrefix() string {
	return c.ProxyPrefix
}

func (c *Configuration) GetConnectionTimeout() time.Duration {
	return c.ConnectionTimeout
}

func (c *Configuration) GetConnectionKeepAlive() time.Duration {
	return c.ConnectionKeepAlive
}

func (c *Configuration) GetResponseTimeout() time.Duration {
	return c.ResponseTimeout
}

func (c *Configuration) GetReadTimeout() time.Duration {
	return c.ReadTimeout
}

func (c *Configuration) GetStreamBufferSize() int {
	return c.StreamBufferSize
}

// Compile-time check to ensure both configurations implement the interface
// neat little hack from https://stackoverflow.com/a/10499051
var (
	_ ports.ProxyConfiguration = (*Configuration)(nil)
)
