package stats

import (
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
)

// TestTranslatorCollector_RecordPassthrough verifies passthrough request tracking
func TestTranslatorCollector_RecordPassthrough(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record multiple passthrough requests
	for i := 0; i < 5; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Verify stats
	stats := collector.GetStats()
	if len(stats) != 1 {
		t.Fatalf("Expected 1 translator, got %d", len(stats))
	}

	anthropicStats, exists := stats["anthropic"]
	if !exists {
		t.Fatal("Anthropic translator stats not found")
	}

	if anthropicStats.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", anthropicStats.TotalRequests)
	}
	if anthropicStats.SuccessfulRequests != 5 {
		t.Errorf("Expected 5 successful requests, got %d", anthropicStats.SuccessfulRequests)
	}
	if anthropicStats.FailedRequests != 0 {
		t.Errorf("Expected 0 failed requests, got %d", anthropicStats.FailedRequests)
	}
	if anthropicStats.PassthroughRequests != 5 {
		t.Errorf("Expected 5 passthrough requests, got %d", anthropicStats.PassthroughRequests)
	}
	if anthropicStats.TranslationRequests != 0 {
		t.Errorf("Expected 0 translation requests, got %d", anthropicStats.TranslationRequests)
	}

	// Verify no fallback reasons recorded for passthrough
	if anthropicStats.FallbackNoCompatibleEndpoints != 0 {
		t.Errorf("Expected 0 fallback no compatible endpoints, got %d", anthropicStats.FallbackNoCompatibleEndpoints)
	}
	if anthropicStats.FallbackTranslatorDoesNotSupportPassthrough != 0 {
		t.Errorf("Expected 0 fallback translator does not support passthrough, got %d", anthropicStats.FallbackTranslatorDoesNotSupportPassthrough)
	}
	if anthropicStats.FallbackCannotPassthrough != 0 {
		t.Errorf("Expected 0 fallback cannot passthrough, got %d", anthropicStats.FallbackCannotPassthrough)
	}
}

// TestTranslatorCollector_RecordTranslationWithFallback verifies translation mode and fallback reason tracking
func TestTranslatorCollector_RecordTranslationWithFallback(t *testing.T) {
	collector := NewTranslatorCollector()

	// Test each fallback reason
	testCases := []struct {
		name           string
		fallbackReason constants.TranslatorFallbackReason
		count          int
	}{
		{
			name:           "no compatible endpoints",
			fallbackReason: constants.FallbackReasonNoCompatibleEndpoints,
			count:          3,
		},
		{
			name:           "translator does not support passthrough",
			fallbackReason: constants.FallbackReasonTranslatorDoesNotSupportPassthrough,
			count:          2,
		},
		{
			name:           "cannot passthrough",
			fallbackReason: constants.FallbackReasonCannotPassthrough,
			count:          4,
		},
	}

	totalTranslations := 0
	for _, tc := range testCases {
		for i := 0; i < tc.count; i++ {
			event := ports.TranslatorRequestEvent{
				TranslatorName: "anthropic",
				Model:          "claude-3-5-sonnet-20241022",
				Mode:           constants.TranslatorModeTranslation,
				FallbackReason: tc.fallbackReason,
				Success:        true,
				Latency:        150 * time.Millisecond,
				IsStreaming:    false,
			}
			collector.Record(event)
			totalTranslations++
		}
	}

	// Verify stats
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.TotalRequests != int64(totalTranslations) {
		t.Errorf("Expected %d total requests, got %d", totalTranslations, anthropicStats.TotalRequests)
	}
	if anthropicStats.TranslationRequests != int64(totalTranslations) {
		t.Errorf("Expected %d translation requests, got %d", totalTranslations, anthropicStats.TranslationRequests)
	}
	if anthropicStats.PassthroughRequests != 0 {
		t.Errorf("Expected 0 passthrough requests, got %d", anthropicStats.PassthroughRequests)
	}

	// Verify fallback reason counters
	if anthropicStats.FallbackNoCompatibleEndpoints != 3 {
		t.Errorf("Expected 3 fallback no compatible endpoints, got %d", anthropicStats.FallbackNoCompatibleEndpoints)
	}
	if anthropicStats.FallbackTranslatorDoesNotSupportPassthrough != 2 {
		t.Errorf("Expected 2 fallback translator does not support passthrough, got %d", anthropicStats.FallbackTranslatorDoesNotSupportPassthrough)
	}
	if anthropicStats.FallbackCannotPassthrough != 4 {
		t.Errorf("Expected 4 fallback cannot passthrough, got %d", anthropicStats.FallbackCannotPassthrough)
	}
}

