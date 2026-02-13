package translator

import (
	"context"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/core/domain"
)

// converts between api formats (e.g., anthropic → openai)
// lets olla accept multiple formats while using openai internally
type RequestTranslator interface {
	// converts incoming request to openai format
	// returns transformed request with metadata for response translation
	TransformRequest(ctx context.Context, r *http.Request) (*TransformedRequest, error)

	// converts openai response back to original format
	// uses original request to keep context (model, metadata)
	TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error)

	// handles streaming response conversion
	// reads openai sse stream and writes target format
	TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error

	// returns translator name (eg "anthropic")
	Name() string
}

// holds converted request and metadata
// preserving info for response translation
type TransformedRequest struct {
	OpenAIRequest map[string]interface{} // Converted OpenAI format request body
	Metadata      map[string]interface{} // Additional context for response translation
	ModelName     string                 // Extracted model name for routing
	TargetPath    string                 // Target API path for the backend (e.g., "/v1/chat/completions" - proxy handles /olla prefix)
	OriginalBody  []byte                 // Original request body for response translation context
	IsStreaming   bool                   // Whether response should stream
}

// optional interface for translators to define their api endpoints
// if not implemented, routes need manual registration
type PathProvider interface {
	GetAPIPath() string // Returns the API path (e.g., "/olla/anthropic/v1/messages")
}

// optional interface for custom error formatting per translator (eg anthropic error structure)
// falls back to generic json if not implemented
type ErrorWriter interface {
	WriteError(w http.ResponseWriter, err error, statusCode int)
}

// optional interface for translators that support token counting
// enables api compatibility with token estimation endpoints
type TokenCounter interface {
	CountTokens(ctx context.Context, r *http.Request) (*TokenCountResponse, error)
}

// represents token count result
type TokenCountResponse struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// optional interface for translators that provide model listings
// exposes available models in their native api format
type ModelsProvider interface {
	GetModels(ctx context.Context) (interface{}, error)
}

// PassthroughCapable is an optional interface for translators that can bypass
// the translation pipeline entirely. When a backend natively speaks the same
// wire format as the incoming request (e.g. a vLLM instance with Anthropic
// Messages API support), the request can be forwarded directly -- avoiding
// the marshalling overhead of Anthropic->OpenAI->Anthropic round-trips.
//
// The handler checks CanPassthrough first; if it returns true, it calls
// PreparePassthrough to obtain the body and target path, then forwards the
// request to the backend without any translation.
//
// This is intentionally a separate interface rather than a method on
// RequestTranslator so that existing translators remain unaffected and the
// passthrough decision is opt-in per translator.
type PassthroughCapable interface {
	// CanPassthrough inspects the available endpoints (via their profile
	// configurations) and determines whether at least one backend can accept
	// the request in its native format without translation.
	//
	// The profileLookup parameter provides access to per-endpoint-type
	// AnthropicSupportConfig without creating a hard dependency on the
	// profile registry. This keeps the translator layer decoupled from
	// the infrastructure layer.
	//
	// Thread-safe: implementations must not mutate the endpoints slice.
	CanPassthrough(endpoints []*domain.Endpoint, profileLookup ProfileLookup) bool

	// PreparePassthrough validates the already-buffered request body for
	// passthrough eligibility and returns the target backend path, model
	// name, and streaming flag.
	//
	// bodyBytes is the raw request body already read by the handler. This
	// avoids a redundant io.ReadAll inside the implementation -- the handler
	// buffers the body once for model extraction and reuses it here.
	//
	// The returned targetPath is the backend-relative path (e.g.
	// "/v1/messages") that the proxy layer should use when forwarding.
	//
	// Returns an error if the body is invalid for passthrough (e.g.
	// exceeds size limits or uses features the backend doesn't support).
	PreparePassthrough(bodyBytes []byte, r *http.Request, profileLookup ProfileLookup) (*PassthroughRequest, error)
}

// PassthroughRequest holds the result of preparing a request for direct
// forwarding to a backend. Separating this into its own struct (rather than
// returning multiple values) makes it easier to extend in future phases --
// for example, adding header overrides or endpoint filtering hints.
type PassthroughRequest struct {

	// TargetPath is the backend-relative API path (e.g. "/v1/messages").
	// The proxy layer prepends any necessary prefixes.
	TargetPath string

	// ModelName is extracted from the request body for routing and
	// observability (populates X-Olla-Model header).
	ModelName string

	// Body is the original, unmodified request body bytes. The caller
	// should set r.Body = io.NopCloser(bytes.NewReader(Body)) before
	// forwarding to the proxy pipeline.
	Body []byte

	// IsStreaming indicates whether the request has stream:true set,
	// so the handler can select the appropriate response pipeline.
	IsStreaming bool
}

// BodySizeLimiter is an optional interface for translators that declare their
// maximum acceptable request body size. The handler uses this to apply a
// per-translator limit when reading the body, rather than hardcoding a value.
// If a translator does not implement this interface, the handler falls back
// to a sensible default (10 MiB).
type BodySizeLimiter interface {
	MaxBodySize() int64
}

// ProfileLookup provides access to backend AnthropicSupportConfig without
// coupling the translator layer to the profile registry implementation.
// This interface lives in the translator package because it's consumed by
// PassthroughCapable implementations -- the profile registry in the adapter
// layer provides the concrete implementation.
//
// Designed to be easily mockable for testing: a single method, no side
// effects, and a return value that's safe to compare against nil.
type ProfileLookup interface {
	// GetAnthropicSupport returns the AnthropicSupportConfig for the given
	// endpoint type (e.g. "vllm", "sglang", "litellm"). Returns nil if the
	// profile doesn't exist or doesn't declare Anthropic support.
	//
	// The endpointType parameter corresponds to domain.Endpoint.Type, which
	// maps to the profile name loaded from config/profiles/*.yaml.
	GetAnthropicSupport(endpointType string) *domain.AnthropicSupportConfig
}
