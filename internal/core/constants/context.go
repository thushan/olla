package constants

const (
	ContextRoutePrefixKey  = "route_prefix"  // we inject this into the context to allow stripping prefixes for proxy calls
	ContextRequestIdKey    = "request_id"    // generataed each proxy_handler request for the request ID
	ContextRequestTimeKey  = "request_time"  // generated each proxy_handler request to track the time taken for the request
	ContextOriginalPathKey = "original_path" // original path before any modifications, useful for logging/debugging
	ContextKeyStream       = "stream"        // indicates whether the response should be streamed or buffered
	ContextProviderTypeKey = "provider_type" // the provider type for the request, used for routing and load balancing

	// ContextModelAliasMapKey stores a map[string]string of endpoint URL → actual model name
	// when a model alias is resolved, allowing the proxy to rewrite the model name in the
	// request body to match what the selected backend expects
	ContextModelAliasMapKey = "model_alias_map"
)
