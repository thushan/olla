package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/thushan/olla/internal/config"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/logger"
)

const (
	MessageFormat     = "anthropic"
	ClaudeSonnetModel = "claude-sonnet-4-20250929"
	ClaudeOpusModel   = "claude-opus-4-1-20250805"
	ClaudeHaikuModel  = "claude-haiku-4-5-20251001"
)

// createIntegrationTestLogger creates a logger for integration tests
func createIntegrationTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}
func createTestConfig() config.AnthropicTranslatorConfig {
	return config.AnthropicTranslatorConfig{
		Enabled:        true,
		MaxMessageSize: 10 << 20, // 10MB
	}
}

// createHTTPRequest creates a mock HTTP request from an Anthropic request
func createHTTPRequest(t *testing.T, anthropicReq AnthropicRequest) *http.Request {
	t.Helper()

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err, "Failed to marshal Anthropic request")

	req, err := http.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	require.NoError(t, err, "Failed to create HTTP request")

	return req
}

// simulateBackendResponse creates a mock OpenAI-format backend response
func simulateBackendResponse(content string, toolCalls []map[string]interface{}, finishReason string) map[string]interface{} {
	message := map[string]interface{}{
		"role": "assistant",
	}

	// Set content - null if only tool calls
	if content != "" {
		message["content"] = content
	} else if len(toolCalls) > 0 {
		message["content"] = nil
	}

	// Add tool calls if present - convert to []interface{} for proper type assertion
	if len(toolCalls) > 0 {
		converted := make([]interface{}, len(toolCalls))
		for i, tc := range toolCalls {
			converted[i] = tc
		}
		message["tool_calls"] = converted
	}

	return map[string]interface{}{
		"id":    "chatcmpl-test-123",
		"model": ClaudeSonnetModel,
		"choices": []interface{}{
			map[string]interface{}{
				"message":       message,
				"finish_reason": finishReason,
				"index":         0,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(25),
			"completion_tokens": float64(15),
			"total_tokens":      float64(40),
		},
	}
}

// TestAnthropicToOpenAIToAnthropic_RoundTrip tests complete request-response cycle
// Validates that data survives the full transformation pipeline:
// Anthropic request → OpenAI format → backend response → Anthropic response
func TestAnthropicToOpenAIToAnthropic_RoundTrip(t *testing.T) {
	translator := NewTranslator(createIntegrationTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("simple_text_conversation", func(t *testing.T) {
		// 1. Create Anthropic request
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "Hello, how are you?",
				},
			},
		}

		// 2. Transform request to OpenAI format
		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err, "Request transformation should succeed")

		// 3. Verify OpenAI request format
		assert.Equal(t, ClaudeSonnetModel, transformed.ModelName)
		assert.False(t, transformed.IsStreaming)
		assert.Equal(t, MessageFormat, transformed.Metadata["format"])

		openaiReq := transformed.OpenAIRequest
		assert.Equal(t, ClaudeSonnetModel, openaiReq["model"])
		assert.Equal(t, 1024, openaiReq["max_tokens"])
		assert.Equal(t, false, openaiReq["stream"])

		messages, ok := openaiReq["messages"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, messages, 1)
		assert.Equal(t, "user", messages[0]["role"])
		assert.Equal(t, "Hello, how are you?", messages[0]["content"])

		// 4. Simulate backend response
		backendResp := simulateBackendResponse(
			"I'm doing well, thank you for asking! How can I help you today?",
			nil,
			"stop",
		)

		// 5. Transform response back to Anthropic format
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err, "Response transformation should succeed")

		// 6. Verify round-trip preservation
		resp, ok := anthropicResp.(AnthropicResponse)
		require.True(t, ok, "Response should be AnthropicResponse type")

		assert.Equal(t, "message", resp.Type)
		assert.Equal(t, "assistant", resp.Role)
		assert.Equal(t, ClaudeSonnetModel, resp.Model)
		assert.Equal(t, "end_turn", resp.StopReason)
		assert.NotEmpty(t, resp.ID)

		require.Len(t, resp.Content, 1)
		assert.Equal(t, "text", resp.Content[0].Type)
		assert.Equal(t, "I'm doing well, thank you for asking! How can I help you today?", resp.Content[0].Text)

		assert.Equal(t, 25, resp.Usage.InputTokens)
		assert.Equal(t, 15, resp.Usage.OutputTokens)
	})

	t.Run("multi_turn_conversation", func(t *testing.T) {
		// Test multi-turn conversation with history
		anthropicReq := AnthropicRequest{
			Model:     ClaudeOpusModel,
			MaxTokens: 2048,
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "What is the capital of France?",
				},
				{
					Role:    "assistant",
					Content: "The capital of France is Paris.",
				},
				{
					Role:    "user",
					Content: "What about Germany?",
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify conversation history preserved
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 3)
		assert.Equal(t, "user", messages[0]["role"])
		assert.Equal(t, "assistant", messages[1]["role"])
		assert.Equal(t, "user", messages[2]["role"])
		assert.Equal(t, "What about Germany?", messages[2]["content"])

		// Simulate response with correct model
		backendResp := simulateBackendResponse("The capital of Germany is Berlin.", nil, "stop")
		backendResp["model"] = ClaudeOpusModel
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, ClaudeOpusModel, resp.Model)
		assert.Equal(t, "The capital of Germany is Berlin.", resp.Content[0].Text)
	})

	t.Run("with_system_prompt", func(t *testing.T) {
		// Test system prompt injection
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			System:    "You are a helpful assistant that speaks like a pirate.",
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "Hello",
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify system prompt injected as first message
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 2)
		assert.Equal(t, "system", messages[0]["role"])
		assert.Equal(t, "You are a helpful assistant that speaks like a pirate.", messages[0]["content"])
		assert.Equal(t, "user", messages[1]["role"])

		// Simulate response
		backendResp := simulateBackendResponse("Ahoy there, matey! How can I help ye today?", nil, "stop")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "Ahoy there, matey! How can I help ye today?", resp.Content[0].Text)
	})

	t.Run("with_optional_parameters", func(t *testing.T) {
		// Test parameter mapping
		temp := 0.7
		topP := 0.9

		anthropicReq := AnthropicRequest{
			Model:         ClaudeSonnetModel,
			MaxTokens:     2048,
			Temperature:   &temp,
			TopP:          &topP,
			StopSequences: []string{"END", "STOP"},
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "Count to 5",
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify parameters mapped correctly
		assert.Equal(t, 2048, transformed.OpenAIRequest["max_tokens"])
		assert.Equal(t, 0.7, transformed.OpenAIRequest["temperature"])
		assert.Equal(t, 0.9, transformed.OpenAIRequest["top_p"])
		assert.Equal(t, []string{"END", "STOP"}, transformed.OpenAIRequest["stop"])

		// Simulate response
		backendResp := simulateBackendResponse("1, 2, 3, 4, 5", nil, "stop")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "1, 2, 3, 4, 5", resp.Content[0].Text)
	})

	t.Run("finish_reason_mapping", func(t *testing.T) {
		// Test all finish_reason mappings
		testCases := []struct {
			name               string
			finishReason       string
			expectedStopReason string
		}{
			{"stop_to_end_turn", "stop", "end_turn"},
			{"length_to_max_tokens", "length", "max_tokens"},
			{"unknown_defaults_to_end_turn", "content_filter", "end_turn"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				anthropicReq := AnthropicRequest{
					Model:     ClaudeSonnetModel,
					MaxTokens: 10,
					Messages: []AnthropicMessage{
						{Role: "user", Content: "Hi"},
					},
				}

				httpReq := createHTTPRequest(t, anthropicReq)
				_, err := translator.TransformRequest(ctx, httpReq)
				require.NoError(t, err)

				backendResp := simulateBackendResponse("Response", nil, tc.finishReason)
				anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
				require.NoError(t, err)

				resp := anthropicResp.(AnthropicResponse)
				assert.Equal(t, tc.expectedStopReason, resp.StopReason,
					"finish_reason %s should map to stop_reason %s", tc.finishReason, tc.expectedStopReason)
			})
		}
	})

	t.Run("message_id_generation", func(t *testing.T) {
		// Test that message IDs are generated
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Test"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		backendResp := simulateBackendResponse("Response", nil, "stop")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.NotEmpty(t, resp.ID)
		assert.Contains(t, resp.ID, "msg_", "Message ID should start with msg_ prefix")
	})

	t.Run("complex_content_blocks", func(t *testing.T) {
		// Test content with multiple text blocks (should concatenate)
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
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

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify text blocks concatenated
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 1)
		assert.Equal(t, "First part second part", messages[0]["content"])

		// Round-trip response
		backendResp := simulateBackendResponse("Combined response", nil, "stop")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "Combined response", resp.Content[0].Text)
	})
}

