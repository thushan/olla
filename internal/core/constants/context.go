package constants

const (
	ContextRoutePrefixKey  = "route_prefix"  // we inject this into the context to allow stripping prefixes for proxy calls
	ContextRequestIdKey    = "request_id"    // generataed each proxy_handler request for the request ID
	ContextRequestTimeKey  = "request_time"  // generated each proxy_handler request to track the time taken for the request
	ContextOriginalPathKey = "original_path" // original path before any modifications, useful for logging/debugging
	ContextKeyStream       = "stream"        // indicates whether the response should be streamed or buffered
)
