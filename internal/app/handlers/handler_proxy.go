package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

type proxyRequest struct {
	requestLogger            logger.StyledLogger
	stats                    *ports.RequestStats
	profile                  *domain.RequestProfile
	clientIP                 string
	targetPath               string
	model                    string
	contentType              string
	method                   string
	path                     string
	query                    string
	userAgent                string
	translatorMode           constants.TranslatorMode
	translatorFallbackReason string
	// stickyOutcome, stickySource, and sessionID are populated after endpoint
	// selection so the routing outcome is visible in completed-request log lines.
	// sessionID must only appear at DEBUG because client-supplied IDs are user data.
	stickyOutcome string
	stickySource  string
	sessionID     string
	contentLength int64
	hadError      bool
	isStreaming   bool
}

func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	pr := a.initializeProxyRequest(r)

	ctx, r := a.setupRequestContext(r, pr.stats)

	a.analyzeRequest(ctx, r, pr)

	// Sticky session key must be computed after analyzeRequest so the model
	// name is available; inject into context before endpoint selection.
	// The outcome pointer is stored in context; the proxy engine reads it before WriteHeader.
	if a.Config.Proxy.StickySessions.Enabled {
		ctx, r, _ = a.injectStickyKey(ctx, r, pr.model)
	}

	endpoints, err := a.getCompatibleEndpoints(ctx, pr)
	if err != nil {
		a.handleEndpointError(w, pr, err)
		return
	}

	a.dispatchToEndpoints(ctx, w, r, pr, endpoints, "")
}

// dispatchToEndpoints is the shared tail of proxyHandler and providerProxyHandler:
// forward to the proxy engine when endpoints were selected, or fail fast with the
// correct status when selection produced none. Consolidating this here means a
// routing rejection (#191) is honoured identically regardless of which route the
// request came in on, instead of each handler drifting its own empty-endpoint handling.
func (a *Application) dispatchToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, pr *proxyRequest, endpoints []*domain.Endpoint, providerType string) {
	if len(endpoints) == 0 {
		a.writeNoRoutableEndpoints(w, r, pr, providerType)
		return
	}

	// Strip the route prefix before forwarding to the backend.
	// Without this, BuildTargetURL receives the full /olla/proxy/... path and
	// GetProxyPrefix() returns "route_prefix" (a context key name, not a URL path),
	// so its StripPrefix is a no-op.
	r.URL.Path = pr.targetPath

	a.logRequestStart(pr, len(endpoints))

	// Wrap w once so both the proxy engine and handleProxyError observe the same
	// commit signal. RetryHandler.ExecuteWithRetry wraps it again internally, which
	// is harmless: both trackers read the same underlying writes, and Unwrap() lets
	// http.NewResponseController chain through both layers to reach the real flusher.
	tracker := core.NewResponseStartedWriter(w)

	err := a.executeProxyRequest(ctx, tracker, r, endpoints, pr)
	pr.captureStickyOutcome(ctx, r)
	a.logRequestResult(pr, err)

	if err != nil {
		a.handleProxyError(tracker, err)
	}
}

// writeNoRoutableEndpoints short-circuits a request when endpoint selection produced
// zero candidates, instead of letting it fall through to the proxy engine (which would
// either proxy to a compatible-but-wrong backend, or return a generic 502/404 that hides
// the actual routing verdict). A rejection routing decision takes priority: it carries
// the precise status code and reason (strict model_not_found gives 404). Without a
// decision we keep the historical per-route defaults.
func (a *Application) writeNoRoutableEndpoints(w http.ResponseWriter, r *http.Request, pr *proxyRequest, providerType string) {
	var decision *domain.ModelRoutingDecision
	if pr.profile != nil {
		decision = pr.profile.RoutingDecision
	}

	var status int
	var reason string

	switch {
	case decision != nil && decision.StatusCode >= http.StatusBadRequest:
		status = decision.StatusCode
		reason = decision.Reason
		pr.stats.RoutingDecision = decision
	case providerType != "":
		// no decision was recorded (e.g. modelRegistry unset), so preserve the
		// precise provider-route message rather than a vague generic one.
		status = http.StatusNotFound
		reason = fmt.Sprintf("No %s endpoints available", providerType)
	default:
		status = http.StatusServiceUnavailable
		reason = "no healthy endpoints available"
	}

	// Preserve normal request telemetry (client_ip, model, duration, routing fields)
	// even though we're short-circuiting before the proxy engine ever runs. Logged as
	// an explicit rejection rather than logRequestResult's "completed", so a fail-fast
	// 404/503 is never mistaken for a successful proxy in the logs (#191).
	a.logRequestStart(pr, 0)
	pr.captureStickyOutcome(r.Context(), r)
	a.logRequestRejected(pr, status)

	// Headers must be set before http.Error, which calls WriteHeader.
	a.setStickyResponseHeadersFromRequest(w, r)
	a.setRoutingDecisionHeaders(w, decision)

	http.Error(w, reason, status)
}

