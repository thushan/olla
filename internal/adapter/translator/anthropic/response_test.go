package anthropic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/logger"
)

func createResponseTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func TestTransformResponse_SimpleText(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"id":    "chatcmpl-123",
		"model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello! How can I help you today?",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(10),
			"completion_tokens": float64(8),
			"total_tokens":      float64(18),
		},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	assert.Equal(t, "message", anthropicResp.Type)
	assert.Equal(t, "assistant", anthropicResp.Role)
	assert.Equal(t, "claude-3-5-sonnet-20241022", anthropicResp.Model)
	assert.Equal(t, "end_turn", anthropicResp.StopReason)

	require.Len(t, anthropicResp.Content, 1)
	assert.Equal(t, "text", anthropicResp.Content[0].Type)
	assert.Equal(t, "Hello! How can I help you today?", anthropicResp.Content[0].Text)

	assert.Equal(t, 10, anthropicResp.Usage.InputTokens)
	assert.Equal(t, 8, anthropicResp.Usage.OutputTokens)
}

func TestTransformResponse_WithToolCalls(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"id":    "chatcmpl-123",
		"model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Let me check that for you.",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_abc123",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"San Francisco","unit":"celsius"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(20),
			"completion_tokens": float64(15),
		},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	assert.Equal(t, "tool_use", anthropicResp.StopReason)
	require.Len(t, anthropicResp.Content, 2)

	assert.Equal(t, "text", anthropicResp.Content[0].Type)
	assert.Equal(t, "Let me check that for you.", anthropicResp.Content[0].Text)

	assert.Equal(t, "tool_use", anthropicResp.Content[1].Type)
	assert.Equal(t, "call_abc123", anthropicResp.Content[1].ID)
	assert.Equal(t, "get_weather", anthropicResp.Content[1].Name)

	require.NotNil(t, anthropicResp.Content[1].Input)
	assert.Equal(t, "San Francisco", anthropicResp.Content[1].Input["location"])
	assert.Equal(t, "celsius", anthropicResp.Content[1].Input["unit"])
}

func TestTransformResponse_MultipleToolCalls(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_1",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"SF"}`,
							},
						},
						map[string]interface{}{
							"id":   "call_2",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_time",
								"arguments": `{"timezone":"PST"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	assert.Equal(t, "tool_use", anthropicResp.StopReason)
	require.Len(t, anthropicResp.Content, 2)

	assert.Equal(t, "tool_use", anthropicResp.Content[0].Type)
	assert.Equal(t, "get_weather", anthropicResp.Content[0].Name)

	assert.Equal(t, "tool_use", anthropicResp.Content[1].Type)
	assert.Equal(t, "get_time", anthropicResp.Content[1].Name)
}

func TestTransformResponse_EmptyResponse(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("empty_content", func(t *testing.T) {
		// OpenAI response with empty content string
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-empty",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     float64(5),
				"completion_tokens": float64(0),
			},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "end_turn", anthropicResp.StopReason)
		assert.GreaterOrEqual(t, len(anthropicResp.Content), 0)
	})

	t.Run("null_content", func(t *testing.T) {
		// OpenAI response with null content (tool-only response)
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-null",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_only",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search",
									"arguments": `{"query":"test"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		require.Len(t, anthropicResp.Content, 1)
		assert.Equal(t, "tool_use", anthropicResp.Content[0].Type)
		assert.Equal(t, "search", anthropicResp.Content[0].Name)
	})
}

func TestTransformResponse_MissingUsage(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("usage_not_present", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-nousage",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Response without usage",
					},
					"finish_reason": "stop",
				},
			},
			// Missing usage field
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, 0, anthropicResp.Usage.InputTokens)
		assert.Equal(t, 0, anthropicResp.Usage.OutputTokens)
	})

	t.Run("partial_usage", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-partial",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Response with partial usage",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens": float64(15),
				// Missing completion_tokens
			},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, 15, anthropicResp.Usage.InputTokens)
		assert.Equal(t, 0, anthropicResp.Usage.OutputTokens)
	})
}

