package metrics

import (
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

// RequestMetrics is an alias for ports.RequestMetricsEvent for internal storage.
type RequestMetrics = ports.RequestMetricsEvent

// AggregatedStats provides summary metrics across recent requests.
type AggregatedStats struct {
	// Request counts
	TotalRequests      int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`
	StreamingRequests  int64 `json:"streaming_requests"`

	// TTFT statistics (milliseconds)
	TTFTAvgMs int64 `json:"ttft_avg_ms"`
	TTFTP50Ms int64 `json:"ttft_p50_ms"`
	TTFTP95Ms int64 `json:"ttft_p95_ms"`
	TTFTP99Ms int64 `json:"ttft_p99_ms"`

	// Duration statistics (milliseconds)
	DurationAvgMs int64 `json:"duration_avg_ms"`
	DurationP50Ms int64 `json:"duration_p50_ms"`
	DurationP95Ms int64 `json:"duration_p95_ms"`
	DurationP99Ms int64 `json:"duration_p99_ms"`

	// Token throughput
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	AvgTokensPerSec   float64 `json:"avg_tokens_per_sec"`

	// Per-model breakdown
	ByModel map[string]*ModelAggregatedStats `json:"by_model,omitempty"`

	// Per-endpoint breakdown
	ByEndpoint map[string]*EndpointAggregatedStats `json:"by_endpoint,omitempty"`

	// Time window
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// ModelAggregatedStats provides per-model summary.
type ModelAggregatedStats struct {
	TotalRequests     int64   `json:"total_requests"`
	AvgTTFTMs         int64   `json:"avg_ttft_ms"`
	AvgDurationMs     int64   `json:"avg_duration_ms"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	AvgTokensPerSec   float64 `json:"avg_tokens_per_sec"`
}

// EndpointAggregatedStats provides per-endpoint summary.
type EndpointAggregatedStats struct {
	TotalRequests     int64   `json:"total_requests"`
	AvgTTFTMs         int64   `json:"avg_ttft_ms"`
	AvgDurationMs     int64   `json:"avg_duration_ms"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	AvgTokensPerSec   float64 `json:"avg_tokens_per_sec"`
}
