package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/docker/go-units"
	"github.com/thushan/olla/internal/core/constants"
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

	if configServer.RequestLogging {
		a.server.Handler = a.loggingMiddleware(mux)
	} else {
		a.server.Handler = mux
	}

	a.logger.Info("Started Olla Server", "bind", a.server.Addr)
}
func (a *Application) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"query", r.URL.RawQuery,
			"request_uri", r.RequestURI,
			"content_type", r.Header.Get(ContentTypeHeader),
			"content_length", r.ContentLength,
			"host", r.Host,
			"referer", r.Referer(),
			"user_agent", r.UserAgent())
		next.ServeHTTP(w, r)
	})
}
func (a *Application) registerRoutes() {
	/*
	 /olla/proxy => Standard load balancing
	 /olla/model => Model-aware routing
	 /olla/route => Direct endpoint routing
	*/
	a.routeRegistry.RegisterWithMethod(constants.DefaultHealthCheckEndpoint, a.healthHandler, "Health check endpoint", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status", a.statusHandler, "Endpoint status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status/endpoints", a.endpointsStatusHandler, "Endpoints status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status/models", a.modelsStatusHandler, "Models status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/stats/models", a.modelStatsHandler, "Model statistics", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/process", a.processStatsHandler, "Process status", "GET")
	a.routeRegistry.RegisterWithMethod("/version", a.versionHandler, "Olla version information", "GET")

	// Unified models endpoints
	a.routeRegistry.RegisterWithMethod("/olla/models", a.unifiedModelsHandler, "Unified models listing with filtering", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/models/", a.unifiedModelByAliasHandler, "Get unified model by ID or alias", "GET")

	// Sherpa / Scout Proxy behaviour
	a.routeRegistry.RegisterProxyRoute("/olla/proxy/", a.proxyHandler, "Olla API proxy endpoint (sherpa)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/proxy/v1/models", a.openaiModelsHandler, "OpenAI-compatible models", "GET")

	// Ollama endpoints (intercept specific ones, proxy the rest)
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/tags", a.ollamaModelsHandler, "Ollama models listing", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/v1/models", a.ollamaOpenAIModelsHandler, "Ollama models (OpenAI format)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/list", a.ollamaRunningModelsHandler, "Ollama running models", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/show", a.ollamaModelShowHandler, "Ollama model details", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/pull", a.unsupportedModelManagementHandler, "Model pull (unsupported)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/push", a.unsupportedModelManagementHandler, "Model push (unsupported)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/create", a.unsupportedModelManagementHandler, "Model create (unsupported)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/copy", a.unsupportedModelManagementHandler, "Model copy (unsupported)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/ollama/api/delete", a.unsupportedModelManagementHandler, "Model delete (unsupported)", "DELETE")
	a.routeRegistry.RegisterProxyRoute("/olla/ollama/", a.providerProxyHandler, "Ollama proxy", "")

	// LM Studio endpoints (both OpenAI-compatible and beta API)
	// Support multiple URL variations: lmstudio, lm-studio, lm_studio
	a.routeRegistry.RegisterWithMethod("/olla/lmstudio/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (OpenAI format)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lmstudio/api/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (OpenAI format alt path)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lmstudio/api/v0/models", a.lmstudioEnhancedModelsHandler, "LM Studio enhanced models", "GET")
	a.routeRegistry.RegisterProxyRoute("/olla/lmstudio/", a.providerProxyHandler, "LM Studio proxy", "")

	// Register alternative URL patterns for lm-studio
	a.routeRegistry.RegisterWithMethod("/olla/lm-studio/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (hyphenated)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lm-studio/api/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (hyphenated alt)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lm-studio/api/v0/models", a.lmstudioEnhancedModelsHandler, "LM Studio enhanced (hyphenated)", "GET")
	a.routeRegistry.RegisterProxyRoute("/olla/lm-studio/", a.providerProxyHandler, "LM Studio proxy (hyphenated)", "")

	// Register underscore variant
	a.routeRegistry.RegisterWithMethod("/olla/lm_studio/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (underscore)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lm_studio/api/v1/models", a.lmstudioOpenAIModelsHandler, "LM Studio models (underscore alt)", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/lm_studio/api/v0/models", a.lmstudioEnhancedModelsHandler, "LM Studio enhanced (underscore)", "GET")
	a.routeRegistry.RegisterProxyRoute("/olla/lm_studio/", a.providerProxyHandler, "LM Studio proxy (underscore)", "")

	// OpenAI-compatible endpoints (LocalAI, text-generation-webui, etc)
	a.routeRegistry.RegisterWithMethod("/olla/openai/v1/models", a.openaiModelsHandler, "OpenAI-compatible models", "GET")
	a.routeRegistry.RegisterProxyRoute("/olla/openai/", a.providerProxyHandler, "OpenAI-compatible proxy", "")

	/*
		// vLLM endpoints
		a.routeRegistry.RegisterWithMethod("/olla/vllm/v1/models", a.vllmModelsHandler, "vLLM models", "GET")
		a.routeRegistry.RegisterProxyRoute("/olla/vllm/", a.providerProxyHandler, "vLLM proxy", "")
	*/

}
