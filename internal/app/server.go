package app

import (
	"errors"
	"github.com/docker/go-units"
	"net/http"
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
		// m,aybe make this a debug log?
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
			"trust_proxy", configServer.RateLimits.IPExtractionTrustProxy)
	}

	mux := http.NewServeMux()

	sizeLimiter := NewRequestSizeLimiter(configServer.RequestLimits, a.logger)
	rateLimiter := NewRateLimiter(configServer.RateLimits, a.logger)

	a.rateLimiter = rateLimiter

	a.registerRoutes()
	a.registry.WireUpWithMiddleware(mux, sizeLimiter, rateLimiter)

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
	// Register the main proxy handler for Ollama API
	// We need to append a trailing slash to the path to avoid issues with path matching
	a.registry.RegisterProxyRoute("/proxy/", a.proxyHandler, "Ollama API proxy endpoint (default)", "POST")
	a.registry.RegisterProxyRoute("/ma/", a.proxyHandler, "Ollama API proxy endpoint (mirror)", "POST")
	// a.registry.RegisterWithMethod("/", a.proxyHandler, "Ollama API proxy endpoint (mirror)", "POST")
	a.registry.RegisterWithMethod("/internal/health", a.healthHandler, "Health check endpoint", "GET")
	a.registry.RegisterWithMethod("/internal/status", a.statusHandler, "Endpoint status", "GET")
	a.registry.RegisterWithMethod("/internal/process", a.processStatsHandler, "Process status", "GET")
}
