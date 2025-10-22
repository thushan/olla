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
	"github.com/thushan/olla/internal/logger"
)

// createTestLogger creates a logger for testing
func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func TestTransformRequest_SimpleMessage(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello, world!",
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
	assert.Equal(t, "claude-3-5-sonnet-20241022", result.ModelName)
	assert.False(t, result.IsStreaming)
	assert.Equal(t, "anthropic", result.Metadata["format"])

	openaiReq := result.OpenAIRequest
	assert.Equal(t, "claude-3-5-sonnet-20241022", openaiReq["model"])
	assert.Equal(t, 1024, openaiReq["max_tokens"])
	assert.Equal(t, false, openaiReq["stream"])

	messages, ok := openaiReq["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "Hello, world!", messages[0]["content"])
}

func TestTransformRequest_WithSystemPrompt(t *testing.T) {
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

	// First message should be system
	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful assistant", messages[0]["content"])

	// Second message should be user
	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "Hello", messages[1]["content"])
}

func TestTransformRequest_WithTools(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "What's the weather?",
			},
		},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather information",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
		ToolChoice: "auto",
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	tools, ok := result.OpenAIRequest["tools"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tools, 1)

	assert.Equal(t, "function", tools[0]["type"])
	function, ok := tools[0]["function"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "get_weather", function["name"])
	assert.Equal(t, "Get weather information", function["description"])

	parameters, ok := function["parameters"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", parameters["type"])

	toolChoice := result.OpenAIRequest["tool_choice"]
	assert.Equal(t, "auto", toolChoice)
}

func TestTransformRequest_MultipleTools(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "Check the weather and time",
			},
		},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather information",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				Name:        "get_time",
				Description: "Get current time",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
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

	tools, ok := result.OpenAIRequest["tools"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tools, 2)

	assert.Equal(t, "function", tools[0]["type"])
	function0 := tools[0]["function"].(map[string]interface{})
	assert.Equal(t, "get_weather", function0["name"])

	assert.Equal(t, "function", tools[1]["type"])
	function1 := tools[1]["function"].(map[string]interface{})
	assert.Equal(t, "get_time", function1["name"])
}

func TestConvertToolChoice(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	testCases := []struct {
		name            string
		anthropicChoice interface{}
		expectedChoice  interface{}
		description     string
	}{
		{
			name:            "string_auto",
			anthropicChoice: "auto",
			expectedChoice:  "auto",
			description:     "auto maps to auto",
		},
		{
			name:            "string_any",
			anthropicChoice: "any",
			expectedChoice:  "required",
			description:     "any maps to required",
		},
		{
			name:            "string_none",
			anthropicChoice: "none",
			expectedChoice:  "none",
			description:     "none maps to none",
		},
		{
			name:            "object_auto",
			anthropicChoice: map[string]interface{}{"type": "auto"},
			expectedChoice:  "auto",
			description:     "object form with auto",
		},
		{
			name:            "object_any",
			anthropicChoice: map[string]interface{}{"type": "any"},
			expectedChoice:  "required",
			description:     "object form with any",
		},
		{
			name: "object_tool",
			anthropicChoice: map[string]interface{}{
				"type": "tool",
				"name": "get_weather",
			},
			expectedChoice: map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
			description: "specific tool selection",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := translator.convertToolChoice(tc.anthropicChoice)
			require.NoError(t, err, tc.description)
			assert.Equal(t, tc.expectedChoice, result, tc.description)
		})
	}
}