// TestAnthropicToolCalling_RoundTrip tests complete tool calling workflow
// Validates the full tool calling cycle:
// Request with tools → OpenAI format → Response with tool_use → Final assistant reply
func TestAnthropicToolCalling_RoundTrip(t *testing.T) {
	translator := NewTranslator(createIntegrationTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("single_tool_definition", func(t *testing.T) {
		// 1. Create request with tool definition
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role:    "user",
					Content: "What's the weather in Melbourne?",
				},
			},
			Tools: []AnthropicTool{
				{
					Name:        "get_weather",
					Description: "Get current weather for a location",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "City name",
							},
							"unit": map[string]interface{}{
								"type": "string",
								"enum": []string{"celsius", "fahrenheit"},
							},
						},
						"required": []string{"location"},
					},
				},
			},
			ToolChoice: "auto",
		}

		// 2. Transform request to OpenAI
		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// 3. Verify tool definition conversion
		tools, ok := transformed.OpenAIRequest["tools"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, tools, 1)

		assert.Equal(t, "function", tools[0]["type"])
		function := tools[0]["function"].(map[string]interface{})
		assert.Equal(t, "get_weather", function["name"])
		assert.Equal(t, "Get current weather for a location", function["description"])

		// Verify input_schema mapped to parameters
		parameters := function["parameters"].(map[string]interface{})
		assert.Equal(t, "object", parameters["type"])
		properties := parameters["properties"].(map[string]interface{})
		assert.Contains(t, properties, "location")

		// Verify tool_choice mapping
		assert.Equal(t, "auto", transformed.OpenAIRequest["tool_choice"])

		// 4. Simulate backend response with tool call
		toolCall := map[string]interface{}{
			"id":   "call_weather_123",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "get_weather",
				"arguments": `{"location":"Melbourne","unit":"celsius"}`,
			},
		}
		backendResp := simulateBackendResponse(
			"Let me check the weather for you.",
			[]map[string]interface{}{toolCall},
			"tool_calls",
		)

		// 5. Transform response to Anthropic
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		// 6. Verify tool_use block creation
		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "tool_use", resp.StopReason)

		// When text content and tool calls are present, we should have both blocks
		// Find text and tool_use blocks
		var textBlock, toolBlock *ContentBlock
		for i := range resp.Content {
			switch resp.Content[i].Type {
			case "text":
				textBlock = &resp.Content[i]
			case "tool_use":
				toolBlock = &resp.Content[i]
			}
		}

		// Verify both blocks present
		require.NotNil(t, textBlock, "Should have text block")
		require.NotNil(t, toolBlock, "Should have tool_use block")

		// Verify text content
		assert.Equal(t, "Let me check the weather for you.", textBlock.Text)

		// Verify tool_use content
		assert.Equal(t, "call_weather_123", toolBlock.ID)
		assert.Equal(t, "get_weather", toolBlock.Name)

		// Verify input arguments parsed correctly
		require.NotNil(t, toolBlock.Input)
		assert.Equal(t, "Melbourne", toolBlock.Input["location"])
		assert.Equal(t, "celsius", toolBlock.Input["unit"])
	})

	t.Run("tool_result_round_trip", func(t *testing.T) {
		// Test sending tool result back to assistant
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				// Original user question
				{
					Role:    "user",
					Content: "What's the weather?",
				},
				// Assistant's tool call
				{
					Role: "assistant",
					Content: []interface{}{
						map[string]interface{}{
							"type": "tool_use",
							"id":   "toolu_weather_123",
							"name": "get_weather",
							"input": map[string]interface{}{
								"location": "Melbourne",
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
							"tool_use_id": "toolu_weather_123",
							"content":     "Temperature: 18°C, partly cloudy",
						},
					},
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify OpenAI message structure
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 3) // user, assistant, tool

		// Check user message
		assert.Equal(t, "user", messages[0]["role"])
		assert.Equal(t, "What's the weather?", messages[0]["content"])

		// Check assistant message with tool call
		assert.Equal(t, "assistant", messages[1]["role"])
		toolCalls := messages[1]["tool_calls"].([]map[string]interface{})
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "toolu_weather_123", toolCalls[0]["id"])
		assert.Equal(t, "get_weather", toolCalls[0]["function"].(map[string]interface{})["name"])

		// Check tool result message
		assert.Equal(t, "tool", messages[2]["role"])
		assert.Equal(t, "toolu_weather_123", messages[2]["tool_call_id"])
		assert.Equal(t, "Temperature: 18°C, partly cloudy", messages[2]["content"])

		// Simulate final assistant response
		backendResp := simulateBackendResponse(
			"Based on the weather data, it's currently 18°C and partly cloudy in Melbourne.",
			nil,
			"stop",
		)

		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "end_turn", resp.StopReason)
		assert.Contains(t, resp.Content[0].Text, "18°C")
	})

	t.Run("multiple_tools", func(t *testing.T) {
		// Test with multiple tool definitions
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Check weather and time"}},
			Tools: []AnthropicTool{
				{
					Name:        "get_weather",
					Description: "Get weather",
					InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
				},
				{
					Name:        "get_time",
					Description: "Get current time",
					InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Verify both tools converted
		tools := transformed.OpenAIRequest["tools"].([]map[string]interface{})
		require.Len(t, tools, 2)
		assert.Equal(t, "get_weather", tools[0]["function"].(map[string]interface{})["name"])
		assert.Equal(t, "get_time", tools[1]["function"].(map[string]interface{})["name"])
	})

	t.Run("multiple_tool_calls_in_response", func(t *testing.T) {
		// Test response with multiple simultaneous tool calls
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Get info"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Simulate multiple tool calls
		toolCalls := []map[string]interface{}{
			{
				"id":   "call_1",
				"type": "function",
				"function": map[string]interface{}{
					"name":      "get_weather",
					"arguments": `{"location":"NYC"}`,
				},
			},
			{
				"id":   "call_2",
				"type": "function",
				"function": map[string]interface{}{
					"name":      "get_time",
					"arguments": `{"timezone":"EST"}`,
				},
			},
		}

		backendResp := simulateBackendResponse("Gathering information.", toolCalls, "tool_calls")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "tool_use", resp.StopReason)

		// Find blocks by type
		var textBlock *ContentBlock
		var toolBlocks []ContentBlock
		for i := range resp.Content {
			if resp.Content[i].Type == "text" {
				textBlock = &resp.Content[i]
			} else if resp.Content[i].Type == "tool_use" {
				toolBlocks = append(toolBlocks, resp.Content[i])
			}
		}

		require.NotNil(t, textBlock, "Should have text block")
		require.Len(t, toolBlocks, 2, "Should have 2 tool_use blocks")

		assert.Equal(t, "Gathering information.", textBlock.Text)
		assert.Equal(t, "get_weather", toolBlocks[0].Name)
		assert.Equal(t, "get_time", toolBlocks[1].Name)
	})

	t.Run("tool_choice_variations", func(t *testing.T) {
		// Test different tool_choice values
		testCases := []struct {
			name               string
			toolChoice         interface{}
			expectedOpenAI     interface{}
			expectedShouldFail bool
		}{
			{"auto", "auto", "auto", false},
			{"any_to_required", "any", "required", false},
			{"none", "none", "none", false},
			{
				"specific_tool",
				map[string]interface{}{"type": "tool", "name": "get_weather"},
				map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name": "get_weather",
					},
				},
				false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				anthropicReq := AnthropicRequest{
					Model:     ClaudeSonnetModel,
					MaxTokens: 1024,
					Messages:  []AnthropicMessage{{Role: "user", Content: "Test"}},
					Tools: []AnthropicTool{
						{
							Name:        "get_weather",
							Description: "Get weather",
							InputSchema: map[string]interface{}{"type": "object"},
						},
					},
					ToolChoice: tc.toolChoice,
				}

				httpReq := createHTTPRequest(t, anthropicReq)
				transformed, err := translator.TransformRequest(ctx, httpReq)

				if tc.expectedShouldFail {
					assert.Error(t, err)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tc.expectedOpenAI, transformed.OpenAIRequest["tool_choice"])
			})
		}
	})

	t.Run("tool_only_response", func(t *testing.T) {
		// Test response with tool calls but no text content
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Search something"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Simulate tool-only response (no text)
		toolCall := map[string]interface{}{
			"id":   "call_search",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "web_search",
				"arguments": `{"query":"test"}`,
			},
		}
		backendResp := map[string]interface{}{
			"id":    "chatcmpl-test",
			"model": ClaudeSonnetModel,
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":       "assistant",
						"content":    nil, // No text content
						"tool_calls": []interface{}{toolCall},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     float64(10),
				"completion_tokens": float64(5),
			},
		}

		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, "tool_use", resp.StopReason)

		// Should only have tool_use block, no empty text block
		require.Len(t, resp.Content, 1)
		assert.Equal(t, "tool_use", resp.Content[0].Type)
		assert.Equal(t, "web_search", resp.Content[0].Name)
	})

	t.Run("complex_tool_arguments", func(t *testing.T) {
		// Test tool call with complex nested arguments
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Process data"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Simulate response with complex arguments
		complexArgs := map[string]interface{}{
			"filters": map[string]interface{}{
				"category": "electronics",
				"price_range": map[string]interface{}{
					"min": 100,
					"max": 500,
				},
			},
			"sort": "price_asc",
			"tags": []string{"sale", "featured"},
		}
		argsJSON, _ := json.Marshal(complexArgs)

		toolCall := map[string]interface{}{
			"id":   "call_complex",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "process_data",
				"arguments": string(argsJSON),
			},
		}

		backendResp := simulateBackendResponse("Processing", []map[string]interface{}{toolCall}, "tool_calls")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)

		// Find the tool_use block
		var toolBlock *ContentBlock
		for i := range resp.Content {
			if resp.Content[i].Type == "tool_use" {
				toolBlock = &resp.Content[i]
				break
			}
		}
		require.NotNil(t, toolBlock, "Should have tool_use block")

		// Verify complex structure preserved
		assert.Equal(t, "tool_use", toolBlock.Type)
		filters := toolBlock.Input["filters"].(map[string]interface{})
		assert.Equal(t, "electronics", filters["category"])

		priceRange := filters["price_range"].(map[string]interface{})
		assert.Equal(t, float64(100), priceRange["min"])
		assert.Equal(t, float64(500), priceRange["max"])

		tags := toolBlock.Input["tags"].([]interface{})
		require.Len(t, tags, 2)
		assert.Equal(t, "sale", tags[0])
	})
}

