package inspector

import (
	"bytes"
	"context"
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
			name:           "Non-JSON content type",
			body:           `{"model": "gpt-4"}`,
			contentType:    "text/plain",
			expectedModel:  "",
			skipInspection: true,
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

	// Create a large body that exceeds max size
	largeBody := strings.Repeat("a", MaxBodySize+1000)

	req := &http.Request{
		Body:          io.NopCloser(strings.NewReader(largeBody)),
		Header:        make(http.Header),
		ContentLength: int64(len(largeBody)),
	}
	req.Header.Set("Content-Type", "application/json")

	profile := domain.NewRequestProfile("/v1/chat/completions")

	// Should skip inspection for large body
	err = inspector.Inspect(ctx, req, profile)
	assert.NoError(t, err)
	assert.Empty(t, profile.ModelName)
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
	for i := 0; i < 10; i++ {
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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
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
