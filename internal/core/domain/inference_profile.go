package domain

import "time"

// InferenceProfile extends PlatformProfile with model-aware capabilities.
// We embed PlatformProfile to avoid breaking thousands of lines of existing code
// while laying the groundwork for smarter routing decisions.
type InferenceProfile interface {
	PlatformProfile

	// Ollama can take 5+ minutes to load a 70B model from disk,
	// while OpenAI typically responds in seconds. This matters for
	// circuit breakers and user experience.
	GetTimeout() time.Duration

	// LM Studio chokes on concurrent requests (it's single-threaded),
	// but cloud providers can handle hundreds. We need to respect these limits
	// to avoid overwhelming local instances.
	GetMaxConcurrentRequests() int

	// Some platforms have quirks - like requiring specific headers or
	// API key formats. This gives each profile a chance to validate
	// before we waste time on doomed requests.
	ValidateEndpoint(endpoint *Endpoint) error

	// Local instances should generally be preferred over cloud endpoints
	// (lower latency, no egress costs). This provides sensible defaults
	// when users don't explicitly configure priorities.
	GetDefaultPriority() int

	// Escape hatch for platform-specific settings that don't deserve
	// their own interface method. Prevents interface bloat.
	GetConfig() *ProfileConfig
}
