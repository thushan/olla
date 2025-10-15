package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// anthropicHandler processes Anthropic API requests
// Translates Anthropic format to OpenAI, routes through the proxy pipeline,
// then translates the response back to Anthropic format, this is best effort
// for tools like Claude Code to route through existing OpenAI compatible EPs.
func (a *Application) anthropicHandler(w http.ResponseWriter, r *http.Request) {
	pr := a.initializeProxyRequest(r)
	ctx, r := a.setupRequestContext(r, pr.stats)
	translator := anthropic.NewTranslator(pr.requestLogger)

	// translate Anthropic request to OpenAI format
	// we parse request body, extracting model and streaming infos
	transformedReq, err := translator.TransformRequest(ctx, r)
	if err != nil {
		a.writeAnthropicError(w, pr, "invalid_request_error", err.Error(), http.StatusBadRequest)
		return
	}

	// translator determines model name and streaming mode during transformation
	// we use that in the new request
	pr.model = transformedReq.ModelName
	pr.stats.Model = pr.model

	// reconstruct
	openaiBody, err := json.Marshal(transformedReq.OpenAIRequest)
	if err != nil {
		a.writeAnthropicError(w, pr, "internal_error", "failed to serialize request", http.StatusInternalServerError)
		return
	}

	// Replace request body with OpenAI format
	r.Body = io.NopCloser(bytes.NewReader(openaiBody))
	r.ContentLength = int64(len(openaiBody))

	// Continue with standard proxy flow
	a.analyzeRequest(ctx, r, pr)

	// Get compatible endpoints
	// The inspector chain has already identified which endpoints can handle OpenAI requests
	endpoints, err := a.getCompatibleEndpoints(ctx, pr)
	if err != nil {
		a.writeAnthropicError(w, pr, "overloaded_error", "no healthy endpoints available", http.StatusServiceUnavailable)
		return
	}

	a.logRequestStart(pr, len(endpoints))

	// Execute proxy request with response capture or streaming
	// The approach differs based on whether the response should stream
	var proxyErr error
	if transformedReq.IsStreaming {
		proxyErr = a.executeAnthropicStreamingRequest(ctx, w, r, endpoints, pr, translator)
	} else {
		proxyErr = a.executeAnthropicNonStreamingRequest(ctx, w, r, endpoints, pr, translator)
	}

	a.logRequestResult(pr, proxyErr)

	if proxyErr != nil {
		// Only write error if we haven't started streaming
		// content-type check prevents double-writing response after partial stream
		// there are times when this fails spectacularly
		if w.Header().Get(constants.HeaderContentType) == "" {
			a.writeAnthropicError(w, pr, "api_error", fmt.Sprintf("proxy error: %v", proxyErr), http.StatusBadGateway)
		}
	}
}

// executeAnthropicNonStreamingRequest handles non-streaming Anthropic requests
// Captures the OpenAI response, translates to Anthropic format, then writes to client
func (a *Application) executeAnthropicNonStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	translator *anthropic.Translator,
) error {
	recorder := newResponseRecorder()

	if pr.model != "" {
		ctx = context.WithValue(ctx, "model", pr.model)
		r = r.WithContext(ctx)
	}

	if pr.profile != nil && pr.profile.RoutingDecision != nil {
		pr.stats.RoutingDecision = pr.profile.RoutingDecision
	}

	err := a.proxyService.ProxyRequestToEndpoints(ctx, recorder, r, endpoints, pr.stats, pr.requestLogger)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}

	var openaiResp map[string]interface{}
	if jerr := json.Unmarshal(recorder.body.Bytes(), &openaiResp); jerr != nil {
		return fmt.Errorf("failed to parse OpenAI response: %w", jerr)
	}

	anthropicResp, err := translator.TransformResponse(ctx, openaiResp, r)
	if err != nil {
		return fmt.Errorf("failed to transform response: %w", err)
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)

	// Copy Olla-specific headers from recorder
	// they headers provide observability into the proxy routing decision
	a.copyOllaHeaders(recorder, w)

	respBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(respBody); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}

