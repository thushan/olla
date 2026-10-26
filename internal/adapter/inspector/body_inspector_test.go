package inspector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestBodyInspector_Inspect(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}

	tests := []struct {
		name           string
		body           string
		contentType    string
		expectedModel  string
		skipInspection bool
	}{
		{
			name:          "OpenAI format",
			body:          `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`,
			contentType:   "application/json",
			expectedModel: "gpt-4",
		},
		{
			name:          "Ollama format with tag",
			body:          `{"model": "llama3.1:8b", "prompt": "Hello"}`,
			contentType:   "application/json",
			expectedModel: "llama3.1:8b",
		},
		{
			name:          "LM Studio format",
			body:          `{"model": "meta-llama-3.1-8b-instruct", "messages": []}`,
			contentType:   "application/json",
			expectedModel: "meta-llama-3.1-8b-instruct",
		},
		{
			name:          "Model with latest tag",
			body:          `{"model": "codellama:latest"}`,
			contentType:   "application/json",
			expectedModel: "codellama:latest",
		},
		{
			name:          "Model with uppercase",
			body:          `{"model": "GPT-4-Turbo"}`,
			contentType:   "application/json",
			expectedModel: "gpt-4-turbo",
		},
		{
			name:          "Model with whitespace",
			body:          `{"model": "  llama3.1:8b  "}`,
			contentType:   "application/json",
			expectedModel: "llama3.1:8b",
		},
		{
			name:          "Empty model field",
			body:          `{"model": "", "messages": []}`,
			contentType:   "application/json",
			expectedModel: "",
		},
		{
			name:          "No model field",
			body:          `{"messages": [{"role": "user", "content": "Hello"}]}`,
			contentType:   "application/json",
			expectedModel: "",
		},
		{
			// the body decides, not the header: fetch() and curl -d send JSON as text/plain
			// and form-urlencoded
			name:          "JSON body with text/plain content type",
			body:          `{"model": "gpt-4"}`,
			contentType:   "text/plain",
			expectedModel: "gpt-4",
		},
		{
			name:          "Invalid JSON",
			body:          `{invalid json}`,
			contentType:   "application/json",
			expectedModel: "",
		},
		{
			name:          "Case insensitive model key",
			body:          `{"Model": "gpt-4"}`,
			contentType:   "application/json",
			expectedModel: "gpt-4",
		},
		{
			name:          "Nested object",
			body:          `{"request": {"model": "gpt-4"}, "model": "llama3"}`,
			contentType:   "application/json",
			expectedModel: "llama3", // Should get top-level model
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector, err := NewBodyInspector(styledLog)
			if err != nil {
				t.Fatalf("Failed to create body inspector: %v", err)
			}

			// Create request with body
			req := &http.Request{
				Body:   io.NopCloser(strings.NewReader(tt.body)),
				Header: make(http.Header),
			}
			req.Header.Set("Content-Type", tt.contentType)
			req.ContentLength = int64(len(tt.body))

			// Create profile
			profile := domain.NewRequestProfile("/v1/chat/completions")

			// Inspect
			err = inspector.Inspect(ctx, req, profile)
			require.NoError(t, err)

			// Verify model extraction
			assert.Equal(t, tt.expectedModel, profile.ModelName)

			// Verify body is still readable
			if !tt.skipInspection && tt.body != "" {
				bodyBytes, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.Equal(t, tt.body, string(bodyBytes))
			}
		})
	}
}

