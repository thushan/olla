package anthropic

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

// Translator converts between Anthropic and OpenAI API formats
// Uses buffer pooling to minimise memory allocations during translation
type Translator struct {
	logger         logger.StyledLogger
	bufferPool     *pool.Pool[*bytes.Buffer]
	config         config.AnthropicTranslatorConfig
	maxMessageSize int64 // derived from config
	inspector      *inspector.Simple
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
