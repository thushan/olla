package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathTranslation verifies that the translator sets the correct target path
// This ensures requests are proxied to the correct OpenAI endpoint
func TestPathTranslation(t *testing.T) {
	translator := NewTranslator(createTestLogger())
	ctx := context.Background()

	tests := []struct {
		name               string
		anthropicReq       AnthropicRequest
		expectedTargetPath string
	}{
		{
			name: "non_streaming_message",
			anthropicReq: AnthropicRequest{
				Model:     "claude-sonnet-4-20250929",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{Role: "user", Content: "Hello"},
				},
				Stream: false,
			},
			expectedTargetPath: "/v1/chat/completions",
		},
		{
			name: "streaming_message",
			anthropicReq: AnthropicRequest{
				Model:     "claude-sonnet-4-20250929",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{Role: "user", Content: "Hello"},
				},
				Stream: true,
			},
			expectedTargetPath: "/v1/chat/completions",
		},
		{
			name: "message_with_tools",
			anthropicReq: AnthropicRequest{
				Model:     "claude-sonnet-4-20250929",
				MaxTokens: 1024,
				Messages: []AnthropicMessage{
					{Role: "user", Content: "What's the weather?"},
				},
				Tools: []AnthropicTool{
					{
						Name:        "get_weather",
						Description: "Get weather",
						InputSchema: map[string]interface{}{"type": "object"},
					},
				},
			},
			expectedTargetPath: "/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal request to JSON
			body, err := json.Marshal(tt.anthropicReq)
			require.NoError(t, err)

			// Create HTTP request
			req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(body))
			require.NoError(t, err)

			// Transform request
			transformed, err := translator.TransformRequest(ctx, req)
			require.NoError(t, err)

			// Verify target path is set correctly
			assert.Equal(t, tt.expectedTargetPath, transformed.TargetPath,
				"Target path should be set to OpenAI chat completions endpoint")

			// Verify target path is not empty
			assert.NotEmpty(t, transformed.TargetPath,
				"Target path must be populated for proper routing")
		})
	}
}

// TestPathTranslationPreservesOtherFields verifies that setting TargetPath doesn't affect other fields
func TestPathTranslationPreservesOtherFields(t *testing.T) {
	translator := NewTranslator(createTestLogger())
	ctx := context.Background()

	anthropicReq := AnthropicRequest{
		Model:     "claude-sonnet-4-20250929",
		MaxTokens: 2048,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Test message"},
		},
		System: "You are a helpful assistant",
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(body))
	require.NoError(t, err)

	transformed, err := translator.TransformRequest(ctx, req)
	require.NoError(t, err)

	// Verify target path is set
	assert.Equal(t, "/v1/chat/completions", transformed.TargetPath)

	// Verify other fields are preserved
	assert.Equal(t, "claude-sonnet-4-20250929", transformed.ModelName)
	assert.False(t, transformed.IsStreaming)
	assert.NotNil(t, transformed.OpenAIRequest)
	assert.NotNil(t, transformed.Metadata)
	assert.NotEmpty(t, transformed.OriginalBody)

	// Verify OpenAI request has correct structure
	assert.Equal(t, "claude-sonnet-4-20250929", transformed.OpenAIRequest["model"])
	assert.Equal(t, 2048, transformed.OpenAIRequest["max_tokens"])
	stream, ok := transformed.OpenAIRequest["stream"].(bool)
	require.True(t, ok, "stream should be a boolean")
	assert.False(t, stream)

	// Verify messages contain system prompt
	messages, ok := transformed.OpenAIRequest["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 2) // system + user
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful assistant", messages[0]["content"])
}

// TestTranslatorSetsPathWithoutOllaPrefix verifies that the translator sets the path correctly WITHOUT /olla prefix
// The handler layer is responsible for stripping the /olla prefix, not the translator
func TestTranslatorSetsPathWithoutOllaPrefix(t *testing.T) {
	translator := NewTranslator(createTestLogger())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	reqBody, _ := json.Marshal(anthropicReq)
	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	transformed, err := translator.TransformRequest(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "/v1/chat/completions", transformed.TargetPath,
		"TargetPath should NOT include /olla prefix")
	assert.NotContains(t, transformed.TargetPath, "/olla",
		"TargetPath should never contain /olla prefix")
}