func TestBodyInspector_LargeBody(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	if err != nil {
		t.Fatalf("Failed to create body inspector: %v", err)
	}

	// Build a large JSON body that mimics a vision request: the model field is at the very
	// start (well within modelScanSize), while the bulk of the body is a large base64 image.
	imagePayload := strings.Repeat("A", MaxBodySize+1000)
	largeBody := `{"model":"vision-model","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + imagePayload + `"}}]}]}`

	req := &http.Request{
		Body:          io.NopCloser(strings.NewReader(largeBody)),
		Header:        make(http.Header),
		ContentLength: int64(len(largeBody)),
	}
	req.Header.Set("Content-Type", "application/json")

	profile := domain.NewRequestProfile("/v1/chat/completions")

	// Large body: model name must still be extracted from the prefix scan so routing works.
	// Capability detection is intentionally skipped (no full parse) for large requests.
	err = inspector.Inspect(ctx, req, profile)
	assert.NoError(t, err)
	assert.Equal(t, "vision-model", profile.ModelName, "model name must be extracted even from large vision requests")
	assert.Nil(t, profile.ModelCapabilities, "capability detection must be skipped for large requests")

	// The full body must be restored intact for downstream proxy handlers.
	restored, readErr := io.ReadAll(req.Body)
	assert.NoError(t, readErr)
	assert.Equal(t, largeBody, string(restored), "full body must be restored after prefix scan")
}

// TestBodyInspector_LargeBodyModelAfterMessages verifies that model extraction works
// correctly when the "model" field appears after a large "messages" array (containing
// base64 image payloads) in a request body exceeding MaxBodySize.
// This is the field-order case that the fixed 64 KB prefix approach would silently miss.
func TestBodyInspector_LargeBodyModelAfterMessages(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	require.NoError(t, err)

	// Build a large JSON body where "messages" (with a base64 image payload) appears
	// before "model". The model field is beyond the first 64 KB of the body, which
	// would have caused extraction to fail with the old fixed-prefix approach.
	imagePayload := strings.Repeat("B", MaxBodySize+1000)
	largeBody := `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` +
		imagePayload + `"}}]}],"model":"vision-model-late"}`

	req := &http.Request{
		Body:          io.NopCloser(strings.NewReader(largeBody)),
		Header:        make(http.Header),
		ContentLength: int64(len(largeBody)),
	}
	req.Header.Set("Content-Type", "application/json")

	profile := domain.NewRequestProfile("/v1/chat/completions")

	err = inspector.Inspect(ctx, req, profile)
	assert.NoError(t, err)
	assert.Equal(t, "vision-model-late", profile.ModelName,
		"model name must be extracted even when 'model' field appears after large 'messages'")

	// The full body must be restored intact for downstream proxy handlers.
	restored, readErr := io.ReadAll(req.Body)
	assert.NoError(t, readErr)
	assert.Equal(t, largeBody, string(restored), "full body must be restored after incremental scan")
}

func TestBodyInspector_NoBody(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	if err != nil {
		t.Fatalf("Failed to create body inspector: %v", err)
	}

	tests := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "nil body",
			req: &http.Request{
				Body:   nil,
				Header: make(http.Header),
			},
		},
		{
			name: "zero content length",
			req: &http.Request{
				Body:          io.NopCloser(strings.NewReader("")),
				Header:        make(http.Header),
				ContentLength: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Header.Set("Content-Type", "application/json")
			profile := domain.NewRequestProfile("/v1/chat/completions")

			err = inspector.Inspect(ctx, tt.req, profile)
			assert.NoError(t, err)
			assert.Empty(t, profile.ModelName)
		})
	}
}

func TestBodyInspector_BufferPoolReuse(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	if err != nil {
		t.Fatalf("Failed to create body inspector: %v", err)
	}

	// Run multiple inspections to test buffer pool reuse
	for range 10 {
		body := `{"model": "test-model"}`
		req := &http.Request{
			Body:          io.NopCloser(strings.NewReader(body)),
			Header:        make(http.Header),
			ContentLength: int64(len(body)),
		}
		req.Header.Set("Content-Type", "application/json")

		profile := domain.NewRequestProfile("/v1/chat/completions")

		err = inspector.Inspect(ctx, req, profile)
		assert.NoError(t, err)
		assert.Equal(t, "test-model", profile.ModelName)
	}
}