// TestAnthropicEdgeCases_RoundTrip tests edge cases and error scenarios
// Validates that the translator handles unusual inputs gracefully
func TestAnthropicEdgeCases_RoundTrip(t *testing.T) {
	translator := NewTranslator(createIntegrationTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("empty_messages_array", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{}, // Empty
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one message is required")
	})

	t.Run("empty_content_string", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{Role: "user", Content: ""}, // Empty content
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Empty content should be filtered out
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		assert.Len(t, messages, 0)
	})

	t.Run("invalid_json_in_tool_arguments", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Test"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Simulate backend response with malformed JSON
		toolCall := map[string]interface{}{
			"id":   "call_bad",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "test",
				"arguments": `{invalid json}`, // Malformed JSON
			},
		}

		backendResp := simulateBackendResponse("", []map[string]interface{}{toolCall}, "tool_calls")
		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)

		// Should handle gracefully - either error or empty input
		if err != nil {
			assert.Contains(t, err.Error(), "json")
		} else {
			resp := anthropicResp.(AnthropicResponse)
			// If no error, input should be empty map
			assert.NotNil(t, resp.Content)
		}
	})

	t.Run("missing_backend_choices", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{{Role: "user", Content: "Test"}},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		_, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Backend response missing choices
		backendResp := map[string]interface{}{
			"id":    "chatcmpl-nochoices",
			"model": ClaudeSonnetModel,
			// Missing choices field
		}

		_, err = translator.TransformResponse(ctx, backendResp, httpReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "choices")
	})

	t.Run("assistant_with_only_tool_calls", func(t *testing.T) {
		// Test assistant message with no text, only tool_use
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role: "assistant",
					Content: []interface{}{
						map[string]interface{}{
							"type": "tool_use",
							"id":   "toolu_only",
							"name": "search",
							"input": map[string]interface{}{
								"query": "test",
							},
						},
					},
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 1)

		// Content should be null when only tool calls present
		assert.Equal(t, "assistant", messages[0]["role"])
		assert.Nil(t, messages[0]["content"])
		assert.NotNil(t, messages[0]["tool_calls"])
	})

	t.Run("user_with_text_and_tool_result", func(t *testing.T) {
		// Test user message with both text and tool result
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
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
							"tool_use_id": "tool_123",
							"content":     "Result data",
						},
					},
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		// Should create separate messages: user text + tool result
		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 2)

		assert.Equal(t, "user", messages[0]["role"])
		assert.Equal(t, "Here's the result:", messages[0]["content"])

		assert.Equal(t, "tool", messages[1]["role"])
		assert.Equal(t, "tool_123", messages[1]["tool_call_id"])
		assert.Equal(t, "Result data", messages[1]["content"])
	})

	t.Run("tool_result_with_structured_content", func(t *testing.T) {
		// Test tool result with non-string content (should be serialised to JSON)
		anthropicReq := AnthropicRequest{
			Model:     ClaudeSonnetModel,
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{
					Role: "user",
					Content: []interface{}{
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "tool_struct",
							"content": map[string]interface{}{
								"temperature": 18,
								"conditions":  "cloudy",
							},
						},
					},
				},
			},
		}

		httpReq := createHTTPRequest(t, anthropicReq)
		transformed, err := translator.TransformRequest(ctx, httpReq)
		require.NoError(t, err)

		messages := transformed.OpenAIRequest["messages"].([]map[string]interface{})
		require.Len(t, messages, 1)

		assert.Equal(t, "tool", messages[0]["role"])

		// Content should be JSON string
		contentStr := messages[0]["content"].(string)
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(contentStr), &parsed)
		require.NoError(t, err)
		assert.Equal(t, float64(18), parsed["temperature"])
	})
}