// setRoutingDecisionHeaders writes the X-Olla-Routing-* observability headers from
// a routing decision. Shared by writeNoRoutableEndpoints and the translation route's
// zero-endpoint rejection so a decision-aware rejection carries the same headers
// regardless of which route produced it (#191).
func (a *Application) setRoutingDecisionHeaders(w http.ResponseWriter, decision *domain.ModelRoutingDecision) {
	if decision == nil {
		return
	}
	w.Header().Set(constants.HeaderXOllaRoutingStrategy, decision.Strategy)
	w.Header().Set(constants.HeaderXOllaRoutingDecision, decision.Action)
	if decision.Reason != "" {
		w.Header().Set(constants.HeaderXOllaRoutingReason, decision.Reason)
	}
}

func (a *Application) initializeProxyRequest(r *http.Request) *proxyRequest {
	// get the requestID from the middleware context first
	requestID := ""
	if id, ok := r.Context().Value(middleware.RequestIDKey).(string); ok {
		requestID = id
	}

	// fallback to generating a new one otherwise
	if requestID == "" {
		requestID = util.GenerateRequestID()
	}

	stats := &ports.RequestStats{
		RequestID: requestID,
		StartTime: time.Now(),
	}

	return &proxyRequest{
		stats:         stats,
		requestLogger: a.logger.WithRequestID(stats.RequestID),
		contentType:   r.Header.Get(constants.HeaderContentType),
		method:        r.Method,
		path:          r.URL.Path,
		query:         r.URL.RawQuery,
		contentLength: r.ContentLength,
		userAgent:     r.UserAgent(),
	}
}

func (a *Application) setupRequestContext(r *http.Request, stats *ports.RequestStats) (context.Context, *http.Request) {
	ctx := context.WithValue(r.Context(), constants.ContextRequestIdKey, stats.RequestID)
	ctx = context.WithValue(ctx, constants.ContextRequestTimeKey, stats.StartTime)
	return ctx, r.WithContext(ctx)
}

func (a *Application) analyzeRequest(ctx context.Context, r *http.Request, pr *proxyRequest) {
	pr.requestLogger.Debug("Proxy handler called", "path", r.URL.Path, "method", r.Method)

	rl := a.Config.Server.RateLimits
	pr.clientIP = util.GetClientIP(r, rl.TrustProxyHeaders, rl.TrustedProxyCIDRsParsed)

	pathResolutionStart := time.Now()
	pr.targetPath = a.stripRoutePrefix(ctx, r.URL.Path)

	// inspector chain figures out which endpoints can handle this request (ollama vs openai)
	// and extracts model requirements. failures here are non-fatal - we'll spray and pray
	profile, err := a.inspectorChain.Inspect(ctx, r, pr.targetPath)
	if err != nil {
		pr.requestLogger.Warn("Request inspection failed, continuing with all endpoints", "error", err)
	}
	pr.profile = profile

	if profile != nil && profile.ModelName != "" {
		pr.model = profile.ModelName
		pr.stats.Model = pr.model
	}

	pr.stats.PathResolutionMs = time.Since(pathResolutionStart).Milliseconds()
}

// captureStickyOutcome copies the sticky session result and the client-supplied session id
// into pr so logRequestResult surfaces them. Every handler that logs a completed request
// must call this after endpoint selection; without it, provider routes silently drop the
// fields from their "Request completed" log line (#178, regression from #139).
// The outcome pointer is allocated in injectStickyKeyWithBody and filled by the balancer
// wrapper before Select() returns, so no lock is needed here.
func (pr *proxyRequest) captureStickyOutcome(ctx context.Context, r *http.Request) {
	if outcome, ok := ctx.Value(constants.ContextStickyOutcomeKey).(*balancer.StickyOutcome); ok && outcome != nil {
		pr.stickyOutcome = outcome.Result
		pr.stickySource = outcome.Source
	}
	pr.sessionID = r.Header.Get(constants.HeaderXOllaSessionID)
}

