package stats

import (
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
)

// TranslatorCollector tracks translator-specific statistics
type TranslatorCollector struct {
	translators *xsync.Map[string, *translatorData]
}

// translatorData holds all metrics for a single translator
type translatorData struct {
	// Total request counts
	totalRequests      *xsync.Counter
	successfulRequests *xsync.Counter
	failedRequests     *xsync.Counter

	// Mode breakdown
	passthroughRequests *xsync.Counter
	translationRequests *xsync.Counter

	// Streaming breakdown
	streamingRequests    *xsync.Counter
	nonStreamingRequests *xsync.Counter

	// Fallback reasons
	fallbackNoCompatibleEndpoints               *xsync.Counter
	fallbackTranslatorDoesNotSupportPassthrough *xsync.Counter
	fallbackCannotPassthrough                   *xsync.Counter

	// Performance metrics
	totalLatency *xsync.Counter
	name         string // translator name
}

// NewTranslatorCollector creates a new TranslatorCollector
func NewTranslatorCollector() *TranslatorCollector {
	return &TranslatorCollector{
		translators: xsync.NewMap[string, *translatorData](),
	}
}

// Record processes a translator request event and updates metrics
func (tc *TranslatorCollector) Record(event ports.TranslatorRequestEvent) {
	data := tc.getOrInit(event.TranslatorName)

	// Update total counts
	data.totalRequests.Inc()
	if event.Success {
		data.totalLatency.Add(event.Latency.Milliseconds())
		data.successfulRequests.Inc()
	} else {
		data.failedRequests.Inc()
	}

	// Update mode
	switch event.Mode {
	case constants.TranslatorModePassthrough:
		data.passthroughRequests.Inc()
	case constants.TranslatorModeTranslation:
		data.translationRequests.Inc()
	}

	// Update streaming
	if event.IsStreaming {
		data.streamingRequests.Inc()
	} else {
		data.nonStreamingRequests.Inc()
	}

	// Update fallback reason (only when mode is translation)
	if event.Mode == constants.TranslatorModeTranslation {
		switch event.FallbackReason {
		case constants.FallbackReasonNone:
			// No fallback occurred, nothing to track
		case constants.FallbackReasonNoCompatibleEndpoints:
			data.fallbackNoCompatibleEndpoints.Inc()
		case constants.FallbackReasonTranslatorDoesNotSupportPassthrough:
			data.fallbackTranslatorDoesNotSupportPassthrough.Inc()
		case constants.FallbackReasonCannotPassthrough:
			data.fallbackCannotPassthrough.Inc()
		}
	}
}

// GetStats returns aggregated statistics for all translators
func (tc *TranslatorCollector) GetStats() map[string]ports.TranslatorStats {
	result := make(map[string]ports.TranslatorStats)

	tc.translators.Range(func(name string, data *translatorData) bool {
		total := data.totalRequests.Value()
		successful := data.successfulRequests.Value()
		totalLatency := data.totalLatency.Value()

		var avgLatency int64
		if successful > 0 {
			avgLatency = totalLatency / successful
		}

		result[name] = ports.TranslatorStats{
			TranslatorName:                              name,
			TotalRequests:                               total,
			SuccessfulRequests:                          successful,
			FailedRequests:                              data.failedRequests.Value(),
			PassthroughRequests:                         data.passthroughRequests.Value(),
			TranslationRequests:                         data.translationRequests.Value(),
			StreamingRequests:                           data.streamingRequests.Value(),
			NonStreamingRequests:                        data.nonStreamingRequests.Value(),
			FallbackNoCompatibleEndpoints:               data.fallbackNoCompatibleEndpoints.Value(),
			FallbackTranslatorDoesNotSupportPassthrough: data.fallbackTranslatorDoesNotSupportPassthrough.Value(),
			FallbackCannotPassthrough:                   data.fallbackCannotPassthrough.Value(),
			AverageLatency:                              avgLatency,
			TotalLatency:                                totalLatency,
		}
		return true
	})

	return result
}

// getOrInit returns existing translator data or creates new one
func (tc *TranslatorCollector) getOrInit(translatorName string) *translatorData {
	data, _ := tc.translators.LoadOrCompute(translatorName, func() (*translatorData, bool) {
		return &translatorData{
			name:                          translatorName,
			totalRequests:                 xsync.NewCounter(),
			successfulRequests:            xsync.NewCounter(),
			failedRequests:                xsync.NewCounter(),
			passthroughRequests:           xsync.NewCounter(),
			translationRequests:           xsync.NewCounter(),
			streamingRequests:             xsync.NewCounter(),
			nonStreamingRequests:          xsync.NewCounter(),
			fallbackNoCompatibleEndpoints: xsync.NewCounter(),
			fallbackTranslatorDoesNotSupportPassthrough: xsync.NewCounter(),
			fallbackCannotPassthrough:                   xsync.NewCounter(),
			totalLatency:                                xsync.NewCounter(),
		}, false
	})
	return data
}
