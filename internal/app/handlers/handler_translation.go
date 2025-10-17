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
)

// translationHandler creates a generic HTTP handler for any RequestTranslator
// This eliminates per-translator handler duplication by reusing the same proxy pipeline
// for all message format conversions (Anthropic, Gemini, Bedrock, etc.)
func (a *Application) translationHandler(trans translator.RequestTranslator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pr := a.initializeProxyRequest(r)
		ctx, r := a.setupRequestContext(r, pr.stats)

		// Transform incoming format (e.g., Anthropic) to OpenAI format
		// Translator extracts model name and streaming flag during transformation
		transformedReq, err := trans.TransformRequest(ctx, r)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, err, http.StatusBadRequest)
			return
		}

		// Use extracted metadata for routing and observability
		pr.model = transformedReq.ModelName
		pr.stats.Model = pr.model

		// Serialize OpenAI request for proxy
		openaiBody, err := json.Marshal(transformedReq.OpenAIRequest)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("failed to serialize request"), http.StatusInternalServerError)
			return
		}

		// Replace request body with OpenAI format
		r.Body = io.NopCloser(bytes.NewReader(openaiBody))
		r.ContentLength = int64(len(openaiBody))

		// Run through standard proxy pipeline (inspector, security, endpoint selection)
		a.analyzeRequest(ctx, r, pr)

		// Get compatible endpoints for this request
		endpoints, err := a.getCompatibleEndpoints(ctx, pr)
		if err != nil {
			a.writeTranslatorError(w, trans, pr, fmt.Errorf("no healthy endpoints available"), http.StatusServiceUnavailable)
			return
		}

		a.logRequestStart(pr, len(endpoints))

		// Execute proxy request with appropriate response handling
		// Streaming and non-streaming require different approaches
		var proxyErr error
		if transformedReq.IsStreaming {
			proxyErr = a.executeTranslatedStreamingRequest(ctx, w, r, endpoints, pr, trans)
		} else {
			proxyErr = a.executeTranslatedNonStreamingRequest(ctx, w, r, endpoints, pr, trans)
		}

		a.logRequestResult(pr, proxyErr)

		if proxyErr != nil {
			// Only write error if response hasn't started
			// Content-Type check prevents double-writing after partial stream
			if w.Header().Get(constants.HeaderContentType) == "" {
				a.writeTranslatorError(w, trans, pr, fmt.Errorf("proxy error: %w", proxyErr), http.StatusBadGateway)
			}
		}
	}
}

// executeTranslatedNonStreamingRequest handles non-streaming translation requests
// Captures complete OpenAI response, transforms to target format, writes to client
func (a *Application) executeTranslatedNonStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) error {
	recorder := newResponseRecorder()

	// Add model to context for routing
	if pr.model != "" {
		ctx = context.WithValue(ctx, "model", pr.model)
		r = r.WithContext(ctx)
	}

	// Pass routing decision to stats for headers
	if pr.profile != nil && pr.profile.RoutingDecision != nil {
		pr.stats.RoutingDecision = pr.profile.RoutingDecision
	}

	// Execute proxy request, capturing response
	err := a.proxyService.ProxyRequestToEndpoints(ctx, recorder, r, endpoints, pr.stats, pr.requestLogger)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}

	// Parse OpenAI response
	var openaiResp map[string]interface{}
	if jerr := json.Unmarshal(recorder.body.Bytes(), &openaiResp); jerr != nil {
		return fmt.Errorf("failed to parse OpenAI response: %w", jerr)
	}

	// Transform OpenAI response back to target format
	targetResp, err := trans.TransformResponse(ctx, openaiResp, r)
	if err != nil {
		return fmt.Errorf("failed to transform response: %w", err)
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)

	// Copy Olla observability headers from recorder
	// These provide insight into routing decisions and backend selection
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

// executeTranslatedStreamingRequest handles streaming translation requests
// Uses io.Pipe to connect proxy output stream to translator input for real-time conversion
func (a *Application) executeTranslatedStreamingRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	pr *proxyRequest,
	trans translator.RequestTranslator,
) error {
	// Create pipe connecting proxy output to translator input
	// Proxy writes OpenAI SSE to pipe writer
	// Translator reads from pipe reader and writes target format SSE to response
	pipeReader, pipeWriter := io.Pipe()

	// Create recorder that writes to pipe and captures headers
	// Headers needed for X-Olla-* observability even during streaming
	streamRecorder := newStreamingResponseRecorder(pipeWriter)

	// Start proxy request in background
	// Runs concurrently with translation process
	proxyErrChan := make(chan error, 1)
	go func() {
		localCtx := ctx
		localR := r

		if pr.model != "" {
			localCtx = context.WithValue(localCtx, "model", pr.model)
			localR = localR.WithContext(localCtx)
		}

		// Pass routing decision to stats for headers
		if pr.profile != nil && pr.profile.RoutingDecision != nil {
			pr.stats.RoutingDecision = pr.profile.RoutingDecision
		}

		err := a.proxyService.ProxyRequestToEndpoints(localCtx, streamRecorder, localR, endpoints, pr.stats, pr.requestLogger)
		pipeWriter.Close() // Signal end of stream
		proxyErrChan <- err
	}()

	// Wait for headers to be set by proxy before copying them
	// This avoids data race between header write (proxy) and header read (copy)
	<-streamRecorder.headersReady

	// Copy Olla observability headers before starting stream
	// Headers must be written before any body content
	a.copyOllaHeaders(streamRecorder, w)

	// Transform streaming response
	// Blocks until stream completes or errors
	transformErr := trans.TransformStreamingResponse(ctx, pipeReader, w, r)

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

// writeTranslatorError writes error response using translator's error format if available
// Falls back to generic JSON error if translator doesn't implement ErrorWriter interface
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

	// Check if translator implements custom error formatting
	// This allows each translator to use its API's error schema
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, err, statusCode)
		return
	}

	// Fallback to generic JSON error
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

// tokenCountHandler creates an HTTP handler for token counting endpoints
// This enables translators to provide token estimation without proxy overhead
// Only available for translators that implement the TokenCounter interface
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

// headerGetter abstracts header access for both response types
type headerGetter interface {
	Header() http.Header
}

// responseRecorder captures complete response for non-streaming requests
// Used when we need to inspect/transform the entire response body
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
	writer       io.Writer
	headers      http.Header
	headersReady chan struct{} // Signals when headers have been written
	headerSent   bool
}

func newStreamingResponseRecorder(w io.Writer) *streamingResponseRecorder {
	return &streamingResponseRecorder{
		headers:      make(http.Header),
		writer:       w,
		headersReady: make(chan struct{}),
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
	if !r.headerSent {
		r.headerSent = true
		close(r.headersReady) // Signal headers are ready
	}
	// We don't write status for streaming, just mark headers sent
}
