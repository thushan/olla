package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/util"
	"github.com/tidwall/gjson"
)

// executePassthroughRequest handles requests that can be forwarded directly to backends
// without translation (e.g. Anthropic API requests to vLLM with native Anthropic support).
// bodyBytes is the pre-buffered request body from the handler, passed through to avoid re-reading.
func (a *Application) executePassthroughRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	// Get passthrough request details
	passthroughTrans, ok := trans.(translator.PassthroughCapable)
	if !ok {
		// This should never happen since we checked the interface before calling this function
		a.writeTranslatorError(w, trans, pr, errors.New("translator does not support passthrough"), http.StatusInternalServerError)
		return
	}

	passthroughReq, err := passthroughTrans.PreparePassthrough(bodyBytes, r, a.profileLookup)
	if err != nil {
		a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
		return
	}

	// The proxy selector chooses the actual endpoint later. When capable backends
	// disagree on their native Anthropic path (e.g. DMR uses /anthropic/v1/messages
	// while vLLM uses /v1/messages), we restrict to the largest path-compatible
	// subset so the selected backend and the request path always agree.
	resolvedPath, filteredEndpoints := a.resolvePassthroughTargetPath(endpoints, passthroughReq.TargetPath)
	if resolvedPath != "" {
		passthroughReq.TargetPath = resolvedPath
	}
	if len(filteredEndpoints) > 0 {
		endpoints = filteredEndpoints
	}

	// Update proxy request details - capture streaming flag for accurate metrics
	// (StreamingMs isn't populated in passthrough mode since we don't intercept the stream)
	pr.isStreaming = passthroughReq.IsStreaming

	// Set mode before logRequestStart so it appears on the lifecycle log lines.
	pr.translatorMode = constants.TranslatorModePassthrough

	pr.requestLogger.Debug("using passthrough mode (native Anthropic support)",
		"model", passthroughReq.ModelName,
		"streaming", passthroughReq.IsStreaming,
		"target_path", passthroughReq.TargetPath,
		"endpoints", len(endpoints))

	// Set request body and path
	r.Body = io.NopCloser(bytes.NewReader(passthroughReq.Body))
	r.ContentLength = int64(len(passthroughReq.Body))
	r.URL.Path = passthroughReq.TargetPath

	// Add passthrough mode header for observability
	w.Header().Set(constants.HeaderXOllaMode, string(constants.TranslatorModePassthrough))

	// Prepare context
	ctx, r = a.prepareProxyContext(ctx, r, pr)

	// Log request start
	a.logRequestStart(pr, len(endpoints))

	// Execute proxy
	err = a.proxyService.ProxyRequestToEndpoints(ctx, w, r, endpoints, pr.stats, pr.requestLogger)
	pr.captureStickyOutcome(ctx, r)
	a.logRequestResult(pr, err)

	if err != nil {
		// only write error if response hasn't started
		if w.Header().Get(constants.HeaderContentType) == "" {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("proxy error: %w", err), http.StatusBadGateway)
		}
	}

	pr.stats.EndTime = time.Now()
}

// resolvePassthroughTargetPath resolves the native Anthropic messages path and
// the endpoint subset that must be used with it.
//
// Uniform fleets: all candidates share the same MessagesPath. Returns that path
// and nil (caller keeps original endpoint list unchanged).
//
// Mixed fleets: candidates disagree on their native path (e.g. DMR uses
// /anthropic/v1/messages while vLLM uses /v1/messages). Restricting the proxy
// to the full list would cause a 404 on whichever backend is selected but whose
// path was not used. Instead we return the largest path-compatible subset.
// Tie-break: prefer the subset whose path equals defaultPath; if no subset
// matches, use the path whose first endpoint appears earliest in the original
// endpoints slice. This ensures deterministic selection regardless of map
// iteration order.
//
// If profileLookup is nil or the endpoint list is empty, returns defaultPath and
// nil so the caller leaves everything unchanged.
func (a *Application) resolvePassthroughTargetPath(endpoints []*domain.Endpoint, defaultPath string) (string, []*domain.Endpoint) {
	if a.profileLookup == nil || len(endpoints) == 0 {
		return defaultPath, nil
	}

	// Build path→endpoints groups while recording the first-seen index for each
	// path. Iterating the original slice preserves endpoint order so the
	// first-seen index is stable across calls.
	pathGroups := make(map[string][]*domain.Endpoint, len(endpoints))
	firstSeen := make(map[string]int, len(endpoints)) // path → index of first endpoint in slice
	for i, ep := range endpoints {
		support := a.profileLookup.GetAnthropicSupport(ep.Type)
		path := defaultPath
		if support != nil && support.MessagesPath != "" {
			path = support.MessagesPath
		}
		if _, exists := firstSeen[path]; !exists {
			firstSeen[path] = i
		}
		pathGroups[path] = append(pathGroups[path], ep)
	}

	// Uniform fleet: all candidates agree on a single path.
	if len(pathGroups) == 1 {
		for path := range pathGroups {
			// nil filtered list signals "keep original slice" to the caller.
			return path, nil
		}
	}

	// Mixed fleet: pick the largest subset. Tie-break order:
	//   1. defaultPath wins (avoids surprising path changes for the majority case).
	//   2. The path whose first endpoint appears earliest in the original slice
	//      wins (deterministic, not dependent on map iteration order).
	var bestPath string
	var bestCount int
	bestFirstSeen := -1

	for path, group := range pathGroups {
		count := len(group)
		fs := firstSeen[path]
		switch {
		case count > bestCount:
			bestPath, bestCount, bestFirstSeen = path, count, fs
		case count == bestCount && path == defaultPath:
			// defaultPath takes priority in a tie regardless of first-seen.
			bestPath, bestFirstSeen = path, fs
		case count == bestCount && bestPath != defaultPath && fs < bestFirstSeen:
			// Neither candidate is defaultPath; earliest first-seen endpoint wins.
			bestPath, bestFirstSeen = path, fs
		}
	}

	return bestPath, pathGroups[bestPath]
}