func TestBodyInspector_CapabilityDetection(t *testing.T) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "debug", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}

	tests := []struct {
		name                   string
		body                   string
		expectedVision         bool
		expectedFunctions      bool
		expectedStreaming      bool
		expectedEmbeddings     bool
		expectedCodeGeneration bool
		expectedChatCompletion bool
		expectedTextGeneration bool
	}{
		{
			name:                   "Simple chat request",
			body:                   `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`,
			expectedVision:         false,
			expectedFunctions:      false,
			expectedStreaming:      true, // Default
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Vision request with image_url",
			body: `{
				"model": "gpt-4-vision",
				"messages": [{
					"role": "user",
					"content": [{
						"type": "text",
						"text": "What's in this image?"
					}, {
						"type": "image_url",
						"image_url": {"url": "https://example.com/image.jpg"}
					}]
				}]
			}`,
			expectedVision:         true,
			expectedFunctions:      false,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Vision request with base64 image",
			body: `{
				"model": "llava",
				"messages": [{
					"role": "user",
					"content": [{
						"type": "text",
						"text": "data:image/jpeg;base64,/9j/4AAQ..."
					}]
				}]
			}`,
			expectedVision:         true,
			expectedFunctions:      false,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Function calling request with tools",
			body: `{
				"model": "gpt-4-turbo",
				"messages": [{"role": "user", "content": "Get the weather"}],
				"tools": [{
					"type": "function",
					"function": {
						"name": "get_weather",
						"description": "Get weather for a location"
					}
				}]
			}`,
			expectedVision:         false,
			expectedFunctions:      true,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Function calling with functions array",
			body: `{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Calculate something"}],
				"functions": [{
					"name": "calculate",
					"description": "Perform calculations"
				}]
			}`,
			expectedVision:         false,
			expectedFunctions:      true,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Function calling with tool_choice",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Use a tool"}],
				"tool_choice": "auto"
			}`,
			expectedVision:         false,
			expectedFunctions:      true,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Embeddings request",
			body: `{
				"model": "text-embedding-ada-002",
				"input": "This is a test sentence for embeddings"
			}`,
			expectedVision:         false,
			expectedFunctions:      false,
			expectedStreaming:      true,
			expectedEmbeddings:     true,
			expectedCodeGeneration: false,
			expectedChatCompletion: false,
			expectedTextGeneration: false,
		},
		{
			name: "Code generation with system prompt",
			body: `{
				"model": "codellama",
				"messages": [{
					"role": "system",
					"content": "You are a code assistant. Help me debug this function."
				}, {
					"role": "user",
					"content": "Fix this code"
				}]
			}`,
			expectedVision:         false,
			expectedFunctions:      false,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: true,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Code generation with language field",
			body: `{
				"model": "deepseek-coder",
				"messages": [{"role": "user", "content": "Write a function"}],
				"language": "python"
			}`,
			expectedVision:         false,
			expectedFunctions:      false,
			expectedStreaming:      true,
			expectedEmbeddings:     false,
			expectedCodeGeneration: true,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Non-streaming request",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Hello"}],
				"stream": false
			}`,
			expectedVision:         false,
			expectedFunctions:      false,
			expectedStreaming:      false,
			expectedEmbeddings:     false,
			expectedCodeGeneration: false,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
		{
			name: "Combined capabilities",
			body: `{
				"model": "gpt-4-turbo",
				"messages": [{
					"role": "system",
					"content": "You are a programming assistant"
				}, {
					"role": "user",
					"content": [{
						"type": "text",
						"text": "Debug this code in the image"
					}, {
						"type": "image_url",
						"image_url": {"url": "https://example.com/code.png"}
					}]
				}],
				"tools": [{"type": "function", "function": {"name": "run_code"}}],
				"stream": false
			}`,
			expectedVision:         true,
			expectedFunctions:      true,
			expectedStreaming:      false,
			expectedEmbeddings:     false,
			expectedCodeGeneration: true,
			expectedChatCompletion: true,
			expectedTextGeneration: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector, err := NewBodyInspector(styledLog)
			require.NoError(t, err)

			req := &http.Request{
				Body:          io.NopCloser(strings.NewReader(tt.body)),
				Header:        make(http.Header),
				ContentLength: int64(len(tt.body)),
			}
			req.Header.Set("Content-Type", "application/json")

			profile := domain.NewRequestProfile("/v1/chat/completions")

			err = inspector.Inspect(ctx, req, profile)
			require.NoError(t, err)

			// Check if capabilities should be set
			hasSpecialCapabilities := tt.expectedVision || tt.expectedFunctions ||
				tt.expectedEmbeddings || tt.expectedCodeGeneration

			if hasSpecialCapabilities {
				// If special capabilities are expected, ModelCapabilities should be set
				require.NotNil(t, profile.ModelCapabilities, "ModelCapabilities should be set when special capabilities are detected")

				assert.Equal(t, tt.expectedVision, profile.ModelCapabilities.VisionUnderstanding, "Vision capability mismatch")
				assert.Equal(t, tt.expectedFunctions, profile.ModelCapabilities.FunctionCalling, "Function calling capability mismatch")
				assert.Equal(t, tt.expectedStreaming, profile.ModelCapabilities.StreamingSupport, "Streaming capability mismatch")
				assert.Equal(t, tt.expectedEmbeddings, profile.ModelCapabilities.Embeddings, "Embeddings capability mismatch")
				assert.Equal(t, tt.expectedCodeGeneration, profile.ModelCapabilities.CodeGeneration, "Code generation capability mismatch")
				assert.Equal(t, tt.expectedChatCompletion, profile.ModelCapabilities.ChatCompletion, "Chat completion capability mismatch")
				assert.Equal(t, tt.expectedTextGeneration, profile.ModelCapabilities.TextGeneration, "Text generation capability mismatch")
			} else {
				// For simple requests without special capabilities, ModelCapabilities may be nil
				if profile.ModelCapabilities != nil {
					// If it is set, verify the values are as expected
					assert.Equal(t, tt.expectedVision, profile.ModelCapabilities.VisionUnderstanding, "Vision capability mismatch")
					assert.Equal(t, tt.expectedFunctions, profile.ModelCapabilities.FunctionCalling, "Function calling capability mismatch")
					assert.Equal(t, tt.expectedStreaming, profile.ModelCapabilities.StreamingSupport, "Streaming capability mismatch")
					assert.Equal(t, tt.expectedEmbeddings, profile.ModelCapabilities.Embeddings, "Embeddings capability mismatch")
					assert.Equal(t, tt.expectedCodeGeneration, profile.ModelCapabilities.CodeGeneration, "Code generation capability mismatch")
				}
			}
		})
	}
}

