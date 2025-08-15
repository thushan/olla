package ports

import (
	"context"
	"net/http"

	"github.com/thushan/olla/internal/core/domain"
)

// MetricsExtractor extracts provider-specific metrics from responses
type MetricsExtractor interface {
	// ValidateProfile validates metrics configuration at startup
	ValidateProfile(profile domain.InferenceProfile) error

	// ExtractMetrics attempts to extract metrics from response body and headers
	// Returns nil if extraction fails or is not configured - best effort approach
	ExtractMetrics(ctx context.Context, responseBody []byte, headers http.Header, providerName string) *domain.ProviderMetrics

	// ExtractFromChunk extracts metrics from a streaming chunk (final chunk for streaming responses)
	ExtractFromChunk(ctx context.Context, chunk []byte, providerName string) *domain.ProviderMetrics
}
