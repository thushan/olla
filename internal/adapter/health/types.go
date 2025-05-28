package health

import "time"

const (
	DefaultHealthCheckerTimeout = 5 * time.Second
	SlowResponseThreshold       = 10 * time.Second

	HealthyEndpointStatusRangeStart = 200
	HealthyEndpointStatusRangeEnd   = 300

	DefaultCircuitBreakerThreshold = 3
	DefaultCircuitBreakerTimeout   = 30 * time.Second

	MaxBackoffMultiplier = 12
)
