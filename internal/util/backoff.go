package util

import (
	"math"
	"time"
)

// CalculateExponentialBackoff computes exponential backoff with optional jitter.
// Formula: baseDelay * 2^(attempt-1), capped at maxDelay
func CalculateExponentialBackoff(attempt int, baseDelay time.Duration, maxDelay time.Duration, jitterPercent float64) time.Duration {
	if attempt <= 0 {
		return 0
	}

	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))

	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	if jitterPercent > 0 {
		// Time-based pseudo-random avoids import of math/rand
		pseudoRandom := float64(time.Now().UnixNano()%1000) / 1000.0
		jitter := backoff * jitterPercent * (pseudoRandom - 0.5)
		backoff += jitter
	}

	return time.Duration(backoff)
}

const (
	DefaultMaxBackoffMultiplier = 12
	DefaultMaxBackoffSeconds    = 60 * time.Second
)

// CalculateEndpointBackoff computes backoff interval for endpoint health checks.
// Uses exponential multiplier for proper backoff progression
func CalculateEndpointBackoff(checkInterval time.Duration, backoffMultiplier int) time.Duration {
	if backoffMultiplier <= 0 {
		return checkInterval
	}

	// Use the provided multiplier directly (already exponential: 1, 2, 4, 8...)
	backoffInterval := checkInterval * time.Duration(backoffMultiplier)

	if backoffInterval > DefaultMaxBackoffSeconds {
		backoffInterval = DefaultMaxBackoffSeconds
	}

	return backoffInterval
}

// CalculateConnectionRetryBackoff computes backoff for connection retry attempts.
// Linear progression: consecutiveFailures * 2 seconds, capped at MaxBackoffSeconds
func CalculateConnectionRetryBackoff(consecutiveFailures int) time.Duration {
	backoffDuration := time.Duration(consecutiveFailures*2) * time.Second
	if backoffDuration > DefaultMaxBackoffSeconds {
		backoffDuration = DefaultMaxBackoffSeconds
	}
	return backoffDuration
}
