package constants

import "time"

// Retry and backoff constants
const (
	// Maximum backoff multiplier for exponential backoff (1, 2, 4, 8, 12)
	DefaultMaxBackoffMultiplier = 12
	
	// Maximum backoff duration for health checks and retries
	DefaultMaxBackoffSeconds = 60 * time.Second
	
	// Default base interval for retry attempts
	DefaultRetryInterval = 2 * time.Second
	
	// Connection retry backoff multiplier (linear: failures * 2 seconds)
	ConnectionRetryBackoffMultiplier = 2
)