// TestTranslatorCollector_ConcurrentAccess verifies thread-safety under concurrent load
func TestTranslatorCollector_ConcurrentAccess(t *testing.T) {
	collector := NewTranslatorCollector()

	const numGoroutines = 100
	const requestsPerGoroutine = 50

	var wg sync.WaitGroup

	// Launch goroutines recording passthrough events
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				event := ports.TranslatorRequestEvent{
					TranslatorName: "anthropic",
					Model:          "claude-3-5-sonnet-20241022",
					Mode:           constants.TranslatorModePassthrough,
					FallbackReason: constants.FallbackReasonNone,
					Success:        true,
					Latency:        100 * time.Millisecond,
					IsStreaming:    false,
				}
				collector.Record(event)
			}
		}()
	}

	// Launch goroutines recording translation events with fallback
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				event := ports.TranslatorRequestEvent{
					TranslatorName: "anthropic",
					Model:          "claude-3-5-sonnet-20241022",
					Mode:           constants.TranslatorModeTranslation,
					FallbackReason: constants.FallbackReasonNoCompatibleEndpoints,
					Success:        true,
					Latency:        150 * time.Millisecond,
					IsStreaming:    true,
				}
				collector.Record(event)
			}
		}()
	}

	wg.Wait()

	// Verify final counts
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	if anthropicStats.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, anthropicStats.TotalRequests)
	}

	expectedPassthrough := int64(numGoroutines/2) * int64(requestsPerGoroutine)
	if anthropicStats.PassthroughRequests != expectedPassthrough {
		t.Errorf("Expected %d passthrough requests, got %d", expectedPassthrough, anthropicStats.PassthroughRequests)
	}

	expectedTranslation := int64(numGoroutines/2) * int64(requestsPerGoroutine)
	if anthropicStats.TranslationRequests != expectedTranslation {
		t.Errorf("Expected %d translation requests, got %d", expectedTranslation, anthropicStats.TranslationRequests)
	}

	if anthropicStats.FallbackNoCompatibleEndpoints != expectedTranslation {
		t.Errorf("Expected %d fallback no compatible endpoints, got %d", expectedTranslation, anthropicStats.FallbackNoCompatibleEndpoints)
	}

	// Verify success count
	if anthropicStats.SuccessfulRequests != expectedTotal {
		t.Errorf("Expected %d successful requests, got %d", expectedTotal, anthropicStats.SuccessfulRequests)
	}
}