// executeTranslationRequest handles the translation path where requests are converted
// from the translator's native format (e.g. Anthropic) to OpenAI format for the backend
func (a *Application) executeTranslationRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
	transformedReq *translator.TransformedRequest,
) {
	// Capture streaming flag for metrics before proxying
	pr.isStreaming = transformedReq.IsStreaming

	// Set mode before logRequestStart so it appears on the lifecycle log lines.
	pr.translatorMode = constants.TranslatorModeTranslation

	// Serialize OpenAI request
	openaiBody, err := json.Marshal(transformedReq.OpenAIRequest)
	if err != nil {
		a.writeTranslatorError(w, trans, pr, errors.New("failed to serialize request"), http.StatusInternalServerError)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(openaiBody))
	r.ContentLength = int64(len(openaiBody))

	// Handle path translation if specified
	if transformedReq.TargetPath != "" {
		targetPath := util.StripPrefix(transformedReq.TargetPath, constants.DefaultOllaProxyPathPrefix)

		if targetPath != transformedReq.TargetPath {
			pr.requestLogger.Warn("TargetPath included proxy prefix, stripped it",
				"translator", trans.Name(),
				"proxy_prefix", constants.DefaultOllaProxyPathPrefix,
				"original_target", transformedReq.TargetPath,
				"corrected_target", targetPath)
		}

		pr.requestLogger.Debug("Path translation applied",
			"original_path", r.URL.Path,
			"target_path", targetPath,
			"translator", trans.Name())
		r.URL.Path = targetPath
	} else if trans.Name() != "passthrough" {
		// warn if translator might need path translation (passthrough can ignore)
		pr.requestLogger.Warn("Translator did not set TargetPath, using original path",
			"translator", trans.Name(),
			"original_path", r.URL.Path,
			"note", "This may cause routing issues if translation requires different endpoint")
	}

	a.logRequestStart(pr, len(endpoints))

	// For streaming requests, seed the context with an estimated input token count
	// computed from the original (pre-translation) request body. This lets the
	// streaming translator populate a non-zero input_tokens in message_start, matching
	// the behaviour of vLLM and lmdeploy which both emit real input_tokens at that point.
	// The estimate is overwritten by the actual upstream usage when it arrives.
	if transformedReq.IsStreaming {
		if estimator, ok := trans.(translator.InputTokenEstimator); ok && len(transformedReq.OriginalBody) > 0 {
			estimate := estimator.EstimateInputTokens(transformedReq.OriginalBody)
			ctx = context.WithValue(ctx, constants.ContextInputTokensKey, estimate)
			r = r.WithContext(ctx)
		}
	}

	// Execute proxy with appropriate response handling (streaming vs non-streaming)
	var proxyErr error
	if transformedReq.IsStreaming {
		proxyErr = a.executeTranslatedStreamingRequest(ctx, w, r, endpoints, pr, trans)
	} else {
		proxyErr = a.executeTranslatedNonStreamingRequest(ctx, w, r, endpoints, pr, trans)
	}

	pr.captureStickyOutcome(ctx, r)
	a.logRequestResult(pr, proxyErr)

	if proxyErr != nil {
		// only write error if response hasn't started
		if w.Header().Get(constants.HeaderContentType) == "" {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("proxy error: %w", proxyErr), http.StatusBadGateway)
		}
	}

	pr.stats.EndTime = time.Now()
}

// tryPassthrough attempts to serve the request via passthrough mode if the translator
// and at least one backend support it. Returns true if the request was handled.
func (a *Application) tryPassthrough(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) bool {
	passthroughTrans, ok := trans.(translator.PassthroughCapable)
	if !ok || a.profileLookup == nil {
		return false
	}

	// Only pass endpoints whose backend natively supports the wire format.
	// Mixed deployments (e.g. ollama + vllm) must not block passthrough for
	// the capable subset - the proxy will route within that filtered list.
	//
	// Additionally, exclude endpoints that declare a limitation matching a
	// feature present in this specific request. Token-counting limitations
	// (e.g. token_counting_404) use different names and are never affected.
	reqFeatures := detectRequestFeatures(bodyBytes)
	passthroughEndpoints := make([]*domain.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		support := a.profileLookup.GetAnthropicSupport(ep.Type)
		if support == nil || !support.Enabled {
			continue
		}
		if reqFeatures.toolUse && support.HasLimitation(domain.AnthropicLimitationNoToolUse) {
			continue
		}
		if reqFeatures.extendedThinking && support.HasLimitation(domain.AnthropicLimitationNoExtendedThinking) {
			continue
		}
		if reqFeatures.vision && support.HasLimitation(domain.AnthropicLimitationNoVision) {
			continue
		}
		passthroughEndpoints = append(passthroughEndpoints, ep)
	}

	if !passthroughTrans.CanPassthrough(passthroughEndpoints, a.profileLookup) {
		return false
	}

	a.executePassthroughRequest(ctx, w, r, bodyBytes, passthroughEndpoints, pr, trans)
	a.recordTranslatorMetrics(trans, pr, constants.TranslatorModePassthrough, constants.FallbackReasonNone)
	return true
}

