package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// TestIntegration_OllamaMetricsExtraction verifies the full metrics extraction pipeline
func TestIntegration_OllamaMetricsExtraction(t *testing.T) {
	// Real Ollama response
	ollamaResponse := []byte(`{
		"model": "llama2:latest",
		"created_at": "2024-01-01T00:00:00Z",
		"done": true,
		"total_duration": 5589157167,
		"load_duration": 3013701500,
		"prompt_eval_count": 26,
		"prompt_eval_duration": 2000000000,
		"eval_count": 290,
		"eval_duration": 2575455000
	}`)

	// Create test profile for Ollama
	profile := &mockProfile{
		name: "ollama",
		config: &domain.ProfileConfig{
			Metrics: domain.MetricsConfig{
				Extraction: domain.MetricsExtractionConfig{
					Enabled: true,
					Source:  "response_body",
					Format:  "json",
					Paths: map[string]string{
						"model":              "$.model",
						"done":               "$.done",
						"input_tokens":       "$.prompt_eval_count",
						"output_tokens":      "$.eval_count",
						"prompt_duration_ns": "$.prompt_eval_duration",
						"eval_duration_ns":   "$.eval_duration",
						"total_duration_ns":  "$.total_duration",
						"load_duration_ns":   "$.load_duration",
					},
					Calculations: map[string]string{
						"tokens_per_second": "output_tokens / (eval_duration_ns / 1000000000)",
						"ttft_ms":           "prompt_duration_ns / 1000000",
					},
				},
			},
		},
	}

	factory := &mockProfileFactory{
		profiles: map[string]*mockProfile{
			"ollama": profile,
		},
	}

	// Create extractor with the new implementation
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	testLogger := logger.NewPlainStyledLogger(log)
	extractor, err := NewExtractor(factory, testLogger)
	require.NoError(t, err)

	// Validate profile (pre-compiles expressions)
	err = extractor.ValidateProfile(profile)
	require.NoError(t, err)

	// Test extraction
	ctx := context.Background()
	metrics := extractor.ExtractFromChunk(ctx, ollamaResponse, "ollama")
	require.NotNil(t, metrics)

	// Verify extracted metrics
	assert.Equal(t, "llama2:latest", metrics.Model)
	assert.Equal(t, int32(26), metrics.InputTokens)
	assert.Equal(t, int32(290), metrics.OutputTokens)
	assert.Equal(t, int32(26+290), metrics.TotalTokens)
	assert.True(t, metrics.IsComplete)

	// Verify calculated metrics
	assert.Equal(t, int32(2000), metrics.TTFTMs)           // 2000000000 ns / 1000000 = 2000 ms
	assert.InDelta(t, 112.6, metrics.TokensPerSecond, 0.1) // 290 / 2.575455 ≈ 112.6

	// Verify timing conversions
	assert.Equal(t, int32(2000), metrics.PromptMs)     // 2000000000 ns / 1000000
	assert.Equal(t, int32(2575), metrics.GenerationMs) // 2575455000 ns / 1000000
	assert.Equal(t, int32(5589), metrics.TotalMs)      // 5589157167 ns / 1000000
	assert.Equal(t, int32(3013), metrics.ModelLoadMs)  // 3013701500 ns / 1000000
}

// TestIntegration_PerformanceRegression ensures the new implementation is fast
func TestIntegration_PerformanceRegression(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ollamaResponse := []byte(`{
		"model": "llama2",
		"done": true,
		"eval_count": 290,
		"eval_duration": 2575455000
	}`)

	profile := &mockProfile{
		name: "ollama",
		config: &domain.ProfileConfig{
			Metrics: domain.MetricsConfig{
				Extraction: domain.MetricsExtractionConfig{
					Enabled: true,
					Paths: map[string]string{
						"output_tokens":    "$.eval_count",
						"eval_duration_ns": "$.eval_duration",
					},
				},
			},
		},
	}

	factory := &mockProfileFactory{
		profiles: map[string]*mockProfile{
			"ollama": profile,
		},
	}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	testLogger := logger.NewPlainStyledLogger(log)
	extractor, err := NewExtractor(factory, testLogger)
	require.NoError(t, err)

	err = extractor.ValidateProfile(profile)
	require.NoError(t, err)

	ctx := context.Background()

	// Warm up
	for i := 0; i < 100; i++ {
		_ = extractor.ExtractFromChunk(ctx, ollamaResponse, "ollama")
	}

	// Measure performance
	start := time.Now()
	iterations := 10000
	for i := 0; i < iterations; i++ {
		metrics := extractor.ExtractFromChunk(ctx, ollamaResponse, "ollama")
		if metrics == nil {
			t.Fatal("Expected metrics to be extracted")
		}
	}
	elapsed := time.Since(start)

	// Calculate per-operation time
	perOp := elapsed / time.Duration(iterations)

	// Assert performance is acceptable (should be < 50µs per operation with new implementation)
	assert.Less(t, perOp, 50*time.Microsecond,
		"Extraction took %v per operation, expected < 50µs", perOp)

	t.Logf("Performance: %v per extraction (%d iterations in %v)", perOp, iterations, elapsed)
}

// TestIntegration_LargeChunkHandling verifies we handle large responses correctly
func TestIntegration_LargeChunkHandling(t *testing.T) {
	// Create a large response that would have caused issues with old implementation
	largeResponse := []byte(`{
		"model": "llama2",
		"done": true,
		"eval_count": 1000,
		"eval_duration": 10000000000,
		"context": [` + generateLargeArray(1000) + `]
	}`)

	profile := &mockProfile{
		name: "ollama",
		config: &domain.ProfileConfig{
			Metrics: domain.MetricsConfig{
				Extraction: domain.MetricsExtractionConfig{
					Enabled: true,
					Paths: map[string]string{
						"output_tokens":    "$.eval_count",
						"eval_duration_ns": "$.eval_duration",
					},
				},
			},
		},
	}

	factory := &mockProfileFactory{
		profiles: map[string]*mockProfile{
			"ollama": profile,
		},
	}

	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	testLogger := logger.NewPlainStyledLogger(log)
	extractor, err := NewExtractor(factory, testLogger)
	require.NoError(t, err)

	ctx := context.Background()
	metrics := extractor.ExtractFromChunk(ctx, largeResponse, "ollama")
	require.NotNil(t, metrics)

	// Verify extraction still works with large payloads
	assert.Equal(t, int32(1000), metrics.OutputTokens)
}

func generateLargeArray(size int) string {
	result := ""
	for i := 0; i < size; i++ {
		if i > 0 {
			result += ","
		}
		result += "1"
	}
	return result
}
