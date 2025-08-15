package core

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// ExtractProviderMetrics extracts metrics from the last chunk of a response.
// This is a common operation for both proxy engines.
func ExtractProviderMetrics(
	ctx context.Context,
	extractor ports.MetricsExtractor,
	lastChunk []byte,
	endpoint *domain.Endpoint,
	stats *ports.RequestStats,
	rlog logger.StyledLogger,
	engineName string,
) {
	// Early return if we can't extract metrics
	if extractor == nil || len(lastChunk) == 0 || endpoint == nil || endpoint.Type == "" {
		if extractor == nil {
			rlog.Debug("Metrics extraction skipped - no extractor",
				"engine", engineName)
		} else {
			rlog.Debug("Metrics extraction skipped",
				"engine", engineName,
				"chunk_size", len(lastChunk),
				"has_endpoint", endpoint != nil,
				"endpoint_type", endpoint.Type)
		}
		return
	}

	// Attempt extraction
	rlog.Debug("Attempting metrics extraction",
		"engine", engineName,
		"chunk_size", len(lastChunk),
		"endpoint_type", endpoint.Type)

	stats.ProviderMetrics = extractor.ExtractFromChunk(ctx, lastChunk, endpoint.Type)

	// Log results
	if stats.ProviderMetrics != nil {
		pm := stats.ProviderMetrics
		rlog.Debug("Metrics extracted successfully",
			"engine", engineName,
			"input_tokens", pm.InputTokens,
			"output_tokens", pm.OutputTokens,
			"total_tokens", pm.TotalTokens,
			"tokens_per_sec", pm.TokensPerSecond,
			"ttft_ms", pm.TTFTMs)
	} else {
		rlog.Debug("No metrics extracted from response",
			"engine", engineName)
	}
}

// AppendProviderMetricsToLog appends provider metrics fields to log fields array.
// Used by both proxy engines for consistent logging.
func AppendProviderMetricsToLog(logFields []interface{}, pm *domain.ProviderMetrics) []interface{} {
	if pm == nil {
		return logFields
	}

	if pm.InputTokens > 0 {
		logFields = append(logFields, "input_tokens", pm.InputTokens)
	}
	if pm.OutputTokens > 0 {
		logFields = append(logFields, "output_tokens", pm.OutputTokens)
	}
	if pm.TotalTokens > 0 {
		logFields = append(logFields, "total_tokens", pm.TotalTokens)
	}
	if pm.TokensPerSecond > 0 {
		logFields = append(logFields, "tokens_per_sec", pm.TokensPerSecond)
	}
	if pm.TTFTMs > 0 {
		logFields = append(logFields, "ttft_ms", pm.TTFTMs)
	}
	if pm.PromptMs > 0 {
		logFields = append(logFields, "prompt_ms", pm.PromptMs)
	}
	if pm.GenerationMs > 0 {
		logFields = append(logFields, "generation_ms", pm.GenerationMs)
	}
	if pm.Model != "" {
		logFields = append(logFields, "provider_model", pm.Model)
	}
	if pm.FinishReason != "" {
		logFields = append(logFields, "finish_reason", pm.FinishReason)
	}

	return logFields
}