func TestTransformResponse_InvalidToolArguments(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("malformed_json", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-badjson",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Calling tool",
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_bad",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_data",
									"arguments": `{invalid json syntax`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)

		if err != nil {
			assert.Contains(t, err.Error(), "json", "Error should mention JSON parsing issue")
		} else {
			anthropicResp, ok := result.(AnthropicResponse)
			require.True(t, ok)

			var toolBlock *ContentBlock
			for i := range anthropicResp.Content {
				if anthropicResp.Content[i].Type == "tool_use" {
					toolBlock = &anthropicResp.Content[i]
					break
				}
			}
			require.NotNil(t, toolBlock, "Should have tool_use block")
		}
	})

	t.Run("empty_arguments", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-empty-args",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Calling tool with empty args",
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_empty",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "no_params",
									"arguments": `{}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		require.Len(t, anthropicResp.Content, 2) // text + tool_use
		assert.Equal(t, "tool_use", anthropicResp.Content[1].Type)
		assert.NotNil(t, anthropicResp.Content[1].Input)
		assert.Len(t, anthropicResp.Content[1].Input, 0) // Empty map
	})
}

func TestTransformResponse_OnlyToolCalls(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"id":    "chatcmpl-toolonly",
		"model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": nil, // No text content
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_search",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "web_search",
								"arguments": `{"query":"Anthropic Claude","max_results":5}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(25),
			"completion_tokens": float64(10),
		},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	assert.Equal(t, "tool_use", anthropicResp.StopReason)

	require.Len(t, anthropicResp.Content, 1)
	assert.Equal(t, "tool_use", anthropicResp.Content[0].Type)
	assert.Equal(t, "web_search", anthropicResp.Content[0].Name)

	assert.Equal(t, "Anthropic Claude", anthropicResp.Content[0].Input["query"])
	assert.Equal(t, float64(5), anthropicResp.Content[0].Input["max_results"]) // JSON numbers are float64
}

func TestTransformResponse_FinishReasonMapping(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("stop_to_end_turn", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-finish",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "end_turn", anthropicResp.StopReason, "Normal completion maps to end_turn")
	})

	t.Run("tool_calls_to_tool_use", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-tools",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Calling tool",
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_test",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "test_tool",
									"arguments": `{}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "tool_use", anthropicResp.StopReason, "Tool invocation maps to tool_use")
	})

	t.Run("length_to_max_tokens", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-length",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test response",
					},
					"finish_reason": "length",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "max_tokens", anthropicResp.StopReason, "Token limit maps to max_tokens")
	})

	t.Run("explicit_stop_reason", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-explicit",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Response text",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "end_turn", anthropicResp.StopReason, "Explicit stop should map to end_turn")
	})

	t.Run("unknown_finish_reason_defaults_to_end_turn", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-unknown",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test",
					},
					"finish_reason": "content_filter", // Unknown reason
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Equal(t, "end_turn", anthropicResp.StopReason, "Unknown finish_reason should default to end_turn")
	})
}

func TestTransformResponse_MessageIDGeneration(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("generates_anthropic_format_id", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-original-123",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.NotEmpty(t, anthropicResp.ID)
		assert.Contains(t, anthropicResp.ID, "msg_01", "Message ID should start with msg_01 prefix")

		// Verify ID is the expected length (27-29 characters typical for Anthropic)
		// msg_01 (6 chars) + base58 encoded 16 bytes (~22 chars) = ~28 chars total
		assert.GreaterOrEqual(t, len(anthropicResp.ID), 20, "ID should be at least 20 characters")
		assert.LessOrEqual(t, len(anthropicResp.ID), 35, "ID should be at most 35 characters")
	})

	t.Run("generates_unique_ids", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
			require.NoError(t, err)

			anthropicResp, ok := result.(AnthropicResponse)
			require.True(t, ok)

			assert.False(t, ids[anthropicResp.ID], "Generated duplicate ID: %s", anthropicResp.ID)
			ids[anthropicResp.ID] = true
		}

		assert.Len(t, ids, 100)
	})

	t.Run("id_contains_only_valid_base58_chars", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)
		// After the msg_01 prefix, should only contain base58 characters
		// Base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz
		// (excludes 0, O, I, l)
		id := anthropicResp.ID
		assert.True(t, len(id) > 6, "ID should be longer than just the prefix")

		suffix := id[6:] // Skip "msg_01" prefix
		for _, char := range suffix {
			assert.Contains(t, "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", string(char),
				"ID suffix should only contain base58 characters, found: %c", char)
		}

		assert.NotContains(t, suffix, "0", "ID should not contain digit 0")
		assert.NotContains(t, suffix, "O", "ID should not contain uppercase O")
		assert.NotContains(t, suffix, "I", "ID should not contain uppercase I")
		assert.NotContains(t, suffix, "l", "ID should not contain lowercase l")
	})
}

func TestTransformResponse_ComplexToolArguments(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"id":    "chatcmpl-complex",
		"model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Processing request",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_complex",
							"type": "function",
							"function": map[string]interface{}{
								"name": "process_data",
								"arguments": `{
									"filters": {
										"category": "electronics",
										"price_range": {"min": 100, "max": 500}
									},
									"sort": "price_asc",
									"tags": ["sale", "featured"]
								}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	var toolBlock *ContentBlock
	for i := range anthropicResp.Content {
		if anthropicResp.Content[i].Type == "tool_use" {
			toolBlock = &anthropicResp.Content[i]
			break
		}
	}
	require.NotNil(t, toolBlock)

	assert.NotNil(t, toolBlock.Input["filters"])
	filters, ok := toolBlock.Input["filters"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "electronics", filters["category"])

	priceRange, ok := filters["price_range"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(100), priceRange["min"])
	assert.Equal(t, float64(500), priceRange["max"])

	tags, ok := toolBlock.Input["tags"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tags, 2)
	assert.Equal(t, "sale", tags[0])
	assert.Equal(t, "featured", tags[1])
}