func BenchmarkBodyInspector_Inspect(b *testing.B) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "error", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(b, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	if err != nil {
		b.Fatalf("Failed to create body inspector: %v", err)
	}

	body := `{"model": "llama3.1:8b", "messages": [{"role": "user", "content": "Hello world"}]}`

	b.ResetTimer()
	for range b.N {
		req := &http.Request{
			Body:          io.NopCloser(bytes.NewReader([]byte(body))),
			Header:        make(http.Header),
			ContentLength: int64(len(body)),
		}
		req.Header.Set("Content-Type", "application/json")

		profile := domain.NewRequestProfile("/v1/chat/completions")

		inspectErr := inspector.Inspect(ctx, req, profile)
		if inspectErr != nil {
			b.Fatal(inspectErr)
		}
	}
}

func BenchmarkBodyInspector_LargeBody(b *testing.B) {
	ctx := context.Background()
	logCfg := &logger.Config{Level: "error", PrettyLogs: false}
	log, _, err := logger.New(logCfg)
	require.NoError(b, err)
	styledLog := &mockStyledLogger{underlying: log}
	inspector, err := NewBodyInspector(styledLog)
	if err != nil {
		b.Fatalf("Failed to create body inspector: %v", err)
	}

	// Create a moderately large but still inspectable body
	messages := make([]string, 50)
	for i := range messages {
		messages[i] = `{"role": "user", "content": "This is a test message"}`
	}
	body := `{"model": "gpt-4", "messages": [` + strings.Join(messages, ",") + `]}`

	b.ResetTimer()
	for range b.N {
		req := &http.Request{
			Body:          io.NopCloser(bytes.NewReader([]byte(body))),
			Header:        make(http.Header),
			ContentLength: int64(len(body)),
		}
		req.Header.Set("Content-Type", "application/json")

		profile := domain.NewRequestProfile("/v1/chat/completions")

		inspectErr := inspector.Inspect(ctx, req, profile)
		if inspectErr != nil {
			b.Fatal(inspectErr)
		}
	}
}