// resolveTranslationFallback determines why passthrough was not used.
func (a *Application) resolveTranslationFallback(trans translator.RequestTranslator) constants.TranslatorFallbackReason {
	if _, ok := trans.(translator.PassthroughCapable); ok {
		return constants.FallbackReasonCannotPassthrough
	}
	return constants.FallbackReasonTranslatorDoesNotSupportPassthrough
}

// generic handler for any translator (eg anthropic to openai and back)
func (a *Application) translationHandler(trans translator.RequestTranslator) http.HandlerFunc {
	// Resolve body size limit once at registration time, not per-request.
	// Translators that implement BodySizeLimiter declare their own max;
	// others get a safe default.
	var maxBodySize int64 = 10 << 20 // 10 MiB default
	if limiter, ok := trans.(translator.BodySizeLimiter); ok {
		maxBodySize = limiter.MaxBodySize()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		pr := a.initializeProxyRequest(r)
		ctx, r := a.setupRequestContext(r, pr.stats)

		// Buffer body once -- both passthrough and translation paths need it.
		// Read maxBodySize+1 to detect oversized requests before JSON parsing
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNone)
			return
		}

		// Explicitly check for oversized body (return 413 instead of confusing JSON parse error)
		if int64(len(bodyBytes)) > maxBodySize {
			a.writeTranslatorError(w, trans, pr,
				fmt.Errorf("request body exceeds maximum size (%d bytes)", maxBodySize),
				http.StatusRequestEntityTooLarge)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNone)
			return
		}

		// Lightweight model extraction via gjson -- avoids a full TransformRequest
		// parse on the passthrough path where the body would be parsed twice
		// (once here for the model name, once in PreparePassthrough for validation).
		modelName, err := translator.ExtractModelName(bodyBytes)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNone)
			return
		}

		pr.model = modelName
		pr.stats.Model = pr.model

		// Restore body so the inspector chain can read it for routing decisions.
		// It was consumed by io.ReadAll above; model name is already captured via
		// ExtractModelName, but analyzeRequest/inspectorChain.Inspect needs the
		// body intact to build the routing profile (endpoint compatibility).
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Run through proxy pipeline (inspector, security, routing)
		a.analyzeRequest(ctx, r, pr)

		// Inject sticky session key. bodyBytes is already buffered from the model-name
		// extraction above, so pass it directly to avoid a second read/restore cycle.
		// The outcome pointer is stored in context; sub-handlers read it before WriteHeader.
		if a.Config.Proxy.StickySessions.Enabled {
			ctx, r, _ = a.injectStickyKeyWithBody(ctx, r, pr.model, bodyBytes)
		}

		// Get compatible endpoints for this request
		endpoints, err := a.getCompatibleEndpoints(ctx, pr)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, errors.New("no healthy endpoints available"), http.StatusServiceUnavailable)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNoCompatibleEndpoints)
			return
		}

		// OLLA-282: When no endpoints available, Olla hangs until timeout
		// make sure that we have at least one endpoint available
		// prevents hanging when model routing fails to find compatible backends
		if len(endpoints) == 0 {
			// A recorded routing decision (e.g. strict model_not_found/model_unavailable)
			// carries the precise status and reason. Without one, defaulting to 404
			// preserves the historical behaviour. Falling through to a hardcoded 404
			// would flatten a strict "model only on unhealthy endpoints" 503 to 404 and
			// drop the X-Olla-Routing-* headers, which the equivalent proxy-route
			// rejection (writeNoRoutableEndpoints) always sets (#191).
			var decision *domain.ModelRoutingDecision
			if pr.profile != nil {
				decision = pr.profile.RoutingDecision
			}

			status := http.StatusNotFound
			reason := fmt.Sprintf("no healthy endpoints available for model: %s", pr.model)
			if decision != nil && decision.StatusCode >= http.StatusBadRequest {
				status = decision.StatusCode
				reason = decision.Reason
				pr.stats.RoutingDecision = decision
				a.setRoutingDecisionHeaders(w, decision)
			}

			// Headers must be set before writeTranslatorError, which calls WriteHeader.
			a.setStickyResponseHeadersFromRequest(w, r)

			a.logRequestStart(pr, 0)
			a.logRequestRejected(pr, status)

			a.writeTranslatorError(w, trans, pr, errors.New(reason), status)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNoCompatibleEndpoints)
			return
		}

		// Attempt passthrough if the translator and backends support it.
		// Returns true when passthrough was used and the request is complete.
		// Sticky headers are written by the proxy engine before WriteHeader in this path.
		if a.tryPassthrough(ctx, w, r, bodyBytes, endpoints, pr, trans) {
			return
		}

		// Passthrough was not used - fall back to full translation.
		mode := constants.TranslatorModeTranslation
		fallbackReason := a.resolveTranslationFallback(trans)

		// surface the fallback reason so completed-request logs explain why
		// translation was required rather than passthrough
		pr.translatorFallbackReason = string(fallbackReason)

		// Translation path only -- perform the full parse and format conversion.
		// This is deferred to here so passthrough requests never pay the cost.
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		transformedReq, err := trans.TransformRequest(ctx, r)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			a.recordTranslatorMetrics(trans, pr, mode, fallbackReason)
			return
		}

		// Sticky headers are written inside executeTranslationRequest before WriteHeader.
		a.executeTranslationRequest(ctx, w, r, endpoints, pr, trans, transformedReq)
		a.recordTranslatorMetrics(trans, pr, mode, fallbackReason)
	}
}