// executeAnthropicStreamingRequest handles streaming Anthropic requests
// Uses io.Pipe to connect proxy output stream to translator input for real-time translation
func (a *Application) executeAnthropicStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	translator *anthropic.Translator,
) error {
	// Create pipe to connect proxy output to translator input
	// The proxy writes OpenAI SSE to the pipe writer,
	// the translator reads from pipe reader and writes Anthropic SSE to response
	pipeReader, pipeWriter := io.Pipe()

	// Create recorder that writes to both the pipe and captures headers
	// We need headers for X-Olla-* observability even during streaming
	streamRecorder := newStreamingResponseRecorder(pipeWriter)

	// Start proxy request in background
	// This runs concurrently with the translation process
	proxyErrChan := make(chan error, 1)
	go func() {
		if pr.model != "" {
			ctx = context.WithValue(ctx, "model", pr.model)
			r = r.WithContext(ctx)
		}

		// Pass routing decision to stats for headers
		if pr.profile != nil && pr.profile.RoutingDecision != nil {
			pr.stats.RoutingDecision = pr.profile.RoutingDecision
		}

		err := a.proxyService.ProxyRequestToEndpoints(ctx, streamRecorder, r, endpoints, pr.stats, pr.requestLogger)
		pipeWriter.Close() // Signal end of stream
		proxyErrChan <- err
	}()

	// Copy Olla-specific headers before starting stream
	// Headers must be written before any body content
	a.copyOllaHeaders(streamRecorder, w)

	// Transform streaming response
	// This blocks until the stream completes or errors
	transformErr := translator.TransformStreamingResponse(ctx, pipeReader, w, r)

	// Wait for proxy to complete
	proxyErr := <-proxyErrChan

	// Return first error encountered
	// Transform errors take precedence as they indicate client-visible issues
	if transformErr != nil {
		return fmt.Errorf("stream transformation failed: %w", transformErr)
	}
	if proxyErr != nil {
		return fmt.Errorf("proxy request failed: %w", proxyErr)
	}

	return nil
}

// copyOllaHeaders copies observability headers from recorder to response
// These headers provide insight into routing decisions and backend selection
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

// writeAnthropicError writes an error response in Anthropic format
// Maps error types and status codes to Anthropic's error schema
func (a *Application) writeAnthropicError(w http.ResponseWriter, pr *proxyRequest, errorType, message string, statusCode int) {
	pr.requestLogger.Error("Anthropic request failed",
		"error_type", errorType,
		"message", message,
		"status", statusCode)

	// Anthropic error format
	// See: https://docs.anthropic.com/claude/reference/errors
	errorResp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		pr.requestLogger.Error("Failed to write error response", "error", err)
	}
}

// responseRecorder captures HTTP response for transformation
// Used for non-streaming responses where we need the complete response before translating
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

// streamingResponseRecorder captures headers while forwarding body to a pipe
// Used for streaming responses where we need headers but must forward body immediately
type streamingResponseRecorder struct {
	headers    http.Header
	writer     io.Writer
	headerSent bool
}

func newStreamingResponseRecorder(w io.Writer) *streamingResponseRecorder {
	return &streamingResponseRecorder{
		headers: make(http.Header),
		writer:  w,
	}
}

func (r *streamingResponseRecorder) Header() http.Header {
	return r.headers
}

func (r *streamingResponseRecorder) Write(data []byte) (int, error) {
	r.headerSent = true
	return r.writer.Write(data)
}

func (r *streamingResponseRecorder) WriteHeader(statusCode int) {
	r.headerSent = true
	// We don't write status for streaming, just mark headers sent
}

// headerGetter abstracts header access for both response types
type headerGetter interface {
	Header() http.Header
}
