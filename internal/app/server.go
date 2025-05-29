package app

import (
	"errors"
	"net/http"
)

const (
	ContentTypeJSON   = "application/json"
	ContentTypeText   = "text/plain"
	ContentTypeHeader = "Content-Type"
)

func (a *Application) startWebServer() {
	configServer := a.Config.Server

	a.logger.Info("Starting WebServer...", "host", configServer.Host, "port", configServer.Port,
		"read_timeout", configServer.ReadTimeout, "write_timeout", configServer.WriteTimeout)

	if configServer.WriteTimeout > 0 {
		a.logger.Warn("Write timeout is set, this may cause issues with long-running requests. (default: 0s)", "write_timeout", configServer.WriteTimeout)
	}

	mux := http.NewServeMux()

	a.registerRoutes()
	a.registry.WireUp(mux)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP server error", "error", err)
			a.errCh <- err
		}
	}()

	a.server.Handler = mux
	a.logger.Info("Started WebServer", "bind", a.server.Addr)
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
