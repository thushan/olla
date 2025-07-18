package inspector

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/logger"
)

func BenchmarkChain_WithAndWithoutBodyInspector(b *testing.B) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "error", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(b, err)
	styledLog := &mockStyledLogger{underlying: log}

	// Create profile factory
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(b, err)

	requestBody := `{"model": "llama3.1:8b", "messages": [{"role": "user", "content": "Hello world"}]}`

	b.Run("WithPathInspectorOnly", func(b *testing.B) {
		// Create chain with only path inspector
		factory := NewFactory(profileFactory, styledLog)
		chain := factory.CreateChain()
		pathInspector := factory.CreatePathInspector()
		chain.AddInspector(pathInspector)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(requestBody)))
			req.Header.Set("Content-Type", "application/json")

			profile, err := chain.Inspect(ctx, req, "/v1/chat/completions")
			if err != nil {
				b.Fatal(err)
			}
			if profile.ModelName != "" {
				b.Fatal("Expected no model name with path inspector only")
			}
		}
	})

	b.Run("WithPathAndBodyInspectors", func(b *testing.B) {
		// Create chain with both inspectors
		factory := NewFactory(profileFactory, styledLog)
		chain := factory.CreateChain()
		pathInspector := factory.CreatePathInspector()
		bodyInspector := factory.CreateBodyInspector()
		chain.AddInspector(pathInspector)
		chain.AddInspector(bodyInspector)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(requestBody)))
			req.Header.Set("Content-Type", "application/json")

			profile, err := chain.Inspect(ctx, req, "/v1/chat/completions")
			if err != nil {
				b.Fatal(err)
			}
			if profile.ModelName != "llama3.1:8b" {
				b.Fatal("Expected model name to be extracted")
			}
		}
	})
}

// Benchmark the overhead of model extraction in isolation
func BenchmarkBodyInspector_Overhead(b *testing.B) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "error", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(b, err)
	styledLog := &mockStyledLogger{underlying: log}

	// Create profile factory
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(b, err)

	factory := NewFactory(profileFactory, styledLog)

	// Small request body
	smallBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hi"}]}`

	// Medium request body
	mediumBody := `{"model": "llama3.1:8b", "messages": [{"role": "user", "content": "This is a longer message with more content to parse through. It simulates a more realistic user query that might be sent to an LLM."}]}`

	// Large request body with multiple messages
	largeMessages := make([]string, 10)
	for i := range largeMessages {
		largeMessages[i] = `{"role": "user", "content": "This is message ` + string(rune(i+'0')) + ` in a multi-turn conversation"}`
	}
	largeBody := `{"model": "claude-3-opus", "messages": [` + bytes.NewBufferString(largeMessages[0]).String()
	for i := 1; i < len(largeMessages); i++ {
		largeBody += "," + largeMessages[i]
	}
	largeBody += `]}`

	benchmarks := []struct {
		name string
		body string
	}{
		{"SmallBody", smallBody},
		{"MediumBody", mediumBody},
		{"LargeBody", largeBody},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			chain := factory.CreateChain()
			pathInspector := factory.CreatePathInspector()
			bodyInspector := factory.CreateBodyInspector()
			chain.AddInspector(pathInspector)
			chain.AddInspector(bodyInspector)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(bm.body)))
				req.Header.Set("Content-Type", "application/json")

				_, err := chain.Inspect(ctx, req, "/v1/chat/completions")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
