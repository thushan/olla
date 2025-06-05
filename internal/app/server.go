package app

import (
	"errors"
	"github.com/docker/go-units"
	"github.com/thushan/olla/internal/core/constants"
	"net/http"
	"strings"
)

const (
	ContentTypeJSON   = "application/json"
	ContentTypeText   = "text/plain"
	ContentTypeHeader = "Content-Type"
)

func (a *Application) startWebServer() {
	configServer := a.Config.Server

	a.logger.Info("Starting Olla Server...", "host", configServer.Host, "port", configServer.Port,
		"read_timeout", configServer.ReadTimeout, "write_timeout", configServer.WriteTimeout)

	if configServer.WriteTimeout > 0 {
		a.logger.Warn("Write timeout is set, this may cause issues with long-running requests. (default: 0s)", "write_timeout", configServer.WriteTimeout)
	}

	if configServer.RequestLimits.MaxBodySize > 0 || configServer.RequestLimits.MaxHeaderSize > 0 {
		a.logger.Info("Request size limits enabled",
			"max_body_size", units.HumanSize(float64(configServer.RequestLimits.MaxBodySize)),
			"max_header_size", units.HumanSize(float64(configServer.RequestLimits.MaxHeaderSize)))
	}

	if configServer.RateLimits.GlobalRequestsPerMinute > 0 || configServer.RateLimits.PerIPRequestsPerMinute > 0 {
		a.logger.Info("Rate limiting enabled",
			"global_limit", configServer.RateLimits.GlobalRequestsPerMinute,
			"per_ip_limit", configServer.RateLimits.PerIPRequestsPerMinute,
			"burst_size", configServer.RateLimits.BurstSize,
			"health_limit", configServer.RateLimits.HealthRequestsPerMinute,
			"trust_proxy", configServer.RateLimits.TrustProxyHeaders)
	}

	if configServer.RateLimits.TrustProxyHeaders && len(configServer.RateLimits.TrustedProxyCIDRs) > 0 {
		cidrsStr := strings.Join(configServer.RateLimits.TrustedProxyCIDRs, ", ")
		a.logger.Info("Configured Trusted Proxy CIDRS", "cidrs", cidrsStr)
	}

	mux := http.NewServeMux()

	a.registerRoutes()
	a.routeRegistry.WireUpWithSecurityChain(mux, a.securityAdapters)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP server error", "error", err)
			a.errCh <- err
		}
	}()

	a.server.Handler = mux
	a.logger.Info("Started Olla Server", "bind", a.server.Addr)
}

func (a *Application) registerRoutes() {
	/*
	 /olla/proxy => Standard load balancing
	 /olla/model => Model-aware routing
	 /olla/route => Direct endpoint routing
	*/
	a.routeRegistry.RegisterProxyRoute("/olla/", a.proxyHandler, "Ollama API proxy endpoint (default)", "POST")
	a.routeRegistry.RegisterProxyRoute("/proxy/", a.proxyHandler, "Ollama API proxy endpoint (mirror)", "POST") // Sherpa compatibility
	a.routeRegistry.RegisterWithMethod(constants.DefaultHealthCheckEndpoint, a.healthHandler, "Health check endpoint", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status", a.statusHandler, "Endpoint status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/process", a.processStatsHandler, "Process status", "GET")
	a.routeRegistry.RegisterWithMethod("/version", a.versionHandler, "Olla version information", "GET")
}
