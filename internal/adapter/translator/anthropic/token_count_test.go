package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountTokens(t *testing.T) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	tests := []struct {
		name          string
		request       AnthropicRequest
		expectedMin   int
		expectedMax   int
		expectedExact int
	}{
		{
			name: "simple string message",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role:    "user",
						Content: "Hello, world!", // 13 chars = 3 tokens (13/4 = 3.25, truncated to 3)
					},
				},
			},
			expectedExact: 3,
		},
		{
			name: "message with system prompt",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				System:    "You are a helpful assistant.", // 28 chars
				Messages: []AnthropicMessage{
					{
						Role:    "user",
						Content: "What is the weather?", // 20 chars
					},
				},
			},
			expectedExact: 12, // (28 + 20) / 4 = 12
		},
		{
			name: "multiple messages",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role:    "user",
						Content: "First message", // 13 chars
					},
					{
						Role:    "assistant",
						Content: "Response", // 8 chars
					},
					{
						Role:    "user",
						Content: "Follow up", // 9 chars
					},
				},
			},
			expectedExact: 7, // (13 + 8 + 9) / 4 = 7.5, truncated to 7
		},
		{
			name: "content blocks with text",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "This is a text block", // 20 chars
							},
						},
					},
				},
			},
			expectedExact: 5, // 20 / 4 = 5
		},
		{
			name: "complex content with tool use",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "Use a tool", // 10 chars
							},
						},
					},
					{
						Role: "assistant",
						Content: []ContentBlock{
							{
								Type: "tool_use",
								ID:   "toolu_123",
								Name: "get_weather", // 11 chars
								Input: map[string]any{
									"location": "Sydney", // JSON: {"location":"Sydney"} = 22 chars
								},
							},
						},
					},
				},
			},
			expectedMin: 10, // At least (10 + 11 + 22) / 4 = 10.75 = 10
			expectedMax: 11, // Could be 11 depending on rounding
		},
		{
			name: "tool result content",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type:      "tool_result",
								ToolUseID: "toolu_123",
								Content:   "The weather is sunny", // 20 chars
							},
						},
					},
				},
			},
			expectedExact: 5, // 20 / 4 = 5
		},
		{
			name: "empty request",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages:  []AnthropicMessage{},
			},
			expectedExact: 1, // Minimum 1 token for empty requests
		},
		{
			name: "long system prompt",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				System:    "You are a highly capable AI assistant with expertise in multiple domains. You should provide detailed, accurate, and helpful responses to all queries.", // 150 chars
				Messages: []AnthropicMessage{
					{
						Role:    "user",
						Content: "Hello!", // 7 chars (including exclamation)
					},
				},
			},
			expectedExact: 39, // (150 + 7) / 4 = 39.25, truncated to 39
		},
		{
			name: "mixed content types",
			request: AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				System:    "System prompt", // 13 chars
				Messages: []AnthropicMessage{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "Text block one", // 14 chars
							},
							{
								Type: "text",
								Text: "Text block two", // 14 chars
							},
						},
					},
					{
						Role:    "assistant",
						Content: "String response", // 15 chars
					},
				},
			},
			expectedExact: 14, // (13 + 14 + 14 + 15) / 4 = 14
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize request to JSON
			reqBody, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			// Create HTTP request
			req, err := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(reqBody))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Call CountTokens
			ctx := context.Background()
			resp, err := trans.CountTokens(ctx, req)
			if err != nil {
				t.Fatalf("CountTokens failed: %v", err)
			}

			// Verify response structure
			if resp == nil {
				t.Fatal("Expected non-nil response")
			}

			// Verify token count
			if tt.expectedExact > 0 {
				if resp.InputTokens != tt.expectedExact {
					t.Errorf("Expected exactly %d tokens, got %d", tt.expectedExact, resp.InputTokens)
				}
			} else {
				// Range check for cases where exact count may vary slightly
				if resp.InputTokens < tt.expectedMin || resp.InputTokens > tt.expectedMax {
					t.Errorf("Expected tokens in range [%d, %d], got %d", tt.expectedMin, tt.expectedMax, resp.InputTokens)
				}
			}

			t.Logf("Token count: input=%d", resp.InputTokens)
		})
	}
}

