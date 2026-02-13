package ports

import (
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

type StatsCollector interface {
	RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64)
	RecordConnection(endpoint *domain.Endpoint, delta int) // +1 connect, -1 disconnect
	RecordSecurityViolation(violation SecurityViolation)
	RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration)

	// Model-specific tracking
	RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64)
	RecordModelError(model string, endpoint *domain.Endpoint, errorType string)
	GetModelStats() map[string]ModelStats
	GetModelEndpointStats() map[string]map[string]EndpointModelStats

	// Translator-specific tracking
	RecordTranslatorRequest(event TranslatorRequestEvent)
	GetTranslatorStats() map[string]TranslatorStats

	GetProxyStats() ProxyStats
	GetEndpointStats() map[string]EndpointStats
	GetSecurityStats() SecurityStats
	GetConnectionStats() map[string]int64
}

type EndpointStats struct {
	Name               string  `json:"name"`
	URL                string  `json:"url"`
	ActiveConnections  int64   `json:"active_connections"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	TotalBytes         int64   `json:"total_bytes"`
	AverageLatency     int64   `json:"avg_latency_ms"`
	MinLatency         int64   `json:"min_latency_ms"`
	MaxLatency         int64   `json:"max_latency_ms"`
	LastUsedNano       int64   `json:"last_used_nano"`
	SuccessRate        float64 `json:"success_rate_percent"`
}

type SecurityStats struct {
	RateLimitViolations  int64 `json:"rate_limit_violations"`
	SizeLimitViolations  int64 `json:"size_limit_violations"`
	UniqueRateLimitedIPs int   `json:"unique_rate_limited_ips"`
}

// ModelStats tracks usage and performance for a specific model
type ModelStats struct {
	LastRequested time.Time `json:"last_requested"`

	Name               string `json:"name"`
	TotalRequests      int64  `json:"total_requests"`
	SuccessfulRequests int64  `json:"successful_requests"`
	FailedRequests     int64  `json:"failed_requests"`
	TotalBytes         int64  `json:"total_bytes"`
	AverageLatency     int64  `json:"avg_latency_ms"`
	P95Latency         int64  `json:"p95_latency_ms"`
	P99Latency         int64  `json:"p99_latency_ms"`
	UniqueClients      int64  `json:"unique_clients"`

	// Routing effectiveness
	RoutingHits      int64 `json:"routing_hits"`      // Found on first endpoint
	RoutingMisses    int64 `json:"routing_misses"`    // Had to retry other endpoints
	RoutingFallbacks int64 `json:"routing_fallbacks"` // Used fallback model
}

// EndpointModelStats tracks how well a specific model performs on a specific endpoint
type EndpointModelStats struct {
	LastUsed          time.Time `json:"last_used"`
	EndpointName      string    `json:"endpoint_name"`
	ModelName         string    `json:"model_name"`
	RequestCount      int64     `json:"request_count"`
	SuccessRate       float64   `json:"success_rate"`
	AverageLatency    int64     `json:"avg_latency_ms"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
}

// TranslatorRequestEvent captures metrics for a single translator request
type TranslatorRequestEvent struct {
	TranslatorName string                             // e.g. "anthropic"
	Model          string                             // requested model
	Mode           constants.TranslatorMode           // passthrough or translation
	FallbackReason constants.TranslatorFallbackReason // why passthrough wasn't used
	Success        bool                               // whether request succeeded
	IsStreaming    bool                               // streaming vs non-streaming
	Latency        time.Duration                      // end-to-end request duration (placed first for optimal alignment)
}

// TranslatorStats aggregates metrics for a specific translator
type TranslatorStats struct {
	TranslatorName string `json:"translator_name"`

	// Total request counts
	TotalRequests      int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`

	// Mode breakdown
	PassthroughRequests int64 `json:"passthrough_requests"` // native format used
	TranslationRequests int64 `json:"translation_requests"` // format conversion required

	// Streaming breakdown
	StreamingRequests    int64 `json:"streaming_requests"`
	NonStreamingRequests int64 `json:"non_streaming_requests"`

	// Fallback reasons (when passthrough couldn't be used)
	FallbackNoCompatibleEndpoints               int64 `json:"fallback_no_compatible_endpoints"`
	FallbackTranslatorDoesNotSupportPassthrough int64 `json:"fallback_translator_does_not_support_passthrough"`
	FallbackCannotPassthrough                   int64 `json:"fallback_cannot_passthrough"`

	// Performance metrics
	AverageLatency int64 `json:"avg_latency_ms"`
	TotalLatency   int64 `json:"total_latency_ms"`
}