// injectStickyKey computes the affinity key for this request and injects it into the context.
// It reads up to prefix_hash_bytes of the request body (for prefix hashing) then restores the
// body so downstream handlers see it intact. The StickyOutcome pointer lets the wrapper report
// its hit/miss/repin decision back to the handler without an extra context lookup.
func (a *Application) injectStickyKey(ctx context.Context, r *http.Request, modelName string) (context.Context, *http.Request, *balancer.StickyOutcome) {
	cfg := a.Config.Proxy.StickySessions

	// Read a small prefix of the body for prefix_hash; restore it afterwards.
	// We cap at cfg.PrefixHashBytes+1 to handle the case where the body is exactly
	// that length without allocating an oversized buffer.
	var bodySnap []byte
	if r.Body != nil && r.ContentLength != 0 {
		limit := cfg.PrefixHashBytes
		if limit <= 0 {
			limit = 512
		}
		snap, readErr := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
		// Restore any bytes already consumed, even on partial-read error, so the
		// downstream proxy always sees a complete body.
		if len(snap) > 0 {
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(snap), r.Body))
		}
		if readErr == nil {
			bodySnap = snap
		}
	}

	return a.injectStickyKeyWithBody(ctx, r, modelName, bodySnap)
}

// injectStickyKeyWithBody is the core sticky key injection path when the body has
// already been buffered by the caller (e.g. the translation handler reads the full
// body to extract the model name before we reach this point). Passing the bytes in
// avoids a second read/restore cycle on the same reader.
func (a *Application) injectStickyKeyWithBody(ctx context.Context, r *http.Request, modelName string, body []byte) (context.Context, *http.Request, *balancer.StickyOutcome) {
	cfg := a.Config.Proxy.StickySessions
	stickyKey, stickySource := balancer.ComputeStickyKey(r, modelName, cfg, body)

	outcome := &balancer.StickyOutcome{}
	ctx = context.WithValue(ctx, constants.ContextStickyKeyKey, stickyKey)
	ctx = context.WithValue(ctx, constants.ContextStickyKeySourceKey, stickySource)
	ctx = context.WithValue(ctx, constants.ContextStickyOutcomeKey, outcome)
	r = r.WithContext(ctx)

	return ctx, r, outcome
}

// setStickyResponseHeadersFromRequest reads the StickyOutcome from the request context
// and writes the sticky session headers. Used by sub-handlers that have *http.Request
// but not the outcome pointer directly. Must be called before w.WriteHeader().
func (a *Application) setStickyResponseHeadersFromRequest(w http.ResponseWriter, r *http.Request) {
	outcome, _ := r.Context().Value(constants.ContextStickyOutcomeKey).(*balancer.StickyOutcome)
	a.setStickyResponseHeaders(w, r, outcome)
}

// setStickyResponseHeaders writes sticky session outcome headers so clients can observe
// affinity routing decisions. When the client provided an explicit session ID header,
// we echo it back so stateless clients can track their own session.
func (a *Application) setStickyResponseHeaders(w http.ResponseWriter, r *http.Request, outcome *balancer.StickyOutcome) {
	if outcome == nil {
		return
	}
	if outcome.Result != "" {
		w.Header().Set(constants.HeaderXOllaStickySession, outcome.Result)
	}
	if outcome.Source != "" && outcome.Source != "none" {
		w.Header().Set(constants.HeaderXOllaStickyKeySource, outcome.Source)
	}
	if outcome.Source == "session_header" {
		if sid := r.Header.Get(constants.HeaderXOllaSessionID); sid != "" {
			w.Header().Set(constants.HeaderXOllaSessionID, sid)
		}
	}
}

func (a *Application) getCompatibleEndpoints(ctx context.Context, pr *proxyRequest) ([]*domain.Endpoint, error) {
	endpoints, err := a.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		pr.requestLogger.Error("Failed to get healthy endpoints", "error", err)
		return nil, fmt.Errorf("no healthy endpoints available: %w", err)
	}

	compatibleEndpoints := a.filterEndpointsByProfile(endpoints, pr.profile, pr.requestLogger)

	return compatibleEndpoints, nil
}

func (a *Application) executeProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, pr *proxyRequest) error {
	ctx, r = a.prepareProxyContext(ctx, r, pr)

	return a.proxyService.ProxyRequestToEndpoints(ctx, w, r, endpoints, pr.stats, pr.requestLogger)
}

