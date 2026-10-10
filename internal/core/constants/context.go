package constants

// contextKey is a private type for context keys to prevent collisions with other packages.
type contextKey string

const (
	ContextRoutePrefixKey  = "route_prefix"  // we inject this into the context to allow stripping prefixes for proxy calls
	ContextRequestIdKey    = "request_id"    // generataed each proxy_handler request for the request ID
	ContextRequestTimeKey  = "request_time"  // generated each proxy_handler request to track the time taken for the request
	ContextOriginalPathKey = "original_path" // original path before any modifications, useful for logging/debugging
	ContextKeyStream       = "stream"        // indicates whether the response should be streamed or buffered
	ContextProviderTypeKey = "provider_type" // the provider type for the request, used for routing and load balancing

	// ContextModelKey carries the resolved model name through the proxy pipeline.
	// Using a typed key prevents accidental collisions with plain-string keys from
	// third-party middleware that might also use "model".
	ContextModelKey = contextKey("model")

	// Sticky session context keys — set by the handler before balancer selection
	// and read back after to surface affinity decisions in response headers.
	ContextStickyKeyKey       = contextKey("sticky-key")        // computed affinity key for this request
	ContextStickyKeySourceKey = contextKey("sticky-key-source") // which source produced the key
	ContextStickyOutcomeKey   = contextKey("sticky-outcome")    // *StickyOutcome written by the wrapper

	// ContextEndpointOverrideKey carries a client-supplied X-Olla-Endpoint value;
	// the sticky wrapper honours it as a hard selection and re-pins the session to it.
	ContextEndpointOverrideKey = contextKey("endpoint-override")

	// ContextModelAliasMapKey stores a map[string]string of endpoint URL → actual model name
	// when a model alias is resolved, allowing the proxy to rewrite the model name in the
	// request body to match what the selected backend expects
	ContextModelAliasMapKey = "model_alias_map"

	// ContextInputTokensKey carries a prompt token estimate through the translation
	// pipeline so the streaming translator can populate input_tokens in message_start
	// before the upstream usage chunk arrives (which only comes at the end of the stream).
	ContextInputTokensKey = contextKey("input-tokens")
)
