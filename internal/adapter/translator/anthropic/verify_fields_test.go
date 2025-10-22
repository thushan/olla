package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONParsingWithThinkingAndSystemArray verifies that JSON with thinking field
// and system array can be parsed without "unknown field" errors
func TestJSONParsingWithThinkingAndSystemArray(t *testing.T) {
	jsonData := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"thinking": {
			"type": "enabled",
			"budget_tokens": 2000
		},
		"system": [
			{
				"type": "text",
				"text": "You are a helpful AI assistant."
			}
		],
		"messages": [
			{
				"role": "user",
				"content": "What is 2+2?"
			}
		]
	}`

	var req AnthropicRequest

	// This should NOT error with "unknown field 'thinking'"
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err, "Should parse JSON with thinking and system array without error")

	// Verify fields were parsed correctly
	assert.Equal(t, "claude-3-5-sonnet-20241022", req.Model)
	assert.Equal(t, 1024, req.MaxTokens)

	// Verify thinking field
	require.NotNil(t, req.Thinking)
	thinkingMap, ok := req.Thinking.(map[string]interface{})
	require.True(t, ok, "Thinking should be a map")
	assert.Equal(t, "enabled", thinkingMap["type"])
	assert.Equal(t, float64(2000), thinkingMap["budget_tokens"]) // JSON numbers are float64

	// Verify system array
	require.NotNil(t, req.System)
	systemArray, ok := req.System.([]interface{})
	require.True(t, ok, "System should be an array")
	require.Len(t, systemArray, 1)

	systemBlock, ok := systemArray[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "text", systemBlock["type"])
	assert.Equal(t, "You are a helpful AI assistant.", systemBlock["text"])

	// Verify messages
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "What is 2+2?", req.Messages[0].Content)
}

// TestJSONParsingWithStringSystem verifies backward compatibility with string system prompts
func TestJSONParsingWithStringSystem(t *testing.T) {
	jsonData := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"system": "You are a helpful AI assistant.",
		"messages": [
			{
				"role": "user",
				"content": "Hello"
			}
		]
	}`

	var req AnthropicRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	// Verify system is a string
	systemStr, ok := req.System.(string)
	require.True(t, ok, "System should be a string")
	assert.Equal(t, "You are a helpful AI assistant.", systemStr)
}

// TestJSONParsingWithThinkingVariants tests different thinking field formats
func TestJSONParsingWithThinkingVariants(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		validate func(t *testing.T, req *AnthropicRequest)
	}{
		{
			name: "thinking_as_object",
			json: `{
				"model": "test",
				"max_tokens": 100,
				"thinking": {"type": "enabled"},
				"messages": [{"role": "user", "content": "test"}]
			}`,
			validate: func(t *testing.T, req *AnthropicRequest) {
				thinkingMap, ok := req.Thinking.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "enabled", thinkingMap["type"])
			},
		},
		{
			name: "thinking_as_string",
			json: `{
				"model": "test",
				"max_tokens": 100,
				"thinking": "enabled",
				"messages": [{"role": "user", "content": "test"}]
			}`,
			validate: func(t *testing.T, req *AnthropicRequest) {
				thinkingStr, ok := req.Thinking.(string)
				require.True(t, ok)
				assert.Equal(t, "enabled", thinkingStr)
			},
		},
		{
			name: "thinking_as_boolean",
			json: `{
				"model": "test",
				"max_tokens": 100,
				"thinking": true,
				"messages": [{"role": "user", "content": "test"}]
			}`,
			validate: func(t *testing.T, req *AnthropicRequest) {
				thinkingBool, ok := req.Thinking.(bool)
				require.True(t, ok)
				assert.True(t, thinkingBool)
			},
		},
		{
			name: "no_thinking_field",
			json: `{
				"model": "test",
				"max_tokens": 100,
				"messages": [{"role": "user", "content": "test"}]
			}`,
			validate: func(t *testing.T, req *AnthropicRequest) {
				assert.Nil(t, req.Thinking)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req AnthropicRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			require.NoError(t, err)
			tt.validate(t, &req)
		})
	}
}

// TestAnthropicRequestMarshalRoundTrip verifies marshal/unmarshal round-trip works
func TestAnthropicRequestMarshalRoundTrip(t *testing.T) {
	original := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "System prompt",
			},
		},
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Thinking: map[string]interface{}{
			"type": "enabled",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var parsed AnthropicRequest
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, original.Model, parsed.Model)
	assert.Equal(t, original.MaxTokens, parsed.MaxTokens)
	assert.NotNil(t, parsed.System)
	assert.NotNil(t, parsed.Thinking)
	assert.Len(t, parsed.Messages, 1)
}