func TestConvertToolChoice_EdgeCases(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	t.Run("unknown_string_defaults_to_auto", func(t *testing.T) {
		result, err := translator.convertToolChoice("unknown")
		require.NoError(t, err)
		assert.Equal(t, "auto", result)
	})

	t.Run("object_tool_without_name_errors", func(t *testing.T) {
		_, err := translator.convertToolChoice(map[string]interface{}{
			"type": "tool",
			// missing name
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'name' field")
	})

	t.Run("unknown_object_type_defaults_to_auto", func(t *testing.T) {
		result, err := translator.convertToolChoice(map[string]interface{}{
			"type": "unknown",
		})
		require.NoError(t, err)
		assert.Equal(t, "auto", result)
	})

	t.Run("nil_defaults_to_auto", func(t *testing.T) {
		result, err := translator.convertToolChoice(nil)
		require.NoError(t, err)
		assert.Equal(t, "auto", result)
	})
}

func TestConvertMessages_ToolUseAndResult(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			// User asks a question
			{
				Role:    "user",
				Content: "What's the weather in San Francisco?",
			},
			// Assistant responds with tool use
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Let me check that for you.",
					},
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_123",
						"name": "get_weather",
						"input": map[string]interface{}{
							"location": "San Francisco",
							"unit":     "celsius",
						},
					},
				},
			},
			// User provides tool result
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_123",
						"content":     "Temperature is 18°C, partly cloudy",
					},
				},
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
	require.Len(t, messages, 3) // user, assistant, tool

	// Check first user message
	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "What's the weather in San Francisco?", messages[0]["content"])

	// Check assistant message with tool call
	assert.Equal(t, "assistant", messages[1]["role"])
	assert.Equal(t, "Let me check that for you.", messages[1]["content"])

	toolCalls, ok := messages[1]["tool_calls"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, toolCalls, 1)

	assert.Equal(t, "toolu_123", toolCalls[0]["id"])
	assert.Equal(t, "function", toolCalls[0]["type"])

	function, ok := toolCalls[0]["function"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "get_weather", function["name"])

	// Parse arguments JSON string
	var args map[string]interface{}
	err = json.Unmarshal([]byte(function["arguments"].(string)), &args)
	require.NoError(t, err)
	assert.Equal(t, "San Francisco", args["location"])
	assert.Equal(t, "celsius", args["unit"])

	// Check tool result message
	assert.Equal(t, "tool", messages[2]["role"])
	assert.Equal(t, "toolu_123", messages[2]["tool_call_id"])
	assert.Equal(t, "Temperature is 18°C, partly cloudy", messages[2]["content"])
}

func TestTransformRequest_ComplexContent(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "First part ",
					},
					map[string]interface{}{
						"type": "text",
						"text": "second part",
					},
				},
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
	require.Len(t, messages, 1)

	// text blocks get concatenated
	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "First part second part", messages[0]["content"])
}

func TestTransformRequest_MultipleMessages(t *testing.T) {
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
			{
				Role:    "assistant",
				Content: "Hi! How can I help?",
			},
			{
				Role:    "user",
				Content: "Tell me about Go",
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
	require.Len(t, messages, 4)

	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful assistant", messages[0]["content"])

	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "Hello", messages[1]["content"])

	assert.Equal(t, "assistant", messages[2]["role"])
	assert.Equal(t, "Hi! How can I help?", messages[2]["content"])

	assert.Equal(t, "user", messages[3]["role"])
	assert.Equal(t, "Tell me about Go", messages[3]["content"])
}

func TestTransformRequest_EmptyContent(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	t.Run("empty_string_content", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     "claude-3-5-sonnet-20241022",
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "",
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
		assert.Len(t, messages, 0) // empty content gets filtered
	})

	t.Run("empty_text_blocks", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     "claude-3-5-sonnet-20241022",
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role: "user",
					Content: []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "",
						},
					},
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
		assert.Len(t, messages, 0) // empty blocks get filtered
	})
}

func TestTransformRequest_InvalidJSON(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	t.Run("malformed_json", func(t *testing.T) {
		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte("{invalid json"))),
		}

		_, err := translator.TransformRequest(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Anthropic request")
	})

	t.Run("empty_body", func(t *testing.T) {
		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(""))),
		}

		_, err := translator.TransformRequest(context.Background(), req)
		assert.Error(t, err)
	})
}

func TestTransformRequest_OptionalParameters(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	temp := 0.7
	topP := 0.9

	anthropicReq := AnthropicRequest{
		Model:         "claude-3-5-sonnet-20241022",
		MaxTokens:     2048,
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"END", "STOP"},
		Stream:        true,
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

	assert.True(t, result.IsStreaming)

	openaiReq := result.OpenAIRequest
	assert.Equal(t, 0.7, openaiReq["temperature"])
	assert.Equal(t, 0.9, openaiReq["top_p"])
	assert.Equal(t, true, openaiReq["stream"])

	stopSeqs, ok := openaiReq["stop"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"END", "STOP"}, stopSeqs)
}