// handle non-streaming, capture full response then transform
func (a *Application) executeTranslatedNonStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) error {
	recorder := newResponseRecorder()

	// prepare context and execute proxy request
	ctx, r = a.prepareProxyContext(ctx, r, pr)
	err := a.proxyService.ProxyRequestToEndpoints(ctx, recorder, r, endpoints, pr.stats, pr.requestLogger)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}

	// Check for backend errors before attempting JSON parse. A reverse proxy or
	// rate-limiter in front of the backend may return plain-text or HTML on 4xx/5xx,
	// so parsing first would surface a misleading "failed to parse OpenAI response"
	// error and lose the upstream status code (e.g. a plain-text 429 must stay 429).
	if recorder.status >= 400 {
		// Opportunistic parse: pass the map if the body is valid JSON so the
		// error formatter can extract a message; pass nil otherwise and let
		// extractAndLogBackendError fall back to its generic message.
		var openaiErrResp map[string]interface{}
		_ = json.Unmarshal(recorder.body.Bytes(), &openaiErrResp)
		return a.handleNonStreamingBackendError(w, r, recorder, openaiErrResp, pr, trans)
	}

	// Success path: strict parse - a malformed 200 body is a gateway error.
	var openaiResp map[string]interface{}
	if jerr := json.Unmarshal(recorder.body.Bytes(), &openaiResp); jerr != nil {
		return fmt.Errorf("failed to parse OpenAI response: %w", jerr)
	}

	// transform and write successful response
	return a.writeTranslatedSuccessResponse(w, ctx, r, recorder, openaiResp, trans)
}

// prepareProxyContext sets up context with model, routing decision, and alias rewrite map
func (a *Application) prepareProxyContext(ctx context.Context, r *http.Request, pr *proxyRequest) (context.Context, *http.Request) {
	if pr.model != "" {
		ctx = context.WithValue(ctx, constants.ContextModelKey, pr.model)
		r = r.WithContext(ctx)
	}

	if pr.profile != nil && pr.profile.RoutingDecision != nil {
		pr.stats.RoutingDecision = pr.profile.RoutingDecision
	}

	// if a model alias was resolved, pass the endpoint→model rewrite map through context
	// so the proxy can rewrite the model name in the request body for the selected backend
	if pr.profile != nil {
		if aliasMapRaw, ok := pr.profile.InspectionMeta.Load(constants.ContextModelAliasMapKey); ok {
			if aliasMap, ok := aliasMapRaw.(map[string]string); ok {
				ctx = context.WithValue(ctx, constants.ContextModelAliasMapKey, aliasMap)
				r = r.WithContext(ctx)
			}
		}
	}

	return ctx, r
}

// handleNonStreamingBackendError processes backend errors and writes translated error response
func (a *Application) handleNonStreamingBackendError(
	w http.ResponseWriter,
	r *http.Request,
	recorder *responseRecorder,
	openaiResp map[string]interface{},
	pr *proxyRequest,
	trans translator.RequestTranslator,
) error {
	pr.requestLogger.Debug("Backend returned error, translating to target format",
		"status_code", recorder.status,
		"translator", trans.Name())

	errorMsg := a.extractAndLogBackendError(openaiResp, recorder.status, pr, trans)

	// copy observability headers before writing error
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	a.copyOllaHeaders(recorder, w)
	a.setModelHeaderIfMissing(w, pr.model)
	a.setStickyResponseHeadersFromRequest(w, r)

	// Use translator's error formatter if available
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, fmt.Errorf("%s", errorMsg), recorder.status)
		return nil
	}

	// fallback to generic error
	w.WriteHeader(recorder.status)
	if _, werr := w.Write(recorder.body.Bytes()); werr != nil {
		return fmt.Errorf("failed to write error response: %w", werr)
	}
	return nil
}

// extractAndLogBackendError extracts error details from OpenAI response and logs them
func (a *Application) extractAndLogBackendError(
	openaiResp map[string]interface{},
	statusCode int,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) string {
	errorMsg := "Backend error"
	var errorType, errorParam, errorCode string

	errObj, ok := openaiResp["error"].(map[string]interface{})
	if !ok {
		pr.requestLogger.Info("Translating backend error response",
			"status_code", statusCode,
			"error_message", errorMsg,
			"translator", trans.Name())
		return errorMsg
	}

	// extract error fields
	if msg, ok := errObj["message"].(string); ok && msg != "" {
		errorMsg = msg
	}
	if typ, ok := errObj["type"].(string); ok {
		errorType = typ
	}
	if param, ok := errObj["param"].(string); ok {
		errorParam = param
	}
	if code, ok := errObj["code"].(string); ok {
		errorCode = code
	}

	// log full error details for debugging
	pr.requestLogger.Info("Translating backend error response",
		"status_code", statusCode,
		"error_message", errorMsg,
		"error_type", errorType,
		"error_param", errorParam,
		"error_code", errorCode,
		"translator", trans.Name())

	return errorMsg
}

