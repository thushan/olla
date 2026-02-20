package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/util"
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
		a.writeTranslatorError(w, trans, pr, fmt.Errorf("translator does not support passthrough"), http.StatusInternalServerError)
		return
	}

	passthroughReq, err := passthroughTrans.PreparePassthrough(bodyBytes, r, a.profileLookup)
	if err != nil {
		a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
		return
	}

	// Update proxy request details - capture streaming flag for accurate metrics
	// (StreamingMs isn't populated in passthrough mode since we don't intercept the stream)
	pr.isStreaming = passthroughReq.IsStreaming

	pr.requestLogger.Info("using passthrough mode (native Anthropic support)",
		"model", passthroughReq.ModelName,
		"streaming", passthroughReq.IsStreaming,
		"endpoints", len(endpoints))

	// Set request body and path
	r.Body = io.NopCloser(bytes.NewReader(passthroughReq.Body))
	r.ContentLength = int64(len(passthroughReq.Body))
	r.URL.Path = passthroughReq.TargetPath

	// Add passthrough mode header for observability
	w.Header().Set("X-Olla-Mode", "passthrough")

	// Prepare context
	ctx, r = a.prepareProxyContext(ctx, r, pr)

	// Log request start
	a.logRequestStart(pr, len(endpoints))

	// Execute proxy
	err = a.proxyService.ProxyRequestToEndpoints(ctx, w, r, endpoints, pr.stats, pr.requestLogger)

	a.logRequestResult(pr, err)

	if err != nil {
		// only write error if response hasn't started
		if w.Header().Get(constants.HeaderContentType) == "" {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("proxy error: %w", err), http.StatusBadGateway)
		}
	}

	pr.stats.EndTime = time.Now()
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

	// Serialize OpenAI request
	openaiBody, err := json.Marshal(transformedReq.OpenAIRequest)
	if err != nil {
		a.writeTranslatorError(w, trans, pr, fmt.Errorf("failed to serialize request"), http.StatusInternalServerError)
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

	// Execute proxy with appropriate response handling (streaming vs non-streaming)
	var proxyErr error
	if transformedReq.IsStreaming {
		proxyErr = a.executeTranslatedStreamingRequest(ctx, w, r, endpoints, pr, trans)
	} else {
		proxyErr = a.executeTranslatedNonStreamingRequest(ctx, w, r, endpoints, pr, trans)
	}

	if proxyErr == nil {
		pr.requestLogger.Debug("Translation request completed successfully",
			"translator", trans.Name(),
			"model", pr.model,
			"path_translated", transformedReq.TargetPath != "",
			"target_path", transformedReq.TargetPath,
			"streaming", transformedReq.IsStreaming)
	}

	a.logRequestResult(pr, proxyErr)

	if proxyErr != nil {
		// only write error if response hasn't started
		if w.Header().Get(constants.HeaderContentType) == "" {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("proxy error: %w", proxyErr), http.StatusBadGateway)
		}
	}

	pr.stats.EndTime = time.Now()
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

		// Get compatible endpoints for this request
		endpoints, err := a.getCompatibleEndpoints(ctx, pr)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("no healthy endpoints available"), http.StatusServiceUnavailable)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNoCompatibleEndpoints)
			return
		}

		// OLLA-282: When no endpoints available, Olla hangs until timeout
		// make sure that we have at least one endpoint available
		// prevents hanging when model routing fails to find compatible backends
		if len(endpoints) == 0 {
			pr.requestLogger.Warn("No endpoints available for model",
				"model", pr.model,
				"translator", trans.Name())
			a.writeTranslatorError(w, trans, pr,
				fmt.Errorf("no healthy endpoints available for model: %s", pr.model),
				http.StatusNotFound)
			a.recordTranslatorMetrics(trans, pr, constants.TranslatorModeTranslation, constants.FallbackReasonNoCompatibleEndpoints)
			return
		}

		// Determine mode and fallback reason
		var mode constants.TranslatorMode
		var fallbackReason constants.TranslatorFallbackReason

		// Check for passthrough capability
		if passthroughTrans, ok := trans.(translator.PassthroughCapable); ok {
			if a.profileLookup != nil {
				// Only pass endpoints whose backend natively supports the wire format.
				// Mixed deployments (e.g. ollama + vllm) must not block passthrough for
				// the capable subset — the proxy will route within that filtered list.
				passthroughEndpoints := make([]*domain.Endpoint, 0, len(endpoints))
				for _, ep := range endpoints {
					support := a.profileLookup.GetAnthropicSupport(ep.Type)
					if support != nil && support.Enabled {
						passthroughEndpoints = append(passthroughEndpoints, ep)
					}
				}

				if passthroughTrans.CanPassthrough(passthroughEndpoints, a.profileLookup) {
					// Passthrough mode -- bodyBytes goes directly to PreparePassthrough
					// which validates without re-reading. No TransformRequest needed.
					mode = constants.TranslatorModePassthrough
					fallbackReason = constants.FallbackReasonNone

					a.executePassthroughRequest(ctx, w, r, bodyBytes, passthroughEndpoints, pr, trans)
					a.recordTranslatorMetrics(trans, pr, mode, fallbackReason)
					return
				}
			}
			// Translation mode with fallback reason
			mode = constants.TranslatorModeTranslation
			fallbackReason = constants.FallbackReasonCannotPassthrough
		} else {
			// Translator doesn't support passthrough
			mode = constants.TranslatorModeTranslation
			fallbackReason = constants.FallbackReasonTranslatorDoesNotSupportPassthrough
		}

		// Translation path only -- perform the full parse and format conversion.
		// This is deferred to here so passthrough requests never pay the cost.
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		transformedReq, err := trans.TransformRequest(ctx, r)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			a.recordTranslatorMetrics(trans, pr, mode, fallbackReason)
			return
		}

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

	// Parse OpenAI response
	var openaiResp map[string]interface{}
	if jerr := json.Unmarshal(recorder.body.Bytes(), &openaiResp); jerr != nil {
		return fmt.Errorf("failed to parse OpenAI response: %w", jerr)
	}

	// handle backend errors
	if recorder.status >= 400 {
		return a.handleNonStreamingBackendError(w, recorder, openaiResp, pr, trans)
	}

	// transform and write successful response
	return a.writeTranslatedSuccessResponse(w, ctx, r, recorder, openaiResp, trans)
}

