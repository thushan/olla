package health

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
)

const (
	DefaultHealthCheckerTimeout = 5 * time.Second
	SlowResponseThreshold       = 10 * time.Second

	// DefaultHealthCheckerResponseHeaderTimeout caps the time a backend may hold
	// the connection open after accepting without sending a single response header.
	// Shorter than the proxy equivalent; health probes are latency-sensitive.
	DefaultHealthCheckerResponseHeaderTimeout = 10 * time.Second

	// DefaultRateLimitBackoff is used when a 429 carries no Retry-After header.
	DefaultRateLimitBackoff = 30 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 3
	DefaultCircuitBreakerTimeout   = 30 * time.Second

	// defaultCheckTimeoutFallback is the checkTimeout probeStaleness (in
	// circuit_breaker.go) falls back to when a caller has no real endpoint
	// context (e.g. a test) to derive the half-open staleness window from.
	// Mirrors config.go's own default for Endpoint.CheckTimeout, so the
	// fallback matches what an unconfigured endpoint would actually have.
	defaultCheckTimeoutFallback = 2 * time.Second

	// Alias the shared constants for backward compatibility
	MaxBackoffMultiplier = constants.DefaultMaxBackoffMultiplier
	MaxBackoffSeconds    = constants.DefaultMaxBackoffSeconds
)