// TestTranslatorCollector_PassthroughRate verifies passthrough rate calculation
func TestTranslatorCollector_PassthroughRate(t *testing.T) {
	testCases := []struct {
		name                string
		passthroughCount    int
		translationCount    int
		expectedPassthrough int64
		expectedTranslation int64
	}{
		{
			name:                "all passthrough",
			passthroughCount:    10,
			translationCount:    0,
			expectedPassthrough: 10,
			expectedTranslation: 0,
		},
		{
			name:                "all translation",
			passthroughCount:    0,
			translationCount:    10,
			expectedPassthrough: 0,
			expectedTranslation: 10,
		},
		{
			name:                "50/50 split",
			passthroughCount:    5,
			translationCount:    5,
			expectedPassthrough: 5,
			expectedTranslation: 5,
		},
		{
			name:                "70/30 split",
			passthroughCount:    7,
			translationCount:    3,
			expectedPassthrough: 7,
			expectedTranslation: 3,
		},
		{
			name:                "zero requests",
			passthroughCount:    0,
			translationCount:    0,
			expectedPassthrough: 0,
			expectedTranslation: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collector := NewTranslatorCollector()

			// Record passthrough requests
			for i := 0; i < tc.passthroughCount; i++ {
				event := ports.TranslatorRequestEvent{
					TranslatorName: "anthropic",
					Model:          "claude-3-5-sonnet-20241022",
					Mode:           constants.TranslatorModePassthrough,
					FallbackReason: constants.FallbackReasonNone,
					Success:        true,
					Latency:        100 * time.Millisecond,
					IsStreaming:    false,
				}
				collector.Record(event)
			}

			// Record translation requests
			for i := 0; i < tc.translationCount; i++ {
				event := ports.TranslatorRequestEvent{
					TranslatorName: "anthropic",
					Model:          "claude-3-5-sonnet-20241022",
					Mode:           constants.TranslatorModeTranslation,
					FallbackReason: constants.FallbackReasonNoCompatibleEndpoints,
					Success:        true,
					Latency:        150 * time.Millisecond,
					IsStreaming:    false,
				}
				collector.Record(event)
			}

			// Verify stats
			stats := collector.GetStats()

			// Handle zero requests case
			if tc.passthroughCount == 0 && tc.translationCount == 0 {
				if len(stats) != 0 {
					t.Errorf("Expected 0 translators for zero requests, got %d", len(stats))
				}
				return
			}

			anthropicStats := stats["anthropic"]

			if anthropicStats.PassthroughRequests != tc.expectedPassthrough {
				t.Errorf("Expected %d passthrough requests, got %d", tc.expectedPassthrough, anthropicStats.PassthroughRequests)
			}
			if anthropicStats.TranslationRequests != tc.expectedTranslation {
				t.Errorf("Expected %d translation requests, got %d", tc.expectedTranslation, anthropicStats.TranslationRequests)
			}

			// Calculate and verify passthrough rate
			total := tc.expectedPassthrough + tc.expectedTranslation
			expectedRate := float64(0)
			if total > 0 {
				expectedRate = float64(tc.expectedPassthrough) / float64(total) * 100
			}

			actualRate := float64(0)
			if anthropicStats.TotalRequests > 0 {
				actualRate = float64(anthropicStats.PassthroughRequests) / float64(anthropicStats.TotalRequests) * 100
			}

			if actualRate != expectedRate {
				t.Errorf("Expected passthrough rate %.2f%%, got %.2f%%", expectedRate, actualRate)
			}
		})
	}
}

// TestTranslatorCollector_MultipleTranslators verifies stats are tracked separately per translator
func TestTranslatorCollector_MultipleTranslators(t *testing.T) {
	collector := NewTranslatorCollector()

	translators := []struct {
		name            string
		passthroughReqs int
		translationReqs int
	}{
		{"anthropic", 10, 5},
		{"openai", 8, 12},
		{"ollama", 15, 3},
	}

	// Record events for each translator
	for _, translator := range translators {
		// Passthrough requests
		for i := 0; i < translator.passthroughReqs; i++ {
			event := ports.TranslatorRequestEvent{
				TranslatorName: translator.name,
				Model:          "test-model",
				Mode:           constants.TranslatorModePassthrough,
				FallbackReason: constants.FallbackReasonNone,
				Success:        true,
				Latency:        100 * time.Millisecond,
				IsStreaming:    false,
			}
			collector.Record(event)
		}

		// Translation requests
		for i := 0; i < translator.translationReqs; i++ {
			event := ports.TranslatorRequestEvent{
				TranslatorName: translator.name,
				Model:          "test-model",
				Mode:           constants.TranslatorModeTranslation,
				FallbackReason: constants.FallbackReasonCannotPassthrough,
				Success:        true,
				Latency:        150 * time.Millisecond,
				IsStreaming:    false,
			}
			collector.Record(event)
		}
	}

	// Verify GetTranslatorStats returns all translators
	stats := collector.GetStats()
	if len(stats) != len(translators) {
		t.Fatalf("Expected %d translators, got %d", len(translators), len(stats))
	}

	// Verify stats are tracked separately
	for _, translator := range translators {
		translatorStats, exists := stats[translator.name]
		if !exists {
			t.Errorf("Stats not found for translator %s", translator.name)
			continue
		}

		expectedTotal := int64(translator.passthroughReqs + translator.translationReqs)
		if translatorStats.TotalRequests != expectedTotal {
			t.Errorf("Translator %s: expected %d total requests, got %d", translator.name, expectedTotal, translatorStats.TotalRequests)
		}

		if translatorStats.PassthroughRequests != int64(translator.passthroughReqs) {
			t.Errorf("Translator %s: expected %d passthrough requests, got %d", translator.name, translator.passthroughReqs, translatorStats.PassthroughRequests)
		}

		if translatorStats.TranslationRequests != int64(translator.translationReqs) {
			t.Errorf("Translator %s: expected %d translation requests, got %d", translator.name, translator.translationReqs, translatorStats.TranslationRequests)
		}

		// Verify fallback counter for translation requests
		if translatorStats.FallbackCannotPassthrough != int64(translator.translationReqs) {
			t.Errorf("Translator %s: expected %d fallback cannot passthrough, got %d", translator.name, translator.translationReqs, translatorStats.FallbackCannotPassthrough)
		}
	}
}