func (a *Application) logRequestStart(pr *proxyRequest, endpointCount int) {
	// Log essential operational info at INFO level
	logFields := []any{
		"client_ip", pr.clientIP,
		"method", pr.method,
		"path", pr.path,
		"compatible_endpoints", endpointCount,
	}

	// Add user agent if present
	if pr.userAgent != "" {
		logFields = append(logFields, "user_agent", pr.userAgent)
	}

	// Add model if identified
	if pr.model != "" {
		logFields = append(logFields, "model", pr.model)
	}

	// Add content length if it's a POST/PUT with body
	if pr.contentLength > 0 {
		logFields = append(logFields, "content_length", pr.contentLength)
	}

	// translator_mode is only set on translation handler requests
	if pr.translatorMode != "" {
		logFields = append(logFields, "translator_mode", string(pr.translatorMode))
	}

	pr.requestLogger.Info("Request received", logFields...)

	// Log additional details at DEBUG level
	debugFields := []any{
		"target_path", pr.targetPath,
		"path_resolution_ms", pr.stats.PathResolutionMs,
		"query", pr.query,
		"content_type", pr.contentType,
	}

	pr.requestLogger.Debug("Request details", debugFields...)
}

// logRequestRejected records a request that never reached the proxy engine because
// endpoint selection produced no routable target. Kept distinct from logRequestResult's
// "completed"/"failed" outcomes so a fail-fast rejection (e.g. strict model_not_found)
// is surfaced as a rejection rather than a successful completion (#191).
func (a *Application) logRequestRejected(pr *proxyRequest, status int) {
	duration := time.Since(pr.stats.StartTime)

	logFields := []any{
		"client_ip", pr.clientIP,
		"path", pr.path,
		"status", status,
		"duration_ms", duration.Milliseconds(),
	}

	if pr.model != "" {
		logFields = append(logFields, "model", pr.model)
	}

	// routing fields explain why nothing was routable (strategy/action/reason)
	if rd := pr.stats.RoutingDecision; rd != nil {
		if rd.Strategy != "" {
			logFields = append(logFields, "routing_strategy", rd.Strategy)
		}
		if rd.Action != "" {
			logFields = append(logFields, "routing_action", rd.Action)
		}
		if rd.Reason != "" {
			logFields = append(logFields, "routing_reason", rd.Reason)
		}
	}

	pr.requestLogger.Warn("Request rejected", logFields...)
}

func (a *Application) logRequestResult(pr *proxyRequest, err error) {
	duration := time.Since(pr.stats.StartTime)

	if err != nil {
		logFields := a.buildLogFields(pr, duration)
		pr.requestLogger.Error("Request failed", append([]any{"error", err}, logFields...)...)
	} else {
		// Log essential completion info at INFO level
		infoFields := []any{
			"endpoint", pr.stats.EndpointName,
			"duration_ms", duration.Milliseconds(),
			"status", "completed",
		}

		if pr.model != "" {
			infoFields = append(infoFields, "model", pr.model)
		}

		if pr.stats.TotalBytes > 0 {
			infoFields = append(infoFields, "total_bytes", pr.stats.TotalBytes)
		}

		// translator_mode is only set on translation handler requests
		if pr.translatorMode != "" {
			infoFields = append(infoFields, "translator_mode", string(pr.translatorMode))
		}

		// Add provider metrics if available
		if pr.stats.ProviderMetrics != nil {
			pm := pr.stats.ProviderMetrics
			if pm.InputTokens > 0 {
				infoFields = append(infoFields, "input_tokens", pm.InputTokens)
			}
			if pm.OutputTokens > 0 {
				infoFields = append(infoFields, "output_tokens", pm.OutputTokens)
			}
			if pm.TotalTokens > 0 {
				infoFields = append(infoFields, "total_tokens", pm.TotalTokens)
			}
			if pm.TokensPerSecond > 0 {
				infoFields = append(infoFields, "tokens_per_sec", fmt.Sprintf("%.1f", pm.TokensPerSecond))
			}
			if pm.TTFTMs > 0 {
				infoFields = append(infoFields, "ttft_ms", pm.TTFTMs)
			}
			// the model the backend actually ran, which can differ from the requested alias
			if pm.Model != "" {
				infoFields = append(infoFields, "provider_model", pm.Model)
			}
		}

		// routing fields at INFO so operators can see the strategy without enabling DEBUG
		if rd := pr.stats.RoutingDecision; rd != nil {
			if rd.Strategy != "" {
				infoFields = append(infoFields, "routing_strategy", rd.Strategy)
			}
			if rd.Action != "" {
				infoFields = append(infoFields, "routing_action", rd.Action)
			}
			if rd.Reason != "" {
				infoFields = append(infoFields, "routing_reason", rd.Reason)
			}
		}

		// "disabled" carries no signal; omit it to avoid noise in deployments without sticky sessions
		if pr.stickyOutcome != "" && pr.stickyOutcome != constants.StickyOutcomeResultDisabled {
			infoFields = append(infoFields, "sticky_outcome", pr.stickyOutcome)
		}

		// the client-supplied session id, logged alongside the outcome so affinity
		// routing can be traced without enabling DEBUG
		if pr.sessionID != "" {
			infoFields = append(infoFields, "session_id", pr.sessionID)
		}

		// FallbackReasonNone is the empty string (passthrough succeeded); omit it
		// because it would appear on the majority of requests and adds no signal
		if pr.translatorFallbackReason != "" {
			infoFields = append(infoFields, "fallback_reason", pr.translatorFallbackReason)
		}

		pr.requestLogger.Info("Request completed", infoFields...)

		// Log detailed metrics at DEBUG level
		debugFields := a.buildLogFields(pr, duration)
		pr.requestLogger.Debug("Request metrics", debugFields...)
	}
}