// writeTranslatedSuccessResponse transforms and writes successful response
func (a *Application) writeTranslatedSuccessResponse(
	w http.ResponseWriter,
	ctx context.Context,
	r *http.Request,
	recorder *responseRecorder,
	openaiResp map[string]interface{},
	trans translator.RequestTranslator,
) error {
	// Transform successful OpenAI response back to target format
	targetResp, err := trans.TransformResponse(ctx, openaiResp, r)
	if err != nil {
		return fmt.Errorf("failed to transform response: %w", err)
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	a.copyOllaHeaders(recorder, w)
	// Write sticky headers before committing the response.
	a.setStickyResponseHeadersFromRequest(w, r)

	// Serialize and write response
	respBody, err := json.Marshal(targetResp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(respBody); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}

// setModelHeaderIfMissing sets X-Olla-Model header if not already present
func (a *Application) setModelHeaderIfMissing(w http.ResponseWriter, model string) {
	if w.Header().Get(constants.HeaderXOllaModel) == "" && model != "" {
		w.Header().Set(constants.HeaderXOllaModel, model)
	}
}

// handle streaming via pipe, proxy writes to translator reads from
func (a *Application) executeTranslatedStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) error {
	// safety check - should never trigger but prevents bugs
	if len(endpoints) == 0 {
		a.writeStreamingNoEndpointsError(w, pr, trans)
		return nil
	}

	// pipe connects proxy output to translator input
	pipeReader, pipeWriter := io.Pipe()
	streamRecorder := newStreamingResponseRecorder(pipeWriter)

	// Wrap the real client ResponseWriter so we can track whether any byte has
	// actually been committed to the client (status line written or body written).
	// This is distinct from streamRecorder.started, which only signals that the
	// backend wrote into the pipe -- bytes may still be in the translator's buffer
	// at that point, not yet on the wire to the client.
	cw := newCommittedResponseWriter(w)

	// run proxy in background while translation processes
	proxyErrChan := a.startProxyGoroutine(ctx, r, endpoints, pr, streamRecorder, pipeWriter)

	// panic recovery prevents goroutine leak; sends a 502 (or SSE error event
	// if the stream has already started) to the client rather than re-panicking,
	// which would produce a connection reset with no useful error for the client.
	defer a.handleStreamingPanic(cw, pipeReader, pipeWriter, proxyErrChan, cw, pr, trans)

	// Wait for headers before inspecting status. The select also handles context
	// cancellation so we don't block forever if the proxy errors without writing.
	select {
	case <-streamRecorder.headersReady:
	case <-ctx.Done():
		pipeReader.CloseWithError(ctx.Err()) // unblock any proxy goroutine stuck mid-write to pipeWriter
		return fmt.Errorf("request cancelled while waiting for backend headers: %w", ctx.Err())
	}

	// handle backend errors before starting sse stream
	if streamRecorder.status >= 400 {
		a.handleStreamingBackendError(cw, r, pipeReader, streamRecorder, proxyErrChan, pr, trans)
		return nil
	}

	// copy olla headers before stream starts
	a.copyOllaHeaders(streamRecorder, cw)
	a.setModelHeaderIfMissing(cw, pr.model)
	// Write sticky headers before the first write to cw commits the response.
	a.setStickyResponseHeadersFromRequest(cw, r)

	// transform stream (blocks until done) and wait for proxy
	return a.transformStreamAndWaitForProxy(ctx, pipeReader, cw, r, proxyErrChan, trans)
}

// writeStreamingNoEndpointsError writes error when no endpoints are available for streaming
func (a *Application) writeStreamingNoEndpointsError(
	w http.ResponseWriter,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	pr.requestLogger.Error("Streaming pipeline called with zero endpoints - this is a bug")
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, errors.New("no healthy endpoints available"), http.StatusServiceUnavailable)
		return
	}

	// fallback to generic error
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": "No healthy endpoints available",
		},
	})
}

// startProxyGoroutine starts background goroutine to proxy request to endpoints
func (a *Application) startProxyGoroutine(
	ctx context.Context,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	streamRecorder *streamingResponseRecorder,
	pipeWriter *io.PipeWriter,
) chan error {
	proxyErrChan := make(chan error, 1)
	go func() {
		localCtx, localR := a.prepareProxyContext(ctx, r, pr)
		err := a.proxyService.ProxyRequestToEndpoints(localCtx, streamRecorder, localR, endpoints, pr.stats, pr.requestLogger)
		// If the proxy returned an error without ever calling Write or WriteHeader,
		// headersReady is never closed and the main goroutine blocks forever.
		// Ensure it is always signalled before closing the pipe.
		streamRecorder.ensureHeadersReady()
		pipeWriter.Close() // Signal end of stream
		proxyErrChan <- err
	}()
	return proxyErrChan
}

