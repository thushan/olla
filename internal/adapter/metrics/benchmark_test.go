package metrics

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

var ollamaResponse = []byte(`{
	"model": "llama2",
	"done": true,
	"context": [1, 2, 3],
	"total_duration": 5589157167,
	"load_duration": 3013701500,
	"prompt_eval_count": 26,
	"prompt_eval_duration": 2000000000,
	"eval_count": 290,
	"eval_duration": 2575455000
}`)

func createTestProfileFactory() ProfileFactory {
	return &mockProfileFactory{
		profiles: map[string]*mockProfile{
			"ollama": {
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
			},
		},
	}
}

func BenchmarkExtractor(b *testing.B) {
	// Create a discard logger for benchmarking
	slogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := logger.NewPlainStyledLogger(slogger)
	factory := createTestProfileFactory()

	extractor, err := NewExtractor(factory, log)
	if err != nil {
		b.Fatal(err)
	}

	// Validate profile
	profile, err := factory.GetProfile("ollama")
	if err != nil {
		b.Fatal(err)
	}
	if err := extractor.ValidateProfile(profile); err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = extractor.ExtractFromChunk(ctx, ollamaResponse, "ollama")
	}
}

// Benchmark just the JSON parsing
func BenchmarkJSONParsing_EncodingJSON(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var data interface{}
		_ = json.Unmarshal(ollamaResponse, &data)
	}
}

func BenchmarkJSONParsing_Gjson(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := gjson.ParseBytes(ollamaResponse)
		_ = result.Get("model").String()
		_ = result.Get("eval_duration").Int()
	}
}