func (a *Application) buildLogFields(pr *proxyRequest, duration time.Duration) []any {
	fields := []any{
		"endpoint", pr.stats.EndpointName,
		"model", pr.model,
		"client_ip", pr.clientIP,
		"total_bytes", pr.stats.TotalBytes,
		"duration_ms", duration.Milliseconds(),
		"latency_ms", pr.stats.Latency,
		"request_processing_ms", pr.stats.RequestProcessingMs,
		"backend_response_ms", pr.stats.BackendResponseMs,
		"first_data_ms", pr.stats.FirstDataMs,
		"streaming_ms", pr.stats.StreamingMs,
		"header_processing_ms", pr.stats.HeaderProcessingMs,
		"path_resolution_ms", pr.stats.PathResolutionMs,
		"selection_ms", pr.stats.SelectionMs,
	}

	if pr.stats.EndpointName == "" {
		fields = append(fields, "target_path", pr.targetPath)
	}

	// "none" means no sticky session is active, which adds no useful signal at DEBUG
	if pr.stickySource != "" && pr.stickySource != "none" {
		fields = append(fields, "sticky_source", pr.stickySource)
	}
	// also carried here so failed requests and the detailed metrics line keep
	// the session id (the success path logs it at INFO)
	if pr.sessionID != "" {
		fields = append(fields, "session_id", pr.sessionID)
	}

	// Add provider metrics if available (detailed view)
	if pr.stats.ProviderMetrics != nil {
		pm := pr.stats.ProviderMetrics
		if pm.InputTokens > 0 {
			fields = append(fields, "input_tokens", pm.InputTokens)
		}
		if pm.OutputTokens > 0 {
			fields = append(fields, "output_tokens", pm.OutputTokens)
		}
		if pm.TotalTokens > 0 {
			fields = append(fields, "total_tokens", pm.TotalTokens)
		}
		if pm.TokensPerSecond > 0 {
			fields = append(fields, "tokens_per_sec", pm.TokensPerSecond)
		}
		if pm.TTFTMs > 0 {
			fields = append(fields, "ttft_ms", pm.TTFTMs)
		}
		if pm.PromptMs > 0 {
			fields = append(fields, "prompt_ms", pm.PromptMs)
		}
		if pm.GenerationMs > 0 {
			fields = append(fields, "generation_ms", pm.GenerationMs)
		}
		if pm.Model != "" {
			fields = append(fields, "provider_model", pm.Model)
		}
		if pm.FinishReason != "" {
			fields = append(fields, "finish_reason", pm.FinishReason)
		}
	}

	return fields
}

func (a *Application) handleEndpointError(w http.ResponseWriter, pr *proxyRequest, err error) {
	pr.requestLogger.Error("Failed to get endpoints", "error", err)
	http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusBadGateway)
}

// only send error response if we haven't started streaming yet.
// w.Started() is the real "response already committed" signal: Header().Set does
// not commit anything, only WriteHeader/Write do, so a Content-Type-presence check
// used to let this guard misfire on any handler that set headers before failing.
// This still prevents double-writing a response after a partial stream
// (learned this the hard way when users got html error messages appended to their json).
func (a *Application) handleProxyError(w *core.ResponseStartedWriter, err error) {
	if w.Started() {
		return
	}

	// A chunked (no Content-Length) request body that exceeds the configured
	// cap is caught mid-read by the http.MaxBytesReader SizeValidator.CreateMiddleware
	// installs on r.Body, not by the pre-check that handles known-Content-Length
	// oversize requests. That surfaces here as a generic read error wrapped up
	// through retry.go's body preservation - without this check it would fall
	// through to a misleading 502, when the real cause is the same "body too
	// large" condition the pre-check rejects with 413. Match that response
	// exactly so the client sees one consistent envelope regardless of which
	// path caught the oversize body.
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
}

