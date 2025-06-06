package constants

const (
	ProxyPathPrefix = "route_prefix" // we inject this into the context to allow stripping prefixes for proxy calls
	RequestIDKey    = "request_id"   // generataed each proxy_handler request for the request ID
	RequestTimeKey  = "request_time" // generated each proxy_handler request to track the time taken for the request
)
