package health

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
)

const (
	DefaultHealthCheckerTimeout = 5 * time.Second
	SlowResponseThreshold       = 10 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 3
	DefaultCircuitBreakerTimeout   = 30 * time.Second

	// Alias the shared constants for backward compatibility
	MaxBackoffMultiplier = constants.DefaultMaxBackoffMultiplier
	MaxBackoffSeconds    = constants.DefaultMaxBackoffSeconds
)
