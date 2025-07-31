package app

import (
	"time"

	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

const (
	DefaultConnectionTimeout   = 30 * time.Second
	DefaultConnectionKeepAlive = 30 * time.Second
	DefaultResponseTimeout     = 600 * time.Second
	DefaultReadTimeout         = 300 * time.Second
	DefaultLoadBalancer        = "priority"
	DefaultStreamBufferSize    = 8 * 1024 // 8KB
)

func updateProxyConfiguration(config *config.Config) *proxy.Configuration {
	return &proxy.Configuration{
		ConnectionTimeout:   config.Proxy.ConnectionTimeout,
		ConnectionKeepAlive: DefaultConnectionKeepAlive,
		ResponseTimeout:     config.Proxy.ResponseTimeout,
		ReadTimeout:         config.Proxy.ReadTimeout,
		ProxyPrefix:         constants.ContextRoutePrefixKey,
		StreamBufferSize:    getStreamBufferSize(config),
	}
}
func getStreamBufferSize(config *config.Config) int {
	if config.Proxy.StreamBufferSize > 0 {
		return config.Proxy.StreamBufferSize
	}
	return DefaultStreamBufferSize
}