// TestAnthropicModelPreservation tests that model names are preserved through the round-trip
func TestAnthropicModelPreservation(t *testing.T) {
	translator := NewTranslator(createIntegrationTestLogger(), createTestConfig())
	ctx := context.Background()

	models := []string{
		ClaudeSonnetModel,
		ClaudeOpusModel,
		ClaudeSonnetModel,
		ClaudeHaikuModel,
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			anthropicReq := AnthropicRequest{
				Model:     model,
				MaxTokens: 1024,
				Messages:  []AnthropicMessage{{Role: "user", Content: "Test"}},
			}

			httpReq := createHTTPRequest(t, anthropicReq)
			transformed, err := translator.TransformRequest(ctx, httpReq)
			require.NoError(t, err)

			// Verify model preserved in request
			assert.Equal(t, model, transformed.ModelName)
			assert.Equal(t, model, transformed.OpenAIRequest["model"])

			// Simulate response with same model
			backendResp := simulateBackendResponse("Response", nil, "stop")
			backendResp["model"] = model

			anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
			require.NoError(t, err)

			// Verify model preserved in response
			resp := anthropicResp.(AnthropicResponse)
			assert.Equal(t, model, resp.Model)
		})
	}
}

// TestAnthropicUsageTracking tests that token usage is correctly tracked
func TestAnthropicUsageTracking(t *testing.T) {
	translator := NewTranslator(createIntegrationTestLogger(), createTestConfig())
	ctx := context.Background()

	anthropicReq := AnthropicRequest{
		Model:     ClaudeSonnetModel,
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: "Count token usage"}},
	}

	httpReq := createHTTPRequest(t, anthropicReq)
	_, err := translator.TransformRequest(ctx, httpReq)
	require.NoError(t, err)

	// Test different usage values
	testCases := []struct {
		name             string
		promptTokens     float64
		completionTokens float64
	}{
		{"small_usage", 10.0, 5.0},
		{"medium_usage", 100.0, 50.0},
		{"large_usage", 1000.0, 500.0},
		{"zero_completion", 20.0, 0.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backendResp := map[string]interface{}{
				"id":    "chatcmpl-usage",
				"model": ClaudeSonnetModel,
				"choices": []interface{}{
					map[string]interface{}{
						"message":       map[string]interface{}{"role": "assistant", "content": "Test"},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     tc.promptTokens,
					"completion_tokens": tc.completionTokens,
					"total_tokens":      tc.promptTokens + tc.completionTokens,
				},
			}

			anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
			require.NoError(t, err)

			resp := anthropicResp.(AnthropicResponse)
			assert.Equal(t, int(tc.promptTokens), resp.Usage.InputTokens)
			assert.Equal(t, int(tc.completionTokens), resp.Usage.OutputTokens)
		})
	}

	t.Run("missing_usage", func(t *testing.T) {
		// Test response without usage field
		backendResp := map[string]interface{}{
			"id":    "chatcmpl-nousage",
			"model": ClaudeSonnetModel,
			"choices": []interface{}{
				map[string]interface{}{
					"message":       map[string]interface{}{"role": "assistant", "content": "Test"},
					"finish_reason": "stop",
				},
			},
			// Missing usage field
		}

		anthropicResp, err := translator.TransformResponse(ctx, backendResp, httpReq)
		require.NoError(t, err)

		resp := anthropicResp.(AnthropicResponse)
		assert.Equal(t, 0, resp.Usage.InputTokens)
		assert.Equal(t, 0, resp.Usage.OutputTokens)
	})
}