func (a *Application) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, constants.ContextRoutePrefixKey)
}

// three-stage filtering pipeline that progressively narrows down endpoints.
// starts broad (platform compatibility), then capabilities (vision, embeddings),
// finally specific model availability. each stage falls back gracefully.
func (a *Application) filterEndpointsByProfile(endpoints []*domain.Endpoint, profile *domain.RequestProfile, logger logger.StyledLogger) []*domain.Endpoint {
	var profileFiltered []*domain.Endpoint

	// stage 1: platform compatibility (ollama can't handle openai requests etc)
	if profile == nil || len(profile.SupportedBy) == 0 {
		logger.Debug("No profile filtering applied", "total_endpoints", len(endpoints))
		profileFiltered = endpoints
	} else {
		compatible := make([]*domain.Endpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			// Normalise endpoint type to handle variations (e.g., lmstudio -> lm-studio)
			normalizedType := NormaliseProviderType(endpoint.Type)
			if profile.IsCompatibleWith(normalizedType) {
				compatible = append(compatible, endpoint)
			}
		}

		if len(compatible) == 0 {
			if profile.FailClosedOnNoMatch {
				// Provider-scoped request (e.g. /olla/vllm/) with zero healthy
				// endpoints of that type: return the empty set so the caller's
				// existing no-routable-endpoints path produces a proper 404/503,
				// instead of silently crossing into another provider type.
				logger.Warn("No compatible endpoints found for provider-scoped path, failing closed",
					"path", profile.Path,
					"supported_by", profile.SupportedBy,
					"total_endpoints", len(endpoints))
				return nil
			}
			logger.Warn("No compatible endpoints found for path, falling back to all endpoints",
				"path", profile.Path,
				"supported_by", profile.SupportedBy,
				"total_endpoints", len(endpoints))
			profileFiltered = endpoints
		} else {
			logger.Debug("Filtered endpoints by profile compatibility",
				"path", profile.Path,
				"compatible_count", len(compatible),
				"total_count", len(endpoints),
				"supported_by", profile.SupportedBy)
			profileFiltered = compatible
		}
	}

	// stage 2: capability filtering (vision requests need vision models)
	if profile != nil && profile.ModelCapabilities != nil && a.modelRegistry != nil {
		capabilityFiltered := a.filterEndpointsByCapabilities(profileFiltered, profile, logger)
		if len(capabilityFiltered) > 0 {
			profileFiltered = capabilityFiltered
		}
	}

	// stage 3: specific model filtering using routing strategy
	if profile != nil && profile.ModelName != "" && a.modelRegistry != nil {
		ctx := context.Background()

		// aliases map one name to multiple backend-specific models, so they
		// need dedicated resolution rather than the standard single-model path
		if a.aliasResolver != nil && a.aliasResolver.IsAlias(profile.ModelName) {
			return a.resolveAliasEndpoints(ctx, profile, profileFiltered, logger)
		}

		// use new routing strategy method
		routableEndpoints, decision, err := a.modelRegistry.GetRoutableEndpointsForModel(ctx, profile.ModelName, profileFiltered)

		// store routing decision for headers and metrics
		if decision != nil {
			profile.RoutingDecision = decision
		}

		if err != nil {
			// handle routing errors based on decision
			if decision != nil && decision.StatusCode > 0 {
				logger.Warn("Model routing rejected request",
					"model", profile.ModelName,
					"strategy", decision.Strategy,
					"reason", decision.Reason,
					"status", decision.StatusCode)
				// return empty to trigger appropriate error response
				return []*domain.Endpoint{}
			}

			logger.Warn("Model routing failed, using all compatible endpoints",
				"model", profile.ModelName,
				"error", err)
			return profileFiltered
		}

		logger.Debug("Model routing decision",
			"model", profile.ModelName,
			"strategy", decision.Strategy,
			"action", decision.Action,
			"routable", len(routableEndpoints),
			"compatible", len(profileFiltered))

		return routableEndpoints
	}

	return profileFiltered
}