// TestTranslatorCollector_StreamingVsNonStreaming verifies streaming mode tracking
func TestTranslatorCollector_StreamingVsNonStreaming(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record streaming requests
	for i := 0; i < 7; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    true,
		}
		collector.Record(event)
	}

	// Record non-streaming requests
	for i := 0; i < 3; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Verify streaming breakdown
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.StreamingRequests != 7 {
		t.Errorf("Expected 7 streaming requests, got %d", anthropicStats.StreamingRequests)
	}
	if anthropicStats.NonStreamingRequests != 3 {
		t.Errorf("Expected 3 non-streaming requests, got %d", anthropicStats.NonStreamingRequests)
	}
	if anthropicStats.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", anthropicStats.TotalRequests)
	}
}

// TestTranslatorCollector_SuccessVsError verifies success and error tracking
func TestTranslatorCollector_SuccessVsError(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record successful requests
	for i := 0; i < 8; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Record failed requests
	for i := 0; i < 2; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        false,
			Latency:        50 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Verify success/failure breakdown
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.SuccessfulRequests != 8 {
		t.Errorf("Expected 8 successful requests, got %d", anthropicStats.SuccessfulRequests)
	}
	if anthropicStats.FailedRequests != 2 {
		t.Errorf("Expected 2 failed requests, got %d", anthropicStats.FailedRequests)
	}
	if anthropicStats.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", anthropicStats.TotalRequests)
	}
}

// TestTranslatorCollector_LatencyTracking verifies latency calculation
func TestTranslatorCollector_LatencyTracking(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record requests with different latencies
	latencies := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		150 * time.Millisecond,
		250 * time.Millisecond,
	}

	totalLatency := int64(0)
	for _, latency := range latencies {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        latency,
			IsStreaming:    false,
		}
		collector.Record(event)
		totalLatency += latency.Milliseconds()
	}

	// Verify latency stats
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.TotalLatency != totalLatency {
		t.Errorf("Expected total latency %dms, got %dms", totalLatency, anthropicStats.TotalLatency)
	}

	expectedAvg := totalLatency / int64(len(latencies))
	if anthropicStats.AverageLatency != expectedAvg {
		t.Errorf("Expected average latency %dms, got %dms", expectedAvg, anthropicStats.AverageLatency)
	}
}

