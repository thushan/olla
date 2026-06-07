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

	// DefaultReadHeaderTimeout guards the inbound server against Slowloris-style
	// attacks where a client opens a connection and trickles headers indefinitely.
	// 10 s is enough for any legitimate client to send its headers; backends that
	// are slow to respond are covered by ConnectionTimeout instead.
	DefaultReadHeaderTimeout = 10 * time.Second
)

func updateProxyConfiguration(config *config.Config) *proxy.Configuration {
	keepAlive := config.Proxy.ConnectionKeepAlive
	if keepAlive == 0 {
		keepAlive = DefaultConnectionKeepAlive
	}
	return &proxy.Configuration{
		ConnectionTimeout:     config.Proxy.ConnectionTimeout,
		ConnectionKeepAlive:   keepAlive,
		ResponseTimeout:       config.Proxy.ResponseTimeout,
		ReadTimeout:           config.Proxy.ReadTimeout,
		ResponseHeaderTimeout: config.Proxy.ResponseHeaderTimeout,
		TLSHandshakeTimeout:   config.Proxy.TLSHandshakeTimeout,
		ProxyPrefix:           constants.ContextRoutePrefixKey,
		StreamBufferSize:      getStreamBufferSize(config),
	}
}
func getStreamBufferSize(config *config.Config) int {
	if config.Proxy.StreamBufferSize > 0 {
		return config.Proxy.StreamBufferSize
	}
	return DefaultStreamBufferSize
}
