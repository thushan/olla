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

	// DefaultHalfOpenProbeStaleness bounds how long a single in-flight
	// half-open probe can gate out further probes before the slot is handed
	// to a replacement caller. This governs health-check probes, not proxied
	// inference requests (see proxy/olla's much larger halfOpenStaleness for
	// that distinction): a /health probe is sub-second by design, so a
	// probe still "in flight" a full second later is almost certainly hung,
	// not just slow.
	DefaultHalfOpenProbeStaleness = time.Second

	// Alias the shared constants for backward compatibility
	MaxBackoffMultiplier = constants.DefaultMaxBackoffMultiplier
	MaxBackoffSeconds    = constants.DefaultMaxBackoffSeconds
)
