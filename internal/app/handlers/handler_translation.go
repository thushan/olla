package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

// generic handler for any translator (eg anthropic to openai and back)
func (a *Application) translationHandler(trans translator.RequestTranslator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pr := a.initializeProxyRequest(r)
		ctx, r := a.setupRequestContext(r, pr.stats)

		transformedReq, err := trans.TransformRequest(ctx, r)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			return
		}

		pr.model = transformedReq.ModelName
		pr.stats.Model = pr.model

		// serialize for proxy
		openaiBody, err := json.Marshal(transformedReq.OpenAIRequest)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("failed to serialize request"), http.StatusInternalServerError)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(openaiBody))
		r.ContentLength = int64(len(openaiBody))

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

		// run through proxy pipeline (inspector, security, routing)
		a.analyzeRequest(ctx, r, pr)

		// Get compatible endpoints for this request
		endpoints, err := a.getCompatibleEndpoints(ctx, pr)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("no healthy endpoints available"), http.StatusServiceUnavailable)
			return
		}

		// OLLA-282: When no endpoints available, Olla hangs until timeout
		// make shure that we have at least one endpoint available
		// prevents hanging when model routing fails to find compatible backends
		if len(endpoints) == 0 {
			pr.requestLogger.Warn("No endpoints available for model",
				"model", pr.model,
				"translator", trans.Name())
			a.writeTranslatorError(w, trans, pr,
				fmt.Errorf("no healthy endpoints available for model: %s", pr.model),
				http.StatusNotFound)
			return
		}

		a.logRequestStart(pr, len(endpoints))

		// Execute proxy request with appropriate response handling
		// streaming vs non-streaming need different handling
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
