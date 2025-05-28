package app

import (
	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"time"
)

const (
	DefaultCOnnectionTimeout   = 30 * time.Second
	DefaultConnectionKeepAlive = 30 * time.Second
	DefaultResponseTimeout     = 600 * time.Second
	DefaultReadTimeout         = 300 * time.Second
	DefaultLoadBalancer        = "priority"
)

func DefaultProxyConfiguration() *proxy.Configuration {
	return &proxy.Configuration{
		ConnectionTimeout:   DefaultCOnnectionTimeout,
		ConnectionKeepAlive: DefaultConnectionKeepAlive,
		ResponseTimeout:     DefaultResponseTimeout,
		ReadTimeout:         DefaultReadTimeout,
		ProxyPrefix:         constants.ProxyPathPrefix,
	}
}

func updateProxyConfiguration(config *config.Config) *proxy.Configuration {
	return &proxy.Configuration{
		ConnectionTimeout:   config.Proxy.ConnectionTimeout,
		ConnectionKeepAlive: DefaultConnectionKeepAlive,
		ResponseTimeout:     config.Proxy.ResponseTimeout,
		ReadTimeout:         config.Proxy.ReadTimeout,
		ProxyPrefix:         constants.ProxyPathPrefix,
	}
}