func TestTransformRequest_AssistantWithOnlyToolCalls(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_456",
						"name": "search",
						"input": map[string]interface{}{
							"query": "golang best practices",
						},
					},
				},
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
	require.Len(t, messages, 1)

	// tool calls only = null content
	assert.Equal(t, "assistant", messages[0]["role"])
	assert.Nil(t, messages[0]["content"])

	toolCalls, ok := messages[0]["tool_calls"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "toolu_456", toolCalls[0]["id"])
}

func TestTransformRequest_UserWithOnlyToolResults(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_789",
						"content":     "Result data here",
					},
				},
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
	require.Len(t, messages, 1)

	// tool results only = tool message
	assert.Equal(t, "tool", messages[0]["role"])
	assert.Equal(t, "toolu_789", messages[0]["tool_call_id"])
	assert.Equal(t, "Result data here", messages[0]["content"])
}

func TestTransformRequest_ToolResultWithStructuredContent(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_struct",
						"content": map[string]interface{}{
							"temperature": 18,
							"conditions":  "partly cloudy",
						},
					},
				},
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
	require.Len(t, messages, 1)

	assert.Equal(t, "tool", messages[0]["role"])
	assert.Equal(t, "toolu_struct", messages[0]["tool_call_id"])

	// structured content becomes json string
	contentStr, ok := messages[0]["content"].(string)
	require.True(t, ok)

	var parsedContent map[string]interface{}
	err = json.Unmarshal([]byte(contentStr), &parsedContent)
	require.NoError(t, err)
	assert.Equal(t, float64(18), parsedContent["temperature"]) // JSON numbers are float64
	assert.Equal(t, "partly cloudy", parsedContent["conditions"])
}

func TestTransformRequest_MultipleToolCalls(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Let me gather that information.",
					},
					map[string]interface{}{
						"type": "tool_use",
						"id":   "tool_1",
						"name": "get_weather",
						"input": map[string]interface{}{
							"location": "NYC",
						},
					},
					map[string]interface{}{
						"type": "tool_use",
						"id":   "tool_2",
						"name": "get_time",
						"input": map[string]interface{}{
							"timezone": "EST",
						},
					},
				},
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
	require.Len(t, messages, 1)

	assert.Equal(t, "assistant", messages[0]["role"])
	assert.Equal(t, "Let me gather that information.", messages[0]["content"])

	toolCalls, ok := messages[0]["tool_calls"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, toolCalls, 2)

	assert.Equal(t, "tool_1", toolCalls[0]["id"])
	func0 := toolCalls[0]["function"].(map[string]interface{})
	assert.Equal(t, "get_weather", func0["name"])

	assert.Equal(t, "tool_2", toolCalls[1]["id"])
	func1 := toolCalls[1]["function"].(map[string]interface{})
	assert.Equal(t, "get_time", func1["name"])
}

func TestConvertToolUse_InvalidData(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	t.Run("missing_id", func(t *testing.T) {
		block := map[string]interface{}{
			"type":  "tool_use",
			"name":  "get_weather",
			"input": map[string]interface{}{},
		}
		result := translator.convertToolUse(block)
		assert.Nil(t, result)
	})

	t.Run("missing_name", func(t *testing.T) {
		block := map[string]interface{}{
			"type":  "tool_use",
			"id":    "tool_123",
			"input": map[string]interface{}{},
		}
		result := translator.convertToolUse(block)
		assert.Nil(t, result)
	})

	t.Run("nil_input", func(t *testing.T) {
		block := map[string]interface{}{
			"type": "tool_use",
			"id":   "tool_123",
			"name": "get_weather",
			// input is nil/missing
		}
		result := translator.convertToolUse(block)
		require.NotNil(t, result)
		function := result["function"].(map[string]interface{})
		assert.Equal(t, "null", function["arguments"])
	})
}

func TestTransformRequest_NoMessages(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	_, err = translator.TransformRequest(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one message is required")
}

func TestTransformRequest_ToolChoiceObjectForm(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "What's the weather?",
			},
		},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		ToolChoice: map[string]interface{}{
			"type": "tool",
			"name": "get_weather",
		},
	}

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	result, err := translator.TransformRequest(context.Background(), req)
	require.NoError(t, err)

	toolChoice, ok := result.OpenAIRequest["tool_choice"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "function", toolChoice["type"])

	function, ok := toolChoice["function"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "get_weather", function["name"])
}