// mockStyledLogger is a simple mock implementation of StyledLogger for testing
type mockStyledLogger struct {
	underlying *slog.Logger
}

func (m *mockStyledLogger) Debug(msg string, args ...any)                                {}
func (m *mockStyledLogger) Info(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Warn(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Error(msg string, args ...any)                                {}
func (m *mockStyledLogger) ResetLine()                                                   {}
func (m *mockStyledLogger) InfoWithStatus(msg string, status string, args ...any)        {}
func (m *mockStyledLogger) InfoWithCount(msg string, count int, args ...any)             {}
func (m *mockStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {}
func (m *mockStyledLogger) InfoWithNumbers(msg string, numbers ...int64)                 {}
func (m *mockStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any)   {}
func (m *mockStyledLogger) InfoHealthy(msg string, endpoint string, args ...any)         {}
func (m *mockStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
}
func (m *mockStyledLogger) GetUnderlying() *slog.Logger                                         { return m.underlying }
func (m *mockStyledLogger) WithRequestID(requestID string) logger.StyledLogger                  { return m }
func (m *mockStyledLogger) WithPrefix(prefix string) logger.StyledLogger                        { return m }
func (m *mockStyledLogger) WithAttrs(attrs ...slog.Attr) logger.StyledLogger                    { return m }
func (m *mockStyledLogger) With(args ...any) logger.StyledLogger                                { return m }
func (m *mockStyledLogger) InfoWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) WarnWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) ErrorWithContext(msg string, endpoint string, ctx logger.LogContext) {}
func (m *mockStyledLogger) InfoConfigChange(oldName, newName string)                            {}

// The path inspector runs first and may already have set what the path implies;
// a body that detects its own capabilities must not erase them. For rerank and
// classification the path is the stronger signal: their bodies carry `input`,
// which the body detector reads as embeddings, and no model does both.
func TestBodyInspector_PathCapabilitiesVersusBody(t *testing.T) {
	t.Parallel()

	log, _, err := logger.New(&logger.Config{Level: "debug", PrettyLogs: false})
	require.NoError(t, err)
	inspector, err := NewBodyInspector(&mockStyledLogger{underlying: log})
	require.NoError(t, err)

	tests := []struct {
		name           string
		prior          *domain.ModelCapabilities
		body           string
		wantRerank     bool
		wantClassify   bool
		wantEmbeddings bool
		wantFunctions  bool
	}{
		{"rerank path beats body input", &domain.ModelCapabilities{Rerank: true}, `{"input":"hello"}`, true, false, false, false},
		{"classification path beats body input", &domain.ModelCapabilities{Classification: true}, `{"input":"hello"}`, false, true, false, false},
		{"rerank path still merges function calling", &domain.ModelCapabilities{Rerank: true}, `{"input":"hello","tools":[{"type":"function"}]}`, true, false, false, true},
		{"embed path keeps embeddings", &domain.ModelCapabilities{Embeddings: true}, `{"input":"hello"}`, false, false, true, false},
		{"embed path merges function calling", &domain.ModelCapabilities{Embeddings: true}, `{"tools":[{"type":"function"}]}`, false, false, true, true},
		{"no path capability uses body embeddings", nil, `{"input":"hello"}`, false, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reqProfile := domain.NewRequestProfile("/x")
			reqProfile.ModelCapabilities = tt.prior

			req, err := http.NewRequest(http.MethodPost, "/x", strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			require.NoError(t, inspector.Inspect(context.Background(), req, reqProfile))

			require.NotNil(t, reqProfile.ModelCapabilities)
			assert.Equal(t, tt.wantRerank, reqProfile.ModelCapabilities.Rerank, "rerank")
			assert.Equal(t, tt.wantClassify, reqProfile.ModelCapabilities.Classification, "classification")
			assert.Equal(t, tt.wantEmbeddings, reqProfile.ModelCapabilities.Embeddings, "embeddings")
			assert.Equal(t, tt.wantFunctions, reqProfile.ModelCapabilities.FunctionCalling, "function calling")
		})
	}
}

func TestBodyInspector_BodyDecidesNotContentType(t *testing.T) {
	t.Parallel()
	log, _, err := logger.New(&logger.Config{Level: "debug", PrettyLogs: false})
	require.NoError(t, err)
	styledLog := &mockStyledLogger{underlying: log}

	const embedBody = `{"model":"nomic-embed-text","input":"hi"}`
	bigPad := strings.Repeat("A", MaxBodySize+1000)
	lateModelBig := `{"messages":[{"content":"` + bigPad + `"}],"model":"late-model"}`
	earlyModelBig := `{"model":"early-model","messages":[{"content":"` + bigPad + `"}]}`

	tests := []struct {
		name         string
		body         string
		contentType  string
		encoding     string
		unknownLen   bool
		wantModel    string
		wantEmbed    bool
		wantUntouch  bool // body must not be interpreted at all
		skipCapCheck bool
	}{
		{name: "form-urlencoded (curl default)", body: embedBody, contentType: "application/x-www-form-urlencoded", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "text/plain", body: embedBody, contentType: "text/plain", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "text/plain charset", body: embedBody, contentType: "text/plain;charset=UTF-8", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "vnd.api+json", body: embedBody, contentType: "application/vnd.api+json", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "ld+json", body: embedBody, contentType: "application/ld+json", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "x-ndjson", body: embedBody, contentType: "application/x-ndjson", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "octet-stream", body: embedBody, contentType: "application/octet-stream", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "missing header", body: embedBody, contentType: "", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "uppercase JSON", body: embedBody, contentType: "APPLICATION/JSON", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "json with charset", body: embedBody, contentType: "application/json; charset=utf-8", wantModel: "nomic-embed-text", wantEmbed: true},
		{name: "leading whitespace", body: "  \r\n\t" + embedBody, contentType: "text/plain", wantModel: "nomic-embed-text", wantEmbed: true},

		{name: "multipart with model field", body: "--b\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nwhisper\r\n--b--\r\n", contentType: "multipart/form-data; boundary=b", wantUntouch: true},
		{name: "multipart even if body looks like JSON", body: embedBody, contentType: "multipart/form-data; boundary=b", wantUntouch: true},
		{name: "gzip body", body: "\x1f\x8b\x08\x00binary", contentType: "application/json", encoding: "gzip", wantUntouch: true},
		{name: "encoded header on JSON-looking body", body: embedBody, contentType: "application/json", encoding: "gzip", wantUntouch: true},
		{name: "real form body", body: "model=nomic-embed-text&input=hi", contentType: "application/x-www-form-urlencoded", wantUntouch: true},
		{name: "binary body", body: "\x00\x01\x02model\xff", contentType: "application/octet-stream", wantUntouch: true},
		{name: "array body", body: `[{"model":"x"}]`, contentType: "application/json", wantUntouch: true},
		{name: "whitespace only", body: "   \n  ", contentType: "text/plain", wantUntouch: true},

		{name: "large body, early model, text/plain", body: earlyModelBig, contentType: "text/plain", wantModel: "early-model", skipCapCheck: true},
		{name: "large body, late model, form-urlencoded", body: lateModelBig, contentType: "application/x-www-form-urlencoded", wantModel: "late-model", skipCapCheck: true},
		{name: "large chunked body, late model", body: lateModelBig, contentType: "application/json", unknownLen: true, wantModel: "late-model", skipCapCheck: true},
		{name: "large chunked body, late model, no content type", body: lateModelBig, contentType: "", unknownLen: true, wantModel: "late-model", skipCapCheck: true},
		{name: "small chunked body keeps capability detection", body: embedBody, contentType: "text/plain", unknownLen: true, wantModel: "nomic-embed-text", wantEmbed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bi, err := NewBodyInspector(styledLog)
			require.NoError(t, err)

			req := &http.Request{
				Body:          io.NopCloser(strings.NewReader(tt.body)),
				Header:        make(http.Header),
				ContentLength: int64(len(tt.body)),
			}
			if tt.unknownLen {
				req.ContentLength = -1
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
			}
			profile := domain.NewRequestProfile("/v1/embeddings")

			require.NoError(t, bi.Inspect(context.Background(), req, profile))

			if tt.wantUntouch {
				assert.Empty(t, profile.ModelName)
				assert.Nil(t, profile.ModelCapabilities)
			} else {
				assert.Equal(t, tt.wantModel, profile.ModelName)
				if !tt.skipCapCheck {
					require.NotNil(t, profile.ModelCapabilities)
					assert.Equal(t, tt.wantEmbed, profile.ModelCapabilities.Embeddings)
				}
			}

			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.True(t, tt.body == string(got), "body must be restored byte-for-byte (len want %d got %d)", len(tt.body), len(got))
		})
	}
}