// TestTranslatorCollector_LatencyWithFailures verifies latency calculation includes only successful requests
func TestTranslatorCollector_LatencyWithFailures(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record successful requests
	for i := 0; i < 3; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Record failed request with higher latency (should still be included in total)
	event := ports.TranslatorRequestEvent{
		TranslatorName: "anthropic",
		Model:          "claude-3-5-sonnet-20241022",
		Mode:           constants.TranslatorModePassthrough,
		FallbackReason: constants.FallbackReasonNone,
		Success:        false,
		Latency:        500 * time.Millisecond,
		IsStreaming:    false,
	}
	collector.Record(event)

	// Verify latency calculation
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	// Total latency includes all requests (3 * 100 + 1 * 500 = 800)
	expectedTotal := int64(800)
	if anthropicStats.TotalLatency != expectedTotal {
		t.Errorf("Expected total latency %dms, got %dms", expectedTotal, anthropicStats.TotalLatency)
	}

	// Average latency is calculated using successful requests only (800 / 3 = 266)
	expectedAvg := expectedTotal / 3
	if anthropicStats.AverageLatency != expectedAvg {
		t.Errorf("Expected average latency %dms, got %dms", expectedAvg, anthropicStats.AverageLatency)
	}
}

// TestTranslatorCollector_ZeroLatency verifies zero latency handling
func TestTranslatorCollector_ZeroLatency(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record request with zero latency
	event := ports.TranslatorRequestEvent{
		TranslatorName: "anthropic",
		Model:          "claude-3-5-sonnet-20241022",
		Mode:           constants.TranslatorModePassthrough,
		FallbackReason: constants.FallbackReasonNone,
		Success:        true,
		Latency:        0,
		IsStreaming:    false,
	}
	collector.Record(event)

	// Verify stats
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.TotalLatency != 0 {
		t.Errorf("Expected total latency 0ms, got %dms", anthropicStats.TotalLatency)
	}
	if anthropicStats.AverageLatency != 0 {
		t.Errorf("Expected average latency 0ms, got %dms", anthropicStats.AverageLatency)
	}
}