// resolveAliasEndpoints handles model alias resolution. When a request uses an alias
// model name, this finds all endpoints that serve any of the aliased actual model names
// and stores a rewrite map in the profile so the proxy can replace the model name in
// the request body with the one the selected backend recognises.
func (a *Application) resolveAliasEndpoints(ctx context.Context, profile *domain.RequestProfile, candidates []*domain.Endpoint, logger logger.StyledLogger) []*domain.Endpoint {
	aliasName := profile.ModelName

	// resolve the alias to find endpoint → actual model name mapping
	endpointToModel, err := a.aliasResolver.ResolveEndpoints(ctx, aliasName, a.modelRegistry)
	if err != nil || len(endpointToModel) == 0 {
		logFields := []any{"alias", aliasName}
		if err != nil {
			logFields = append(logFields, "error", err)
		}
		logger.Warn("Model alias resolved to no endpoints, falling back to standard routing",
			logFields...)

		// fall through to standard routing in case the alias name itself is a known model
		return a.routeByAliasName(ctx, aliasName, profile, candidates)
	}

	// filter candidates to only those that have one of the aliased models
	var aliasEndpoints []*domain.Endpoint
	for _, endpoint := range candidates {
		if _, ok := endpointToModel[endpoint.GetURLString()]; ok {
			aliasEndpoints = append(aliasEndpoints, endpoint)
		}
	}

	if len(aliasEndpoints) == 0 {
		logger.Warn("No healthy endpoints found for model alias",
			"alias", aliasName,
			"resolved_endpoints", len(endpointToModel))

		// The alias resolved to real target models, but none of them are on a healthy/
		// compatible candidate. Rather than synthesising a rejection here, consult the
		// routing strategy for the alias name itself, exactly like the "resolved to no
		// endpoints at all" branch above - this is what lets optimistic routing with
		// fallback_behavior: all substitute a different endpoint instead of the request
		// being unconditionally rejected (#191 follow-up). Trade-off accepted: the
		// resulting rejection reason/status is whatever the registry reports for an
		// unknown model (typically model_not_found/404) rather than the alias-specific
		// model_unavailable/503 this branch used to synthesise; consistency with the
		// policy engine wins over status-code precision.
		return a.routeByAliasName(ctx, aliasName, profile, candidates)
	}

	// store the rewrite map in the profile for use during request proxying
	profile.SetInspectionMeta(constants.ContextModelAliasMapKey, endpointToModel)

	// set routing decision so downstream headers/metrics stay consistent
	profile.RoutingDecision = ports.NewRoutingDecision("alias", ports.RoutingActionRouted,
		fmt.Sprintf("alias %q resolved to %d endpoint(s)", aliasName, len(aliasEndpoints)))

	// collect unique actual model names so the log shows what the backends serve,
	// not just the alias name the client requested
	actualModels := make([]string, 0, len(endpointToModel))
	seen := make(map[string]struct{}, len(endpointToModel))
	for _, m := range endpointToModel {
		if _, dup := seen[m]; !dup {
			seen[m] = struct{}{}
			actualModels = append(actualModels, m)
		}
	}

	logger.Info("Model alias resolved",
		"alias", aliasName,
		"matched_endpoints", len(aliasEndpoints),
		"actual_models", actualModels,
		"total_candidates", len(candidates))

	return aliasEndpoints
}

// routeByAliasName is the shared tail for both resolveAliasEndpoints fallback paths:
// alias resolution producing no endpoints at all, and alias resolution producing
// endpoints that don't intersect the healthy/compatible candidate set. Both cases treat
// the alias name as if it were a plain model name and hand the decision to the configured
// routing strategy, rather than the handler synthesising its own rejection - this is what
// lets fallback_behavior: all under optimistic routing substitute a different endpoint
// instead of always rejecting (#191 follow-up). It does NOT set the alias rewrite map:
// any endpoints returned here were not confirmed to serve one of the alias's actual
// target models, so the proxy must forward the original request body unchanged.
func (a *Application) routeByAliasName(ctx context.Context, aliasName string, profile *domain.RequestProfile, candidates []*domain.Endpoint) []*domain.Endpoint {
	routableEndpoints, decision, routeErr := a.modelRegistry.GetRoutableEndpointsForModel(ctx, aliasName, candidates)
	if decision != nil {
		profile.RoutingDecision = decision
	}

	// A rejection must fail fast exactly like the non-alias path in filterEndpointsByProfile.
	// Returning candidates here would silently proxy to a compatible-but-wrong backend and
	// ignore the routing verdict (#191). Keyed on status code rather than the "rejected"
	// action string because writeNoRoutableEndpoints uses the same status-code contract,
	// and not every registry implementation reports rejections as "rejected" -
	// MemoryModelRegistry's base GetRoutableEndpointsForModel uses "no_model"/"no_healthy"
	// with 404/503. Keying on the action string would silently miss those and reintroduce
	// the bug this fix closes.
	if decision != nil && decision.StatusCode >= http.StatusBadRequest {
		return []*domain.Endpoint{}
	}

	if routeErr != nil || len(routableEndpoints) == 0 {
		return candidates
	}
	return routableEndpoints
}

