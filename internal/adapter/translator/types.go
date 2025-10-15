package translator

import (
	"context"
	"io"
	"net/http"
)

// RequestTranslator converts between API formats (e.g., Anthropic → OpenAI)
// This enables Olla to accept multiple API formats while using OpenAI format internally
type RequestTranslator interface {
	// TransformRequest converts an incoming request to OpenAI format
	// Returns the transformed request with metadata needed for response translation
	TransformRequest(ctx context.Context, r *http.Request) (*TransformedRequest, error)

	// TransformResponse converts an OpenAI response back to the original format
	// Uses the original request to maintain context (e.g., model name, metadata)
	TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error)

	// TransformStreamingResponse handles streaming response conversion
	// Reads OpenAI SSE stream and writes in the target format (e.g., Anthropic SSE)
	TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error

	// Name returns the translator identifier (e.g., "anthropic")
	Name() string
}

// TransformedRequest holds the converted request and metadata
// This preserves information needed to translate the response back to the original format
type TransformedRequest struct {
	OpenAIRequest map[string]interface{} // Converted OpenAI format request body
	Metadata      map[string]interface{} // Additional context for response translation
	ModelName     string                 // Extracted model name for routing
	OriginalBody  []byte                 // Original request body for response translation context
	IsStreaming   bool                   // Whether response should stream
}

// PathProvider is an optional interface that translators can implement to define
// their API endpoints. This enables dynamic route registration without hardcoding
// paths in the application layer. If not implemented, routes must be registered manually.
type PathProvider interface {
	GetAPIPath() string // Returns the API path (e.g., "/olla/anthropic/v1/messages")
}

// ErrorWriter is an optional interface that translators can implement to format
// errors according to their API's error schema. This ensures error responses match
// the expected format for each translator (e.g., Anthropic's error structure).
// If not implemented, generic JSON errors are used as a fallback.
type ErrorWriter interface {
	WriteError(w http.ResponseWriter, err error, statusCode int)
}
