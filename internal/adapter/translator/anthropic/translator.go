package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

// Translator converts between Anthropic and OpenAI API formats
// Uses buffer pooling to minimise memory allocations during translation
type Translator struct {
	logger         logger.StyledLogger
	bufferPool     *pool.Pool[*bytes.Buffer]
	inspector      *inspector.Simple
	config         config.AnthropicTranslatorConfig
	maxMessageSize int64 // derived from config
}

// NewTranslator creates a new Anthropic translator instance
// Uses a buffer pool to reduce GC pressure during high-throughput operations
// Accepts configuration for request size limits and streaming behaviour
func NewTranslator(log logger.StyledLogger, cfg config.AnthropicTranslatorConfig) *Translator {
	// Create buffer pool with 4KB initial capacity
	// This size fits most chat completions without reallocation
	bufferPool, err := pool.NewLitePool(func() *bytes.Buffer {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	})
	if err != nil {
		// This should never happen as the constructor is validated
		log.Error("Failed to create buffer pool", "error", err)
		panic("translator: failed to initialise buffer pool")
	}

	// Apply defaults if needed
	maxSize := cfg.MaxMessageSize
	if maxSize <= 0 {
		maxSize = 10 << 20 // 10MB default
		log.Warn("Invalid or missing max_message_size, using default", "default", maxSize)
	}

	// Create inspector for debugging
	insp := inspector.NewSimple(
		cfg.Inspector.Enabled,
		cfg.Inspector.OutputDir,
		cfg.Inspector.SessionHeader,
		log,
	)

	return &Translator{
		logger:         log,
		bufferPool:     bufferPool,
		config:         cfg,
		maxMessageSize: maxSize,
		inspector:      insp,
	}
}

// Name returns the translator identifier
func (t *Translator) Name() string {
	return "anthropic"
}

// GetAPIPath implements PathProvider interface for dynamic route registration
// Returns the Anthropic Messages API endpoint path
func (t *Translator) GetAPIPath() string {
	return "/olla/anthropic/v1/messages"
}

// MaxBodySize implements BodySizeLimiter so the handler can apply the
// translator's configured limit when reading the request body, rather
// than hardcoding a value.
func (t *Translator) MaxBodySize() int64 {
	return t.maxMessageSize
}

// getSessionID extracts the session ID from the request using a fallback chain.
// this ensures we always have a valid session id for request tracking:
// 1. try the configured session header (for custom session management)
// 2. if there's none, fall back to X-Request-ID (standard request correlation)
// 3. if that's still no go we, use default constant
// the chain exists because different callers may use different correlation mechanisms.
func (t *Translator) getSessionID(r *http.Request) string {
	const HeaderRequestId = "X-Request-ID"

	sessionID := r.Header.Get(t.inspector.GetSessionHeader())
	if sessionID == "" {
		sessionID = r.Header.Get(HeaderRequestId)
		if sessionID == "" {
			sessionID = defaultSessionID
		}
	}

	return sessionID
}

// forEachSystemContentBlock iterates over system prompt content blocks regardless of input format.
// This is a convenience wrapper around the standalone forEachSystemBlock function.
// See forEachSystemBlock in token_count.go for the implementation details.
func (t *Translator) forEachSystemContentBlock(system interface{}, fn func(block ContentBlock) error) error {
	return forEachSystemBlock(system, fn)
}

// WriteError implements ErrorWriter interface for Anthropic-specific error formatting
// Formats errors according to Anthropic's error schema
// See: https://docs.anthropic.com/claude/reference/errors
func (t *Translator) WriteError(w http.ResponseWriter, err error, statusCode int) {
	t.logger.Error("Anthropic request failed",
		"error", err.Error(),
		"status", statusCode)

	// Map HTTP status codes to Anthropic error types
	errorType := "api_error"
	switch statusCode {
	case http.StatusBadRequest:
		errorType = "invalid_request_error"
	case http.StatusUnauthorized:
		errorType = "authentication_error"
	case http.StatusForbidden:
		errorType = "permission_error"
	case http.StatusNotFound:
		errorType = "not_found_error"
	case http.StatusTooManyRequests:
		errorType = "rate_limit_error"
	case http.StatusServiceUnavailable:
		errorType = "overloaded_error"
	}

	// Anthropic error format
	errorResp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": err.Error(),
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(statusCode)

	if encErr := json.NewEncoder(w).Encode(errorResp); encErr != nil {
		t.logger.Error("Failed to write error response", "error", encErr)
	}
}

// CanPassthrough implements PassthroughCapable interface
// Determines whether the request can be forwarded directly to backends without translation.
// Returns true only if passthrough is enabled and ALL endpoints declare native Anthropic support.
func (t *Translator) CanPassthrough(endpoints []*domain.Endpoint, profileLookup translator.ProfileLookup) bool {
	// Fast path: if passthrough is disabled, no need to check endpoints
	if !t.config.PassthroughEnabled {
		return false
	}

	// If we have no endpoints, cannot passthrough
	if len(endpoints) == 0 {
		return false
	}

	// Check all endpoints for native Anthropic support
	// All endpoints must support passthrough - if any endpoint doesn't support it,
	// we must fall back to translation to ensure the request can be routed to any backend
	for _, ep := range endpoints {
		support := profileLookup.GetAnthropicSupport(ep.Type)

		// If support is nil or explicitly disabled, cannot passthrough
		if support == nil || !support.Enabled {
			t.logger.Debug("Endpoint does not support Anthropic passthrough",
				"endpoint", ep.Name,
				"type", ep.Type)
			return false
		}
	}

	t.logger.Debug("All endpoints support Anthropic passthrough", "count", len(endpoints))
	return true
}

// PreparePassthrough implements PassthroughCapable interface.
// Validates the already-buffered request body for direct forwarding to backends.
// Returns the original body bytes, target path, model name, and streaming flag.
// profileLookup is reserved for future per-endpoint path customisation.
func (t *Translator) PreparePassthrough(bodyBytes []byte, r *http.Request, _ translator.ProfileLookup) (*translator.PassthroughRequest, error) {
	// Enforce the translator's size limit on the pre-buffered body.
	// The handler applies its own LimitReader when reading, but we guard
	// here as well so the translator's configured limit is authoritative.
	if int64(len(bodyBytes)) > t.maxMessageSize {
		return nil, fmt.Errorf("request body exceeds maximum size (%d bytes)", t.maxMessageSize)
	}

	// Validate the request structure
	var anthropicReq AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		return nil, fmt.Errorf("invalid Anthropic request: %w", err)
	}

	// Validate required fields and constraints
	if err := anthropicReq.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	// Log request to inspector if enabled
	if t.inspector.Enabled() {
		sessionID := t.getSessionID(r)
		if lerr := t.inspector.LogRequest(sessionID, anthropicReq.Model, bodyBytes); lerr != nil {
			t.logger.Warn("Failed to log request to inspector", "error", lerr)
		}
	}

	t.logger.Debug("Prepared request for passthrough",
		"model", anthropicReq.Model,
		"streaming", anthropicReq.Stream,
		"body_size", len(bodyBytes))

	return &translator.PassthroughRequest{
		Body:        bodyBytes,
		TargetPath:  "/v1/messages",
		ModelName:   anthropicReq.Model,
		IsStreaming: anthropicReq.Stream,
	}, nil
}