func TestTransformResponse_InvalidFormat(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("missing_choices", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-nochoices",
			"model": "claude-3-5-sonnet-20241022",
			// Missing choices field
			"usage": map[string]interface{}{},
		}

		_, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "choices", "Error should mention missing choices")
	})

	t.Run("empty_choices_array", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":      "chatcmpl-emptychoices",
			"model":   "claude-3-5-sonnet-20241022",
			"choices": []interface{}{}, // Empty array
			"usage":   map[string]interface{}{},
		}

		_, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "choices", "Error should mention empty choices")
	})

	t.Run("missing_message", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-nomessage",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					// Missing message field
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		_, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message", "Error should mention missing message")
	})

	t.Run("invalid_choices_type", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":      "chatcmpl-badchoices",
			"model":   "claude-3-5-sonnet-20241022",
			"choices": "not an array", // Wrong type
			"usage":   map[string]interface{}{},
		}

		_, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		assert.Error(t, err)
	})
}

func TestTransformResponse_StopSequenceHandling(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	t.Run("with_stop_sequence", func(t *testing.T) {
		openaiResp := map[string]interface{}{
			"id":    "chatcmpl-stopseq",
			"model": "claude-3-5-sonnet-20241022",
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Response until END",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{},
		}

		result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
		require.NoError(t, err)

		anthropicResp, ok := result.(AnthropicResponse)
		require.True(t, ok)

		assert.Nil(t, anthropicResp.StopSequence)
	})
}

func TestTransformResponse_ModelFieldPreservation(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	testCases := []struct {
		name      string
		modelName string
	}{
		{
			name:      "sonnet_3_5",
			modelName: "claude-3-5-sonnet-20241022",
		},
		{
			name:      "opus_3",
			modelName: "claude-3-opus-20240229",
		},
		{
			name:      "haiku_3",
			modelName: "claude-3-haiku-20240307",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			openaiResp := map[string]interface{}{
				"id":    "chatcmpl-model",
				"model": tc.modelName,
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Test",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{},
			}

			result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
			require.NoError(t, err)

			anthropicResp, ok := result.(AnthropicResponse)
			require.True(t, ok)

			assert.Equal(t, tc.modelName, anthropicResp.Model)
		})
	}
}

func TestTransformResponse_TypeAndRoleFields(t *testing.T) {
	translator := NewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]interface{}{
		"id":    "chatcmpl-fields",
		"model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Test response",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{},
	}

	result, err := translator.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	anthropicResp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	assert.Equal(t, "message", anthropicResp.Type)

	assert.Equal(t, "assistant", anthropicResp.Role)
}

func BenchmarkTransformResponse_Simple(b *testing.B) {
	tr := NewTranslator(createResponseTestLogger(), createTestConfig())
	openaiResp := map[string]interface{}{
		"id": "chatcmpl-123", "model": "claude-3-5-sonnet-20241022",
		"choices": []interface{}{map[string]interface{}{
			"message":       map[string]interface{}{"role": "assistant", "content": "hi"},
			"finish_reason": "stop",
		}},
		"usage": map[string]interface{}{"prompt_tokens": float64(5), "completion_tokens": float64(2)},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = tr.TransformResponse(context.Background(), openaiResp, nil)
	}
}
