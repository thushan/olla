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

// TestTransformRequest_WithThinkingField tests that the thinking field is accepted
// This verifies the Thinking field is a known field and won't cause "unknown field" errors
func TestTransformRequest_WithThinkingField(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	tests := []struct {
		name     string
		thinking interface{}
		desc     string
	}{
		{
			name:     "thinking_as_object",
			thinking: map[string]interface{}{"type": "enabled", "budget_tokens": 1000},
			desc:     "Thinking field as object with configuration",
		},
		{
			name:     "thinking_as_string",
			thinking: "enabled",
			desc:     "Thinking field as string",
		},
		{
			name:     "thinking_as_boolean",
			thinking: true,
			desc:     "Thinking field as boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anthropicReq := AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{
						Role:    "user",
						Content: "Solve this complex problem step by step",
					},
				},
				Thinking: tt.thinking,
			}

			body, err := json.Marshal(anthropicReq)
			require.NoError(t, err)

			// Verify the JSON contains the thinking field
			var parsedBody map[string]interface{}
			err = json.Unmarshal(body, &parsedBody)
			require.NoError(t, err)
			assert.Contains(t, parsedBody, "thinking", tt.desc)

			req := &http.Request{
				Body: io.NopCloser(bytes.NewReader(body)),
			}

			// Transform should succeed without "unknown field" error
			result, err := translator.TransformRequest(context.Background(), req)
			require.NoError(t, err, tt.desc)
			assert.NotNil(t, result)

			// Thinking field is Anthropic-specific and not passed to OpenAI format
			// It's used for extended thinking mode which is transparent to translation
			_, hasThinking := result.OpenAIRequest["thinking"]
			assert.False(t, hasThinking, "Thinking field should not be passed to OpenAI format")
		})
	}
}

// TestTransformRequest_SystemPromptAsArray tests system prompt with content blocks
// Anthropic API supports system prompts as arrays of content blocks
func TestTransformRequest_SystemPromptAsArray(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "You are a helpful assistant. ",
			},
			map[string]interface{}{
				"type": "text",
				"text": "Always be concise.",
			},
		},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	messages, ok := result.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 2) // system + user message

	// First message should be system with concatenated text
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful assistant. Always be concise.", messages[0]["content"])

	// Second message should be user
	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "Hello", messages[1]["content"])
}

// TestTransformRequest_SystemPromptArrayWithEmptyBlocks tests handling of empty content blocks
func TestTransformRequest_SystemPromptArrayWithEmptyBlocks(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "",
			},
			map[string]interface{}{
				"type": "text",
				"text": "Only this matters",
			},
		},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	messages, ok := result.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 2)

	// System message should contain only non-empty text
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "Only this matters", messages[0]["content"])
}

// TestTransformRequest_SystemPromptArrayEmpty tests empty system prompt array
func TestTransformRequest_SystemPromptArrayEmpty(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System:    []interface{}{},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	messages, ok := result.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	// Should only have user message, no system message for empty array
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0]["role"])
}

// TestTransformRequest_SystemPromptString tests traditional string system prompt still works
func TestTransformRequest_SystemPromptString(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System:    "You are a helpful assistant",
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	messages, ok := result.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 2)

	// System message should work as before
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful assistant", messages[0]["content"])
}

// TestTransformRequest_CombinedThinkingAndSystemArray tests both new features together
func TestTransformRequest_CombinedThinkingAndSystemArray(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "You are an expert problem solver.",
			},
		},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Solve this puzzle",
			},
		},
		Thinking: map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": 2000,
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, result)

	messages, ok := result.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 2)

	// System message from array
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are an expert problem solver.", messages[0]["content"])

	// User message
	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "Solve this puzzle", messages[1]["content"])
}

// TestCountSystemChars tests token counting with different system prompt formats
func TestCountSystemChars(t *testing.T) {
	tests := []struct {
		name          string
		system        interface{}
		expectedChars int
		desc          string
	}{
		{
			name:          "nil_system",
			system:        nil,
			expectedChars: 0,
			desc:          "Nil system should return 0",
		},
		{
			name:          "empty_string",
			system:        "",
			expectedChars: 0,
			desc:          "Empty string should return 0",
		},
		{
			name:          "simple_string",
			system:        "You are helpful",
			expectedChars: 15,
			desc:          "Simple string should count characters",
		},
		{
			name: "array_single_block",
			system: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello",
				},
			},
			expectedChars: 5,
			desc:          "Single content block should count text",
		},
		{
			name: "array_multiple_blocks",
			system: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello ",
				},
				map[string]interface{}{
					"type": "text",
					"text": "World",
				},
			},
			expectedChars: 11, // "Hello " (6) + "World" (5) = 11
			desc:          "Multiple blocks should sum characters",
		},
		{
			name: "array_with_empty_blocks",
			system: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "",
				},
				map[string]interface{}{
					"type": "text",
					"text": "Test",
				},
			},
			expectedChars: 4,
			desc:          "Empty blocks should not contribute to count",
		},
		{
			name:          "empty_array",
			system:        []interface{}{},
			expectedChars: 0,
			desc:          "Empty array should return 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countSystemChars(tt.system)
			assert.Equal(t, tt.expectedChars, result, tt.desc)
		})
	}
}

// TestEstimateTokens_WithSystemArray tests token estimation with array system prompt
func TestEstimateTokens_WithSystemArray(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "You are a helpful assistant",
			},
		},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	tokens := estimateTokensFromRequest(req)

	// "You are a helpful assistant" (27 chars) + "Hello" (5 chars) = 32 chars
	// 32 / 4 = 8 tokens
	expectedTokens := 8
	assert.Equal(t, expectedTokens, tokens)
}