// handleStreamingPanic recovers from panic during streaming to prevent goroutine
// leak and connection resets. Instead of re-panicking (which terminates the
// connection abruptly and gives the client no useful error), it closes the pipe,
// drains the error channel, and writes an appropriate error to the client.
//
// The committed flag on cw tracks whether any byte has actually been written to
// the real client ResponseWriter (not just to the backend pipe). When committed,
// net/http silently ignores a WriteHeader(502), so we emit a spec-valid Anthropic
// SSE error event instead. When not committed, a plain HTTP 502 with a structured
// body reaches the client correctly.
func (a *Application) handleStreamingPanic(
	w http.ResponseWriter,
	pipeReader *io.PipeReader,
	pipeWriter *io.PipeWriter,
	proxyErrChan chan error,
	cw *committedResponseWriter,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	if r := recover(); r != nil {
		// Close both ends of the pipe to unblock the proxy goroutine.
		pipeReader.Close()
		pipeWriter.Close()

		// Drain the buffered error channel to release the proxy goroutine.
		<-proxyErrChan

		a.logger.Error("Panic during stream transformation",
			"panic", r,
			"translator", trans.Name(),
			"model", pr.model)

		// If anything was already written to the real client, the response is
		// committed and WriteHeader(502) is a no-op. Emitting a well-formed SSE
		// error event is the only way to signal the failure without corrupting
		// the stream with raw JSON.
		if cw != nil && cw.committed.Load() {
			a.writeSSEErrorEvent(w, "internal error during stream transformation")
			return
		}

		// Nothing reached the client yet - response is uncommitted, so a plain
		// HTTP 502 with a structured error body reaches the client correctly.
		if ew, ok := trans.(translator.ErrorWriter); ok {
			ew.WriteError(w, errors.New("internal error during stream transformation"), http.StatusBadGateway)
		} else {
			http.Error(w, "internal error during stream transformation", http.StatusBadGateway)
		}
	}
}

// writeSSEErrorEvent emits a spec-valid Anthropic SSE error event into an
// already-committed text/event-stream response. This is the only safe way to
// report a mid-stream failure; a WriteHeader call would be silently ignored by
// net/http once the response is committed.
func (a *Application) writeSSEErrorEvent(w http.ResponseWriter, message string) {
	if w == nil {
		return
	}
	payload := fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":%q}}`, message)
	// Prefix with \n\n to guarantee a clean event boundary in case the panic
	// occurred mid-write of a previous event. SSE parsers treat blank lines as
	// field separators, so extra blank lines are harmless when already at a boundary.
	sseEvent := fmt.Sprintf("\n\nevent: error\ndata: %s\n\n", payload)
	_, _ = fmt.Fprint(w, sseEvent)
	// Flush if the underlying writer supports it, so the client receives
	// the event immediately rather than waiting for a buffer drain.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleStreamingBackendError processes backend errors during streaming
func (a *Application) handleStreamingBackendError(
	w http.ResponseWriter,
	r *http.Request,
	pipeReader *io.PipeReader,
	streamRecorder *streamingResponseRecorder,
	proxyErrChan chan error,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	pr.requestLogger.Debug("Backend returned error in streaming mode, translating to target format",
		"status_code", streamRecorder.status,
		"translator", trans.Name())

	// Cap the error-body read so a misbehaving backend cannot exhaust the heap.
	// Anything beyond MaxUpstreamErrorBodyBytes is silently discarded; legitimate
	// error messages are far smaller than this limit.
	// After the limited read, close the reader with an error so the proxy goroutine's
	// blocked pipe write is unblocked rather than hanging forever.
	errorBody, _ := io.ReadAll(io.LimitReader(pipeReader, constants.MaxUpstreamErrorBodyBytes))
	pipeReader.CloseWithError(io.ErrClosedPipe)

	// try to parse OpenAI error format and extract message
	errorMsg := a.parseStreamingErrorMessage(errorBody)

	// Copy observability headers before writing error
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	a.copyOllaHeaders(streamRecorder, w)
	a.setModelHeaderIfMissing(w, pr.model)
	a.setStickyResponseHeadersFromRequest(w, r)

	// Use translator's error formatter if available
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, fmt.Errorf("%s", errorMsg), streamRecorder.status)
		<-proxyErrChan // Wait for proxy goroutine to complete
		return
	}

	// fallback to generic error
	a.writeGenericStreamingError(w, streamRecorder.status)
	<-proxyErrChan // Wait for proxy goroutine to complete
}

// parseStreamingErrorMessage extracts error message from streaming error response
func (a *Application) parseStreamingErrorMessage(errorBody []byte) string {
	errorMsg := "Backend error"

	var openaiResp map[string]interface{}
	if err := json.Unmarshal(errorBody, &openaiResp); err == nil {
		if errObj, ok := openaiResp["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				errorMsg = msg
			}
		}
	}

	return errorMsg
}

// writeGenericStreamingError writes a generic streaming error response
func (a *Application) writeGenericStreamingError(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": "Backend returned an error",
		},
	})
}