// TestTranslatorCollector_ZeroSuccessfulRequests verifies latency calculation with zero successful requests
func TestTranslatorCollector_ZeroSuccessfulRequests(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record only failed requests
	for i := 0; i < 3; i++ {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModePassthrough,
			FallbackReason: constants.FallbackReasonNone,
			Success:        false,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Verify average latency is 0 when no successful requests
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.SuccessfulRequests != 0 {
		t.Errorf("Expected 0 successful requests, got %d", anthropicStats.SuccessfulRequests)
	}
	if anthropicStats.AverageLatency != 0 {
		t.Errorf("Expected average latency 0ms (no successful requests), got %dms", anthropicStats.AverageLatency)
	}
}

// TestTranslatorCollector_AllFallbackReasons verifies tracking of all fallback reasons
func TestTranslatorCollector_AllFallbackReasons(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record translation with each fallback reason
	fallbackReasons := []constants.TranslatorFallbackReason{
		constants.FallbackReasonNoCompatibleEndpoints,
		constants.FallbackReasonTranslatorDoesNotSupportPassthrough,
		constants.FallbackReasonCannotPassthrough,
	}

	for _, reason := range fallbackReasons {
		event := ports.TranslatorRequestEvent{
			TranslatorName: "anthropic",
			Model:          "claude-3-5-sonnet-20241022",
			Mode:           constants.TranslatorModeTranslation,
			FallbackReason: reason,
			Success:        true,
			Latency:        100 * time.Millisecond,
			IsStreaming:    false,
		}
		collector.Record(event)
	}

	// Verify each fallback reason is counted
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.FallbackNoCompatibleEndpoints != 1 {
		t.Errorf("Expected 1 fallback no compatible endpoints, got %d", anthropicStats.FallbackNoCompatibleEndpoints)
	}
	if anthropicStats.FallbackTranslatorDoesNotSupportPassthrough != 1 {
		t.Errorf("Expected 1 fallback translator does not support passthrough, got %d", anthropicStats.FallbackTranslatorDoesNotSupportPassthrough)
	}
	if anthropicStats.FallbackCannotPassthrough != 1 {
		t.Errorf("Expected 1 fallback cannot passthrough, got %d", anthropicStats.FallbackCannotPassthrough)
	}
}

// TestTranslatorCollector_FallbackReasonOnlyForTranslation verifies fallback reasons are only recorded for translation mode
func TestTranslatorCollector_FallbackReasonOnlyForTranslation(t *testing.T) {
	collector := NewTranslatorCollector()

	// Record passthrough request with fallback reason (should be ignored)
	event := ports.TranslatorRequestEvent{
		TranslatorName: "anthropic",
		Model:          "claude-3-5-sonnet-20241022",
		Mode:           constants.TranslatorModePassthrough,
		FallbackReason: constants.FallbackReasonNoCompatibleEndpoints, // Should be ignored
		Success:        true,
		Latency:        100 * time.Millisecond,
		IsStreaming:    false,
	}
	collector.Record(event)

	// Verify no fallback reason recorded for passthrough
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	if anthropicStats.PassthroughRequests != 1 {
		t.Errorf("Expected 1 passthrough request, got %d", anthropicStats.PassthroughRequests)
	}
	if anthropicStats.FallbackNoCompatibleEndpoints != 0 {
		t.Errorf("Expected 0 fallback (passthrough mode), got %d", anthropicStats.FallbackNoCompatibleEndpoints)
	}
}

// TestTranslatorCollector_EmptyStats verifies empty state
func TestTranslatorCollector_EmptyStats(t *testing.T) {
	collector := NewTranslatorCollector()

	// Verify empty stats
	stats := collector.GetStats()
	if len(stats) != 0 {
		t.Errorf("Expected 0 translators, got %d", len(stats))
	}
}

// TestTranslatorCollector_MixedModesConcurrent verifies thread-safety with mixed modes
func TestTranslatorCollector_MixedModesConcurrent(t *testing.T) {
	collector := NewTranslatorCollector()

	const numGoroutines = 100
	const requestsPerGoroutine = 20

	var wg sync.WaitGroup

	// Launch goroutines with mixed modes, streaming states, and success states
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				mode := constants.TranslatorModePassthrough
				fallback := constants.FallbackReasonNone
				if (id+j)%2 == 0 {
					mode = constants.TranslatorModeTranslation
					fallback = constants.FallbackReasonNoCompatibleEndpoints
				}

				event := ports.TranslatorRequestEvent{
					TranslatorName: "anthropic",
					Model:          "claude-3-5-sonnet-20241022",
					Mode:           mode,
					FallbackReason: fallback,
					Success:        (id+j)%3 != 0, // 2/3 success rate
					Latency:        time.Duration(100+id+j) * time.Millisecond,
					IsStreaming:    (id+j)%2 == 0,
				}
				collector.Record(event)
			}
		}(i)
	}

	wg.Wait()

	// Verify final stats
	stats := collector.GetStats()
	anthropicStats := stats["anthropic"]

	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	if anthropicStats.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, anthropicStats.TotalRequests)
	}

	// Verify total equals sum of successful and failed
	if anthropicStats.TotalRequests != anthropicStats.SuccessfulRequests+anthropicStats.FailedRequests {
		t.Errorf("Total requests (%d) should equal successful (%d) + failed (%d)",
			anthropicStats.TotalRequests, anthropicStats.SuccessfulRequests, anthropicStats.FailedRequests)
	}

	// Verify total equals sum of passthrough and translation
	if anthropicStats.TotalRequests != anthropicStats.PassthroughRequests+anthropicStats.TranslationRequests {
		t.Errorf("Total requests (%d) should equal passthrough (%d) + translation (%d)",
			anthropicStats.TotalRequests, anthropicStats.PassthroughRequests, anthropicStats.TranslationRequests)
	}

	// Verify total equals sum of streaming and non-streaming
	if anthropicStats.TotalRequests != anthropicStats.StreamingRequests+anthropicStats.NonStreamingRequests {
		t.Errorf("Total requests (%d) should equal streaming (%d) + non-streaming (%d)",
			anthropicStats.TotalRequests, anthropicStats.StreamingRequests, anthropicStats.NonStreamingRequests)
	}
}