// prepareProxyContext sets up context with model and routing decision
func (a *Application) prepareProxyContext(ctx context.Context, r *http.Request, pr *proxyRequest) (context.Context, *http.Request) {
	if pr.model != "" {
		ctx = context.WithValue(ctx, "model", pr.model)
		r = r.WithContext(ctx)
	}

	if pr.profile != nil && pr.profile.RoutingDecision != nil {
		pr.stats.RoutingDecision = pr.profile.RoutingDecision
	}

	return ctx, r
}

// handleNonStreamingBackendError processes backend errors and writes translated error response
func (a *Application) handleNonStreamingBackendError(
	w http.ResponseWriter,
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

	// run proxy in background while translation processes
	proxyErrChan := a.startProxyGoroutine(ctx, r, endpoints, pr, streamRecorder, pipeWriter)

	// panic recovery prevents goroutine leak, cleanup before re-panic
	defer a.handleStreamingPanic(pipeReader, pipeWriter, proxyErrChan, pr, trans)

	// wait for headers to avoid data race
	<-streamRecorder.headersReady

	// handle backend errors before starting sse stream
	if streamRecorder.status >= 400 {
		a.handleStreamingBackendError(w, pipeReader, streamRecorder, proxyErrChan, pr, trans)
		return nil
	}

	// copy olla headers before stream starts
	a.copyOllaHeaders(streamRecorder, w)
	a.setModelHeaderIfMissing(w, pr.model)

	// transform stream (blocks until done) and wait for proxy
	return a.transformStreamAndWaitForProxy(ctx, pipeReader, w, r, proxyErrChan, trans)
}

// writeStreamingNoEndpointsError writes error when no endpoints are available for streaming
func (a *Application) writeStreamingNoEndpointsError(
	w http.ResponseWriter,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	pr.requestLogger.Error("Streaming pipeline called with zero endpoints - this is a bug")
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, fmt.Errorf("no healthy endpoints available"), http.StatusServiceUnavailable)
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
		pipeWriter.Close() // Signal end of stream
		proxyErrChan <- err
	}()
	return proxyErrChan
}

// handleStreamingPanic recovers from panic during streaming to prevent goroutine leak
func (a *Application) handleStreamingPanic(
	pipeReader *io.PipeReader,
	pipeWriter *io.PipeWriter,
	proxyErrChan chan error,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	if r := recover(); r != nil {
		// Close both ends of the pipe to unblock the goroutine
		pipeReader.Close()
		pipeWriter.Close()

		// Drain the error channel to prevent goroutine leak
		<-proxyErrChan

		a.logger.Error("Panic during stream transformation",
			"panic", r,
			"translator", trans.Name(),
			"model", pr.model)

		// Re-panic after cleanup to preserve the panic behavior
		panic(r)
	}
}

// handleStreamingBackendError processes backend errors during streaming
func (a *Application) handleStreamingBackendError(
	w http.ResponseWriter,
	pipeReader *io.PipeReader,
	streamRecorder *streamingResponseRecorder,
	proxyErrChan chan error,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) {
	pr.requestLogger.Debug("Backend returned error in streaming mode, translating to target format",
		"status_code", streamRecorder.status,
		"translator", trans.Name())

	// Read error response from pipe
	errorBody, _ := io.ReadAll(pipeReader)

	// try to parse OpenAI error format and extract message
	errorMsg := a.parseStreamingErrorMessage(errorBody)

	// Copy observability headers before writing error
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	a.copyOllaHeaders(streamRecorder, w)
	a.setModelHeaderIfMissing(w, pr.model)

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

		// Write successful response
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			a.logger.Error("Failed to encode token count response", "error", err)
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
	headerSent   bool
	status       int
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

func (r *streamingResponseRecorder) Write(data []byte) (int, error) {
	if !r.headerSent {
		r.headerSent = true
		close(r.headersReady) // Signal headers are ready when first write occurs
	}
	return r.writer.Write(data)
}

func (r *streamingResponseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode // Capture status code to detect backend errors
	if !r.headerSent {
		r.headerSent = true
		close(r.headersReady) // Signal headers are ready
	}
	// don't write status for streaming, just mark headers sent
}