// transformStreamAndWaitForProxy transforms stream and waits for proxy completion
func (a *Application) transformStreamAndWaitForProxy(
	ctx context.Context,
	pipeReader *io.PipeReader,
	w http.ResponseWriter,
	r *http.Request,
	proxyErrChan chan error,
	trans translator.RequestTranslator,
) error {
	// transform stream (blocks until done)
	transformErr := trans.TransformStreamingResponse(ctx, pipeReader, w, r)

	// Wait for proxy to complete
	proxyErr := <-proxyErrChan

	// return first error, transform errors take precedence
	if transformErr != nil {
		return fmt.Errorf("stream transformation failed: %w", transformErr)
	}
	if proxyErr != nil {
		return fmt.Errorf("proxy request failed: %w", proxyErr)
	}

	return nil
}

// write error using translator format or fallback to generic json
func (a *Application) writeTranslatorError(
	w http.ResponseWriter,
	trans translator.RequestTranslator,
	pr *proxyRequest,
	err error,
	statusCode int,
) {
	pr.hadError = true

	pr.requestLogger.Error("Translation request failed",
		"translator", trans.Name(),
		"error", err.Error(),
		"status", statusCode)

	// use custom error format if translator implements it
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, err, statusCode)
		return
	}

	// fallback to generic json
	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": err.Error(),
			"type":    "translation_error",
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(statusCode)

	if encErr := json.NewEncoder(w).Encode(errorResp); encErr != nil {
		pr.requestLogger.Error("Failed to write error response", "error", encErr)
	}
}

// token counting handler, only for translators that implement TokenCounter
func (a *Application) tokenCountHandler(trans translator.RequestTranslator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if translator implements token counting
		counter, ok := trans.(translator.TokenCounter)
		if !ok {
			a.logger.Error("Translator does not support token counting", "translator", trans.Name())
			http.Error(w, "Token counting not supported", http.StatusNotImplemented)
			return
		}

		ctx := r.Context()

		// Call the translator's token counting implementation
		resp, err := counter.CountTokens(ctx, r)
		if err != nil {
			a.logger.Error("Token counting failed",
				"translator", trans.Name(),
				"error", err.Error())

			// Use translator's error format if available
			if errorWriter, ok := trans.(translator.ErrorWriter); ok {
				errorWriter.WriteError(w, err, http.StatusBadRequest)
				return
			}

			// Fallback to generic error
			http.Error(w, fmt.Sprintf("Token counting failed: %v", err), http.StatusBadRequest)
			return
		}

		// Serialise before touching the response writer. WriteHeader(200) is a
		// one-way door - once sent the client sees a 200 even if serialisation
		// subsequently fails, which results in a truncated or empty body with a
		// misleading success status.
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)

		if serialiser, ok := trans.(translator.TokenCountSerializer); ok {
			body, serErr := serialiser.SerialiseCountTokens(resp)
			if serErr != nil {
				a.logger.Error("Failed to serialise token count response", "error", serErr)
				if errorWriter, ok := trans.(translator.ErrorWriter); ok {
					errorWriter.WriteError(w, serErr, http.StatusInternalServerError)
				} else {
					http.Error(w, "internal error serialising token count", http.StatusInternalServerError)
				}
				return
			}
			w.WriteHeader(http.StatusOK)
			if _, wErr := w.Write(body); wErr != nil { //nolint:gosec // body is serialised JSON, not user-controlled data
				a.logger.Error("Failed to write token count response", "error", wErr)
			}
		} else {
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				a.logger.Error("Failed to encode token count response", "error", err)
			}
		}
	}
}

// copy olla observability headers
func (a *Application) copyOllaHeaders(from headerGetter, to http.ResponseWriter) {
	ollaHeaders := []string{
		constants.HeaderXOllaRequestID,
		constants.HeaderXOllaEndpoint,
		constants.HeaderXOllaBackendType,
		constants.HeaderXOllaModel,
		constants.HeaderXOllaResponseTime,
		constants.HeaderXOllaRoutingStrategy,
		constants.HeaderXOllaRoutingDecision,
		constants.HeaderXOllaRoutingReason,
	}

	for _, header := range ollaHeaders {
		if value := from.Header().Get(header); value != "" {
			to.Header().Set(header, value)
		}
	}
}

// recordTranslatorMetrics records metrics for translator requests
func (a *Application) recordTranslatorMetrics(
	trans translator.RequestTranslator,
	pr *proxyRequest,
	mode constants.TranslatorMode,
	fallbackReason constants.TranslatorFallbackReason,
) {
	// Calculate latency from request stats
	latency := time.Since(pr.stats.StartTime)
	if !pr.stats.EndTime.IsZero() {
		latency = pr.stats.EndTime.Sub(pr.stats.StartTime)
	}

	// Determine if request was successful (no error flag set)
	success := !pr.hadError

	// Use the streaming flag captured during request preparation rather than
	// inferring from StreamingMs, which isn't populated in passthrough mode
	isStreaming := pr.isStreaming

	// Record the event
	event := ports.TranslatorRequestEvent{
		TranslatorName: trans.Name(),
		Model:          pr.model,
		Mode:           mode,
		FallbackReason: fallbackReason,
		Success:        success,
		Latency:        latency,
		IsStreaming:    isStreaming,
	}

	a.statsCollector.RecordTranslatorRequest(event)
}

// requestFeatures holds which Anthropic features are active in a specific request.
// Used to decide which capable endpoints are eligible for passthrough.
type requestFeatures struct {
	toolUse          bool
	extendedThinking bool
	vision           bool
}