func TestCountTokensWithRawJSON(t *testing.T) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	tests := []struct {
		name          string
		jsonBody      string
		expectedExact int
	}{
		{
			name: "raw JSON with untyped content blocks",
			jsonBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "Hello world"
							}
						]
					}
				]
			}`,
			expectedExact: 2, // "Hello world" = 11 chars, 11/4 = 2 (truncated)
		},
		{
			name: "tool result with nested content",
			jsonBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "tool_result",
								"tool_use_id": "toolu_123",
								"content": "Result text"
							}
						]
					}
				]
			}`,
			expectedExact: 2, // "Result text" = 11 chars, 11/4 = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader([]byte(tt.jsonBody)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			ctx := context.Background()
			resp, err := trans.CountTokens(ctx, req)
			if err != nil {
				t.Fatalf("CountTokens failed: %v", err)
			}

			if resp.InputTokens != tt.expectedExact {
				t.Errorf("Expected exactly %d tokens, got %d", tt.expectedExact, resp.InputTokens)
			}
		})
	}
}

func TestCountTokensErrors(t *testing.T) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	tests := []struct {
		name        string
		body        io.Reader
		shouldError bool
	}{
		{
			name:        "invalid JSON",
			body:        bytes.NewReader([]byte(`{invalid json`)),
			shouldError: true,
		},
		{
			name:        "empty body",
			body:        bytes.NewReader([]byte(``)),
			shouldError: true,
		},
		{
			name: "valid minimal request",
			body: bytes.NewReader([]byte(`{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": []
			}`)),
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/v1/messages/count_tokens", tt.body)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			ctx := context.Background()
			resp, err := trans.CountTokens(ctx, req)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if resp == nil {
					t.Error("Expected non-nil response")
				}
			}
		})
	}
}

// TestCountTokensMatchesPythonReference verifies our implementation matches
// the Python reference from anthropic-proxy.py
func TestCountTokensMatchesPythonReference(t *testing.T) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	// This test case directly mirrors the Python implementation logic
	testCase := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System:    "You are helpful", // 15 chars
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello", // 5 chars
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Hi there", // 8 chars
					},
				},
			},
		},
	}

	// Python logic: total_chars = len(system) + sum(len(content) for each message)
	// = 15 + 5 + 8 = 28
	// token_count = max(1, 28 // 4) = max(1, 7) = 7

	reqBody, err := json.Marshal(testCase)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	resp, err := trans.CountTokens(ctx, req)
	if err != nil {
		t.Fatalf("CountTokens failed: %v", err)
	}

	expectedTokens := 7 // (15 + 5 + 8) / 4 = 7
	if resp.InputTokens != expectedTokens {
		t.Errorf("Expected %d tokens to match Python reference, got %d", expectedTokens, resp.InputTokens)
	}
}

// TestCountTokensWireResponse verifies that SerialiseCountTokens emits only
// {"input_tokens":N} — no output_tokens or total_tokens fields.
// Anthropic's spec, vLLM, lmdeploy, bifrost, and litellm all define this shape.
func TestCountTokensWireResponse(t *testing.T) {
	t.Parallel()

	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	reqBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hello world"}]
	}`)

	req, err := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := trans.CountTokens(context.Background(), req)
	if err != nil {
		t.Fatalf("CountTokens failed: %v", err)
	}
	if resp.InputTokens == 0 {
		t.Fatal("expected non-zero InputTokens")
	}

	wireBytes, err := trans.SerialiseCountTokens(resp)
	if err != nil {
		t.Fatalf("SerialiseCountTokens failed: %v", err)
	}

	var wireMap map[string]any
	if err := json.Unmarshal(wireBytes, &wireMap); err != nil {
		t.Fatalf("wire response is not valid JSON: %v", err)
	}

	// Must contain input_tokens
	if _, ok := wireMap["input_tokens"]; !ok {
		t.Error("wire response missing input_tokens field")
	}

	// Must NOT contain output_tokens or total_tokens
	if _, ok := wireMap["output_tokens"]; ok {
		t.Error("wire response must not contain output_tokens (not part of Anthropic count_tokens spec)")
	}
	if _, ok := wireMap["total_tokens"]; ok {
		t.Error("wire response must not contain total_tokens (not part of Anthropic count_tokens spec)")
	}

	// Must contain exactly one field
	if len(wireMap) != 1 {
		t.Errorf("wire response must have exactly 1 field, got %d: %v", len(wireMap), wireMap)
	}
}

// TestEstimateInputTokens verifies the streaming token seeding path.
func TestEstimateInputTokens(t *testing.T) {
	t.Parallel()

	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	body := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hello, this is a test message for token estimation."}]
	}`)

	estimate := trans.EstimateInputTokens(body)
	if estimate <= 0 {
		t.Errorf("expected positive token estimate for non-empty prompt, got %d", estimate)
	}
}

