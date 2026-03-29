package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

type TranslatorStatsResponse struct {
	Timestamp   time.Time              `json:"timestamp"`
	Translators []TranslatorStatsEntry `json:"translators"`
	Summary     TranslatorStatsSummary `json:"summary"`
}

type TranslatorStatsEntry struct {
	TranslatorName                              string `json:"translator_name"`
	SuccessRate                                 string `json:"success_rate"`
	PassthroughRate                             string `json:"passthrough_rate"`
	AverageLatency                              string `json:"average_latency"`
	TotalRequests                               int64  `json:"total_requests"`
	SuccessfulRequests                          int64  `json:"successful_requests"`
	FailedRequests                              int64  `json:"failed_requests"`
	PassthroughRequests                         int64  `json:"passthrough_requests"`
	TranslationRequests                         int64  `json:"translation_requests"`
	StreamingRequests                           int64  `json:"streaming_requests"`
	NonStreamingRequests                        int64  `json:"non_streaming_requests"`
	FallbackNoCompatibleEndpoints               int64  `json:"fallback_no_compatible_endpoints"`
	FallbackTranslatorDoesNotSupportPassthrough int64  `json:"fallback_translator_does_not_support_passthrough"`
	FallbackCannotPassthrough                   int64  `json:"fallback_cannot_passthrough"`
}

type TranslatorStatsSummary struct {
	OverallSuccessRate string `json:"overall_success_rate"`
	OverallPassthrough string `json:"overall_passthrough_rate"`
	TotalTranslators   int    `json:"total_translators"`
	ActiveTranslators  int    `json:"active_translators"`
	TotalRequests      int64  `json:"total_requests"`
	TotalPassthrough   int64  `json:"total_passthrough"`
	TotalTranslations  int64  `json:"total_translations"`
	TotalStreaming     int64  `json:"total_streaming"`
	TotalNonStreaming  int64  `json:"total_non_streaming"`
}

func (a *Application) translatorStatsHandler(w http.ResponseWriter, r *http.Request) {
	statsCollector := a.statsCollector
	if statsCollector == nil {
		http.Error(w, "Stats collector not initialized", http.StatusServiceUnavailable)
		return
	}

	translatorStats := statsCollector.GetTranslatorStats()

	translators := a.buildTranslatorStats(translatorStats)
	summary := a.buildTranslatorSummary(translatorStats)

	response := TranslatorStatsResponse{
		Timestamp:   time.Now(),
		Translators: translators,
		Summary:     summary,
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Error("Failed to encode translator stats response", "error", err)
	}
}

func (a *Application) buildTranslatorStats(translatorStats map[string]ports.TranslatorStats) []TranslatorStatsEntry {
	translators := make([]TranslatorStatsEntry, 0, len(translatorStats))

	for _, stats := range translatorStats {
		successRate := float64(0)
		if stats.TotalRequests > 0 {
			successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100
		}

		passthroughRate := float64(0)
		if stats.TotalRequests > 0 {
			passthroughRate = float64(stats.PassthroughRequests) / float64(stats.TotalRequests) * 100
		}

		entry := TranslatorStatsEntry{
			TranslatorName:                stats.TranslatorName,
			TotalRequests:                 stats.TotalRequests,
			SuccessfulRequests:            stats.SuccessfulRequests,
			FailedRequests:                stats.FailedRequests,
			SuccessRate:                   format.Percentage(successRate),
			PassthroughRequests:           stats.PassthroughRequests,
			TranslationRequests:           stats.TranslationRequests,
			PassthroughRate:               format.Percentage(passthroughRate),
			StreamingRequests:             stats.StreamingRequests,
			NonStreamingRequests:          stats.NonStreamingRequests,
			AverageLatency:                format.Latency(stats.AverageLatency),
			FallbackNoCompatibleEndpoints: stats.FallbackNoCompatibleEndpoints,
			FallbackTranslatorDoesNotSupportPassthrough: stats.FallbackTranslatorDoesNotSupportPassthrough,
			FallbackCannotPassthrough:                   stats.FallbackCannotPassthrough,
		}

		translators = append(translators, entry)
	}

	// Sort translators by total request count (most popular first)
	sort.Slice(translators, func(i, j int) bool {
		return translators[i].TotalRequests > translators[j].TotalRequests
	})

	return translators
}

func (a *Application) buildTranslatorSummary(translatorStats map[string]ports.TranslatorStats) TranslatorStatsSummary {
	var totalRequests int64
	var successfulRequests int64
	var totalPassthrough int64
	var totalTranslations int64
	var totalStreaming int64
	var totalNonStreaming int64

	// Count translators with any requests as active
	activeCount := 0

	for _, stats := range translatorStats {
		totalRequests += stats.TotalRequests
		successfulRequests += stats.SuccessfulRequests
		totalPassthrough += stats.PassthroughRequests
		totalTranslations += stats.TranslationRequests
		totalStreaming += stats.StreamingRequests
		totalNonStreaming += stats.NonStreamingRequests

		if stats.TotalRequests > 0 {
			activeCount++
		}
	}

	overallSuccessRate := float64(0)
	if totalRequests > 0 {
		overallSuccessRate = float64(successfulRequests) / float64(totalRequests) * 100
	}

	overallPassthroughRate := float64(0)
	if totalRequests > 0 {
		overallPassthroughRate = float64(totalPassthrough) / float64(totalRequests) * 100
	}

	return TranslatorStatsSummary{
		TotalTranslators:   len(translatorStats),
		ActiveTranslators:  activeCount,
		TotalRequests:      totalRequests,
		OverallSuccessRate: format.Percentage(overallSuccessRate),
		TotalPassthrough:   totalPassthrough,
		TotalTranslations:  totalTranslations,
		OverallPassthrough: format.Percentage(overallPassthroughRate),
		TotalStreaming:     totalStreaming,
		TotalNonStreaming:  totalNonStreaming,
	}
}