// detectRequestFeatures inspects the raw Anthropic request body with gjson to
// determine which feature flags are active. It is deliberately cheap: gjson
// does not allocate a full parse tree, and we only scan the fields we care about.
//
// Content blocks can be a plain string (scalar) or an array of typed objects.
// The vision check handles both: a scalar string never contains image blocks,
// so we only iterate when content is a JSON array.
func detectRequestFeatures(body []byte) requestFeatures {
	if len(body) == 0 {
		return requestFeatures{}
	}

	var f requestFeatures

	// Non-empty tools array → tool use is active.
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		f.toolUse = true
	}

	// Presence of top-level "thinking" key → extended thinking is active.
	if gjson.GetBytes(body, "thinking").Exists() {
		f.extendedThinking = true
	}

	// Vision: any message content block with type=="image".
	// messages.#.content yields the content field of every message.
	// Each content entry may be a plain string (skip) or an array of blocks.
	gjson.GetBytes(body, "messages.#.content").ForEach(func(_, contentVal gjson.Result) bool {
		if !contentVal.IsArray() {
			// Plain string content - no image blocks possible.
			return true
		}
		contentVal.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").Str == "image" {
				f.vision = true
				return false // stop iterating blocks
			}
			return true
		})
		return !f.vision // stop iterating messages once found
	})

	return f
}

// committedResponseWriter wraps the real client http.ResponseWriter and sets a
// committed flag on the first call to Write or WriteHeader. The panic handler
// reads this flag to decide whether to attempt a plain HTTP 502 (not yet committed)
// or inject an SSE error event into the already-open stream (committed). Using the
// real client write as the signal is correct; streamRecorder.started only tracks
// whether the backend wrote into the pipe, which can be true before any byte has
// reached the actual client.
type committedResponseWriter struct {
	http.ResponseWriter
	committed atomic.Bool
}

func newCommittedResponseWriter(w http.ResponseWriter) *committedResponseWriter {
	return &committedResponseWriter{ResponseWriter: w}
}

func (c *committedResponseWriter) WriteHeader(statusCode int) {
	c.committed.Store(true)
	c.ResponseWriter.WriteHeader(statusCode)
}

func (c *committedResponseWriter) Write(b []byte) (int, error) {
	c.committed.Store(true)
	return c.ResponseWriter.Write(b)
}

// Flush marks the response as committed (bytes are en route to the client) and
// forwards the flush to the underlying writer. Without this, http.ResponseController
// cannot find http.Flusher on the wrapped writer, causing every SSE flush to return
// "feature not supported" and the translated streaming path to 502.
func (c *committedResponseWriter) Flush() {
	c.committed.Store(true)
	http.NewResponseController(c.ResponseWriter).Flush() //nolint:errcheck // best-effort flush; caller drives SSE cadence
}

// Unwrap returns the underlying ResponseWriter so that http.ResponseController can
// reach optional interfaces (SetWriteDeadline, etc.) that we don't explicitly proxy.
func (c *committedResponseWriter) Unwrap() http.ResponseWriter {
	return c.ResponseWriter
}

// abstract header access for both response types
type headerGetter interface {
	Header() http.Header
}

// captures full response for non-streaming (when we need to inspect/transform)
type responseRecorder struct {
	headers http.Header
	body    *bytes.Buffer
	status  int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		headers: make(http.Header),
		body:    bytes.NewBuffer(make([]byte, 0, 4096)),
		status:  http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

// captures headers while forwarding body to pipe (for streaming)
type streamingResponseRecorder struct {
	writer       io.Writer
	headers      http.Header
	headersReady chan struct{}
	status       int
	closeOnce    sync.Once
	// started is set true on the first Write, signalling that the upstream has
	// begun emitting body bytes. The panic handler uses this to decide whether
	// to inject an SSE error event into the already-open stream rather than
	// attempting a plain HTTP 502 (which net/http silently ignores post-commit).
	// atomic.Bool because Write is called from the proxy goroutine while
	// handleStreamingPanic reads it from the handler goroutine.
	started atomic.Bool
}

func newStreamingResponseRecorder(w io.Writer) *streamingResponseRecorder {
	return &streamingResponseRecorder{
		headers:      make(http.Header),
		writer:       w,
		headersReady: make(chan struct{}),
		status:       200,
	}
}

func (r *streamingResponseRecorder) Header() http.Header {
	return r.headers
}

// ensureHeadersReady closes headersReady exactly once. It is safe to call from
// multiple goroutines and is idempotent - subsequent calls are no-ops.
func (r *streamingResponseRecorder) ensureHeadersReady() {
	r.closeOnce.Do(func() { close(r.headersReady) })
}

func (r *streamingResponseRecorder) Write(data []byte) (int, error) {
	r.started.Store(true)
	r.ensureHeadersReady()
	return r.writer.Write(data)
}

func (r *streamingResponseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode // Capture status code to detect backend errors
	r.ensureHeadersReady()
	// Don't propagate the status write for streaming; just mark headers sent.
}

// Flush implements http.Flusher. The underlying io.Pipe is unbuffered
// (writes block until read), so there is nothing to flush - this is
// intentionally a no-op to satisfy http.ResponseController in proxy engines.
func (r *streamingResponseRecorder) Flush() {}