func TestBodyInspector_CloseReachesOriginalBody(t *testing.T) {
	t.Parallel()
	log, _, err := logger.New(&logger.Config{Level: "debug", PrettyLogs: false})
	require.NoError(t, err)
	bi, err := NewBodyInspector(&mockStyledLogger{underlying: log})
	require.NoError(t, err)

	for _, body := range []string{`{"model":"m"}`, "not json", ""} {
		closer := &closeTracker{Reader: strings.NewReader(body)}
		req := &http.Request{Body: closer, Header: make(http.Header), ContentLength: int64(len(body)) + 1}
		require.NoError(t, bi.Inspect(context.Background(), req, domain.NewRequestProfile("/x")))
		require.NoError(t, req.Body.Close())
		assert.True(t, closer.closed, "Close must propagate for body %q", body)
	}
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }

func TestBodyInspector_PeekWindowBoundary(t *testing.T) {
	t.Parallel()
	log, _, err := logger.New(&logger.Config{Level: "debug", PrettyLogs: false})
	require.NoError(t, err)

	for _, spaces := range []int{peekWindow - 1, peekWindow, peekWindow + 1, 4 * peekWindow} {
		t.Run(fmt.Sprintf("%d leading spaces", spaces), func(t *testing.T) {
			t.Parallel()
			bi, err := NewBodyInspector(&mockStyledLogger{underlying: log})
			require.NoError(t, err)

			body := strings.Repeat(" ", spaces) + `{"model":"x","input":"hi"}`
			req := &http.Request{Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
			req.Header.Set("Content-Type", "application/json")
			profile := domain.NewRequestProfile("/v1/embeddings")

			require.NoError(t, bi.Inspect(context.Background(), req, profile))
			assert.Equal(t, "x", profile.ModelName)
			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.True(t, body == string(got), "body must be restored byte-for-byte")
		})
	}
}

// A chunked body of exactly MaxBodySize is complete and keeps full inspection (capabilities);
// only one byte more is treated as oversized and falls back to the model-only scan.
func TestBodyInspector_ChunkedLimitBoundary(t *testing.T) {
	t.Parallel()
	log, _, err := logger.New(&logger.Config{Level: "debug", PrettyLogs: false})
	require.NoError(t, err)

	const prefix = `{"model":"x","input":"hi","pad":"`
	const suffix = `"}`
	build := func(total int) string {
		return prefix + strings.Repeat("a", total-len(prefix)-len(suffix)) + suffix
	}

	tests := []struct {
		name     string
		size     int
		wantCaps bool
	}{
		{"limit-1", MaxBodySize - 1, true},
		{"limit", MaxBodySize, true},
		{"limit+1", MaxBodySize + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bi, err := NewBodyInspector(&mockStyledLogger{underlying: log})
			require.NoError(t, err)

			body := build(tt.size)
			require.Len(t, body, tt.size)
			req := &http.Request{Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), ContentLength: -1}
			profile := domain.NewRequestProfile("/v1/embeddings")

			require.NoError(t, bi.Inspect(context.Background(), req, profile))
			assert.Equal(t, "x", profile.ModelName)
			if tt.wantCaps {
				require.NotNil(t, profile.ModelCapabilities)
				assert.True(t, profile.ModelCapabilities.Embeddings)
			} else {
				assert.Nil(t, profile.ModelCapabilities)
			}
			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.True(t, body == string(got), "body must be restored byte-for-byte")
		})
	}
}
