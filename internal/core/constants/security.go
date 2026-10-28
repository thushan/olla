package constants

const (
	ViolationRateLimit = "rate_limit"
	ViolationSizeLimit = "size_limit"

	// Rejection reasons carried on ports.SecurityResult. Callers and tests compare
	// these, so they are constants rather than literals repeated at each return
	// site: a reason that drifts out of sync fails silently, since a comparison
	// that never matches looks the same as a rejection that never happened.
	ReasonRateLimitExceeded       = "Rate limit exceeded"
	ReasonGlobalRateLimitExceeded = "Global rate limit exceeded"

	// MaxUpstreamErrorBodyBytes caps how many bytes we read from an upstream error
	// response before discarding the rest. A misbehaving or compromised backend
	// could return an arbitrarily large body; reading it all into memory would
	// exhaust the heap. 1 MiB is enough to capture any reasonable error message.
	MaxUpstreamErrorBodyBytes = 1 << 20 // 1 MiB

	// HeaderXOllaPrefix is the canonical prefix for all Olla-owned response headers.
	// Used to strip backend-supplied spoofs from the upstream→client copy path so
	// Olla is the sole authority for these headers.
	HeaderXOllaPrefix = "X-Olla-"
)