func TestTransformRequest_MixedTextAndToolResults(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Here's the result:",
					},
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "tool_mixed",
						"content":     "Data from tool",
					},
				},
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

	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "Here's the result:", messages[0]["content"])

	assert.Equal(t, "tool", messages[1]["role"])
	assert.Equal(t, "tool_mixed", messages[1]["tool_call_id"])
	assert.Equal(t, "Data from tool", messages[1]["content"])
}

func TestConvertSystemPrompt_AllFormats(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	t.Run("string_format", func(t *testing.T) {
		result := translator.convertSystemPrompt("You are a helpful assistant")
		assert.Equal(t, "You are a helpful assistant", result)
	})

	t.Run("empty_string", func(t *testing.T) {
		result := translator.convertSystemPrompt("")
		assert.Nil(t, result)
	})

	t.Run("interface_array_format", func(t *testing.T) {
		systemBlocks := []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "First part. ",
			},
			map[string]interface{}{
				"type": "text",
				"text": "Second part.",
			},
		}
		result := translator.convertSystemPrompt(systemBlocks)
		assert.Equal(t, "First part. Second part.", result)
	})

	t.Run("interface_array_with_empty_blocks", func(t *testing.T) {
		systemBlocks := []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "",
			},
		}
		result := translator.convertSystemPrompt(systemBlocks)
		assert.Nil(t, result)
	})

	t.Run("strongly_typed_contentblock_array", func(t *testing.T) {
		contentBlocks := []ContentBlock{
			{
				Type: "text",
				Text: "Strongly-typed first part. ",
			},
			{
				Type: "text",
				Text: "Strongly-typed second part.",
			},
		}
		result := translator.convertSystemPrompt(contentBlocks)
		assert.Equal(t, "Strongly-typed first part. Strongly-typed second part.", result)
	})

	t.Run("strongly_typed_contentblock_with_empty_text", func(t *testing.T) {
		contentBlocks := []ContentBlock{
			{
				Type: "text",
				Text: "",
			},
		}
		result := translator.convertSystemPrompt(contentBlocks)
		assert.Nil(t, result)
	})

	t.Run("strongly_typed_contentblock_mixed_types", func(t *testing.T) {
		contentBlocks := []ContentBlock{
			{
				Type: "image",
				Text: "",
			},
			{
				Type: "text",
				Text: "Valid text content",
			},
			{
				Type: "tool_use",
				Text: "",
			},
		}
		result := translator.convertSystemPrompt(contentBlocks)
		assert.Equal(t, "Valid text content", result)
	})

	t.Run("nil_system_prompt", func(t *testing.T) {
		result := translator.convertSystemPrompt(nil)
		assert.Nil(t, result)
	})

	t.Run("unsupported_type", func(t *testing.T) {
		result := translator.convertSystemPrompt(42)
		assert.Nil(t, result)
	})
}

func TestTransformRequest_SystemPromptWithContentBlocks(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	anthropicReq := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "You are a helpful ",
			},
			map[string]interface{}{
				"type": "text",
				"text": "AI assistant.",
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

	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a helpful AI assistant.", messages[0]["content"])

	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "Hello", messages[1]["content"])
}

func TestTransformRequest_StronglyTypedSystemPrompt(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	req := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: "What's 2+2?",
			},
		},
	}

	req.System = []ContentBlock{
		{
			Type: "text",
			Text: "You are a mathematics expert. ",
		},
		{
			Type: "text",
			Text: "Provide precise answers.",
		},
	}

	// test convertMessages directly since json marshalling would convert to []interface{}
	messages, err := translator.convertMessages(req.Messages, req.System)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	assert.Equal(t, "system", messages[0]["role"])
	assert.Equal(t, "You are a mathematics expert. Provide precise answers.", messages[0]["content"])

	assert.Equal(t, "user", messages[1]["role"])
	assert.Equal(t, "What's 2+2?", messages[1]["content"])
}