// TestEstimateInputTokens_InvalidBody returns 0 without panicking on malformed input.
func TestEstimateInputTokens_InvalidBody(t *testing.T) {
	t.Parallel()

	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	estimate := trans.EstimateInputTokens([]byte(`{not json`))
	if estimate != 0 {
		t.Errorf("expected 0 for invalid body, got %d", estimate)
	}
}

func BenchmarkCountTokens(b *testing.B) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	reqBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"system": "You are a helpful assistant.",
		"messages": [{"role": "user", "content": "Hello, world!"}]
	}`)

	b.ResetTimer()
	for range b.N {
		req, _ := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		_, _ = trans.CountTokens(context.Background(), req)
	}
}

// TestEstimateInputTokens_WithTools verifies that tool definitions are included in the
// character estimate. Claude Code sends large tool schemas (10-20k tokens each), so
// omitting them causes a large systematic undercount.
func TestEstimateInputTokens_WithTools(t *testing.T) {
	t.Parallel()

	baseReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What is the weather?"},
		},
	}

	reqWithTools := baseReq
	reqWithTools.Tools = []AnthropicTool{
		{
			Name:        "get_weather",
			Description: "Retrieves current weather for a given location.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City and state, e.g. Sydney, NSW",
					},
				},
				"required": []string{"location"},
			},
		},
	}

	baseCount := estimateTokensFromRequest(&baseReq)
	withToolsCount := estimateTokensFromRequest(&reqWithTools)

	assert.Greater(t, withToolsCount, baseCount,
		"request with tools must yield a larger token estimate than the same request without")
}

// TestEstimateInputTokens_ToolOnlyDeltaMatchesCharsDiv4 verifies that the tool schema
// contribution to the estimate is roughly len(schema JSON) / 4.
func TestEstimateInputTokens_ToolOnlyDeltaMatchesCharsDiv4(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	tool := AnthropicTool{
		Name:        "search",
		Description: "Search the web",
		InputSchema: schema,
	}

	// Manually compute expected char count for the tool.
	schemaJSON, err := json.Marshal(schema)
	require.NoError(t, err)
	expectedChars := len(tool.Name) + len(tool.Description) + len(schemaJSON)
	expectedTokens := expectedChars / 4

	got := countToolDefinitionChars(&tool)
	assert.Equal(t, expectedChars, got, "countToolDefinitionChars must match name+description+schema len")

	// estimateTokensFromRequest for a tools-only request (empty messages, single tool)
	// must be at least expectedTokens.
	req := AnthropicRequest{
		Model:     "test",
		MaxTokens: 1,
		Messages:  []AnthropicMessage{{Role: "user", Content: "hi"}},
		Tools:     []AnthropicTool{tool},
	}
	total := estimateTokensFromRequest(&req)
	assert.GreaterOrEqual(t, total, expectedTokens,
		"total estimate must be at least the tool's contribution")
}

func BenchmarkCountTokensLargeRequest(b *testing.B) {
	trans := mustNewTranslator(createTestLogger(), createTestConfig())

	// Large request with multiple messages and content blocks
	messages := make([]map[string]any, 50)
	for i := range messages {
		messages[i] = map[string]any{
			"role":    "user",
			"content": "This is a test message with reasonable length to simulate real usage patterns.",
		}
	}

	reqData := map[string]any{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"system":     "You are a helpful assistant with detailed knowledge.",
		"messages":   messages,
	}
	reqBody, _ := json.Marshal(reqData)

	b.ResetTimer()
	for range b.N {
		req, _ := http.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		_, _ = trans.CountTokens(context.Background(), req)
	}
}