func (a *Application) filterEndpointsByCapabilities(endpoints []*domain.Endpoint, profile *domain.RequestProfile, logger logger.StyledLogger) []*domain.Endpoint {
	if profile.ModelCapabilities == nil {
		return endpoints
	}

	requiredCapabilities := a.extractRequiredCapabilities(profile.ModelCapabilities)
	if len(requiredCapabilities) == 0 {
		return endpoints
	}

	capableModels := a.findCapableModels(requiredCapabilities, logger)
	if len(capableModels) == 0 {
		logger.Warn("No models found with required capabilities",
			"model", profile.ModelName,
			"capabilities", requiredCapabilities)
		return endpoints
	}

	return a.filterEndpointsByCapableModels(endpoints, capableModels, requiredCapabilities, logger)
}

func (a *Application) extractRequiredCapabilities(caps *domain.ModelCapabilities) []string {
	requiredCapabilities := make([]string, 0, 5)

	if caps.VisionUnderstanding {
		requiredCapabilities = append(requiredCapabilities, "vision")
	}
	if caps.FunctionCalling {
		requiredCapabilities = append(requiredCapabilities, "function_calling", "tools")
	}
	if caps.Embeddings {
		requiredCapabilities = append(requiredCapabilities, "embeddings")
	}
	if caps.CodeGeneration {
		requiredCapabilities = append(requiredCapabilities, "code")
	}

	return requiredCapabilities
}

// intersects capability sets to find models that support ALL requested features.
// uses nil return to signal "no capability support" vs empty map for "no matches"
func (a *Application) findCapableModels(requiredCapabilities []string, logger logger.StyledLogger) map[string]bool {
	ctx := context.Background()
	capableModels := make(map[string]bool)
	hasCapabilitySupport := false

	for i, capability := range requiredCapabilities {
		models, err := a.modelRegistry.GetModelsByCapability(ctx, capability)
		if err != nil {
			logger.Warn("Failed to get models by capability",
				"capability", capability,
				"error", err)
			continue
		}

		if len(models) > 0 {
			hasCapabilitySupport = true
		}

		if i == 0 {
			a.addModelsToMap(models, capableModels)
		} else {
			// set intersection - only keep models that have all capabilities
			capableModels = a.intersectModels(models, capableModels)
		}
	}

	// nil means "don't filter", empty map means "no matches found"
	if !hasCapabilitySupport {
		return nil
	}

	return capableModels
}

func (a *Application) addModelsToMap(models []*domain.UnifiedModel, capableModels map[string]bool) {
	for _, model := range models {
		for _, sourceEndpoint := range model.SourceEndpoints {
			capableModels[sourceEndpoint.EndpointURL] = true
		}
	}
}

func (a *Application) intersectModels(models []*domain.UnifiedModel, existingCapableModels map[string]bool) map[string]bool {
	newCapableModels := make(map[string]bool)
	for _, model := range models {
		for _, sourceEndpoint := range model.SourceEndpoints {
			if existingCapableModels[sourceEndpoint.EndpointURL] {
				newCapableModels[sourceEndpoint.EndpointURL] = true
			}
		}
	}
	return newCapableModels
}

func (a *Application) filterEndpointsByCapableModels(endpoints []*domain.Endpoint, capableModels map[string]bool, requiredCapabilities []string, logger logger.StyledLogger) []*domain.Endpoint {
	// nil check differentiates "no capability support" from "no matches"
	if capableModels == nil {
		logger.Debug("Registry doesn't support capability queries, skipping capability filtering",
			"capabilities", requiredCapabilities)
		return endpoints
	}

	capableEndpoints := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if capableModels[endpoint.GetURLString()] {
			capableEndpoints = append(capableEndpoints, endpoint)
		}
	}

	if len(capableEndpoints) == 0 {
		logger.Warn("No endpoints have models with required capabilities, using unfiltered",
			"capabilities", requiredCapabilities,
			"available_endpoints", len(endpoints))
		return endpoints
	}

	logger.Debug("Filtered endpoints by capabilities",
		"capabilities", requiredCapabilities,
		"capable_count", len(capableEndpoints),
		"total_count", len(endpoints))

	return capableEndpoints
}
