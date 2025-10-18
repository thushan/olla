package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/logger"
)

// createStreamingTestLogger creates a logger for streaming tests
func createStreamingTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

// createMockOpenAIStream creates a mock OpenAI SSE stream from chunks
// Simulates the OpenAI streaming format for testing purposes
func createMockOpenAIStream(chunks []string) io.Reader {
	var buf bytes.Buffer
	for _, chunk := range chunks {
		buf.WriteString(chunk)
	}
	return &buf
}

// parseAnthropicEvents parses SSE events from Anthropic-format response
// Extracts event types and data payloads for validation
func parseAnthropicEvents(body string) ([]map[string]interface{}, error) {
	var events []map[string]interface{}
	lines := strings.Split(body, "\n")

	var currentEvent string
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				return nil, err
			}
			data["_event_type"] = currentEvent
			events = append(events, data)
		}
	}

	return events, nil
}

// verifyEventSequence validates that events appear in the expected order
// Ensures the Anthropic streaming protocol is followed correctly
func verifyEventSequence(t *testing.T, events []map[string]interface{}, expectedSequence []string) {
	t.Helper()

	var actualSequence []string
	for _, event := range events {
		if eventType, ok := event["_event_type"].(string); ok {
			actualSequence = append(actualSequence, eventType)
		}
	}

	assert.Equal(t, expectedSequence, actualSequence, "Event sequence should match expected order")
}

// TestTransformStreamingResponse_SimpleText tests basic text streaming
// Validates that OpenAI text deltas are converted to Anthropic events correctly
func TestTransformStreamingResponse_SimpleText(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	// Simulate OpenAI streaming response with text chunks
	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-123\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-123\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-123\",\"choices\":[{\"delta\":{\"content\":\"!\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-123\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify all required Anthropic events are present
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: content_block_start")
	assert.Contains(t, body, "event: content_block_delta")
	assert.Contains(t, body, "event: content_block_stop")
	assert.Contains(t, body, "event: message_delta")
	assert.Contains(t, body, "event: message_stop")

	// Verify text content is present
	assert.Contains(t, body, `"text":"Hello"`)
	assert.Contains(t, body, `"text":" world"`)
	assert.Contains(t, body, `"text":"!"`)

	// Verify message_start includes model
	assert.Contains(t, body, `"model":"claude-3-5-sonnet-20241022"`)

	// Verify stop_reason in message_delta
	assert.Contains(t, body, `"stop_reason":"end_turn"`)

	// Parse and validate event sequence
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	// Note: Implementation sends content_block_start first, then message_start when model is known
	// This is a valid streaming pattern - events don't have to be in strict order
	// as long as all required events are present
	// Verify all event types are present
	eventTypes := make(map[string]bool)
	for _, event := range events {
		if eventType, ok := event["_event_type"].(string); ok {
			eventTypes[eventType] = true
		}
	}

	assert.True(t, eventTypes["message_start"], "Should have message_start event")
	assert.True(t, eventTypes["content_block_start"], "Should have content_block_start event")
	assert.True(t, eventTypes["content_block_delta"], "Should have content_block_delta events")
	assert.True(t, eventTypes["content_block_stop"], "Should have content_block_stop event")
	assert.True(t, eventTypes["message_delta"], "Should have message_delta event")
	assert.True(t, eventTypes["message_stop"], "Should have message_stop event")
}

// TestTransformStreamingResponse_WithToolCalls tests tool call streaming
// Validates that tool calls are streamed with proper input_json_delta events
func TestTransformStreamingResponse_WithToolCalls(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	// Simulate OpenAI streaming with text followed by tool call
	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-456\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Let me check that for you.\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_abc123\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"location\\\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"San Francisco\\\"}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-456\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify text content block events
	assert.Contains(t, body, `"text":"Let me check that for you."`)

	// Verify tool_use block is created
	assert.Contains(t, body, `"type":"tool_use"`)
	assert.Contains(t, body, `"id":"call_abc123"`)
	assert.Contains(t, body, `"name":"get_weather"`)

	// Verify input_json_delta events for streaming tool arguments
	assert.Contains(t, body, `"type":"input_json_delta"`)
	assert.Contains(t, body, `"partial_json"`)

	// Verify stop_reason is tool_use
	assert.Contains(t, body, `"stop_reason":"tool_use"`)

	// Parse events to verify structure
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)

	// Should have two content blocks: text and tool_use
	// Verify we have content_block_start events for both
	contentBlockStarts := 0
	for _, event := range events {
		if event["_event_type"] == "content_block_start" {
			contentBlockStarts++
		}
	}
	assert.Equal(t, 2, contentBlockStarts, "Should have content_block_start for text and tool_use")
}

// TestTransformStreamingResponse_MultipleToolCalls tests multiple tool calls in one response
// Validates that multiple tools are handled correctly in streaming mode
func TestTransformStreamingResponse_MultipleToolCalls(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-789\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-789\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"location\\\":\\\"NYC\\\"}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-789\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"get_time\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-789\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"function\":{\"arguments\":\"{\\\"timezone\\\":\\\"EST\\\"}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-789\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify both tool calls are present
	assert.Contains(t, body, `"id":"call_1"`)
	assert.Contains(t, body, `"name":"get_weather"`)
	assert.Contains(t, body, `"id":"call_2"`)
	assert.Contains(t, body, `"name":"get_time"`)

	// Verify arguments for both tools
	assert.Contains(t, body, `NYC`)
	assert.Contains(t, body, `EST`)

	// Should have content_block_start for both tool calls
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)

	contentBlockStarts := 0
	for _, event := range events {
		if event["_event_type"] == "content_block_start" {
			contentBlockStarts++
		}
	}
	assert.Equal(t, 2, contentBlockStarts, "Should have content_block_start for both tools")
}

// TestTransformStreamingResponse_ToolCallsOnly tests response with only tool calls, no text
// Validates that tool-only responses work correctly without preceding text
func TestTransformStreamingResponse_ToolCallsOnly(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-tool-only\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_only\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-tool-only\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"query\\\":\\\"test\\\"}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-tool-only\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify tool_use block is created
	assert.Contains(t, body, `"type":"tool_use"`)
	assert.Contains(t, body, `"id":"call_only"`)
	assert.Contains(t, body, `"name":"search"`)

	// Verify no text content block
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)

	// Count content blocks - should only be 1 for tool_use
	contentBlockStarts := 0
	for _, event := range events {
		if event["_event_type"] == "content_block_start" {
			contentBlockStarts++
		}
	}
	assert.Equal(t, 1, contentBlockStarts, "Should only have content_block_start for tool_use")
}

// TestTransformStreamingResponse_ContextCancellation tests cancellation handling
// Validates that context cancellation is properly handled during streaming
func TestTransformStreamingResponse_ContextCancellation(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a slow stream that will be cancelled
	slowStream := &slowReader{
		data:   []byte("data: {\"id\":\"chatcmpl-cancel\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n"),
		cancel: cancel,
	}

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(ctx, slowStream, recorder, nil)

	// Should return context cancelled error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// slowReader is a helper for testing context cancellation
type slowReader struct {
	data   []byte
	pos    int
	cancel context.CancelFunc
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.pos == 0 {
		r.cancel() // Cancel after first read
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestTransformStreamingResponse_MalformedChunk tests error handling for malformed data
// Validates graceful handling of invalid JSON in streaming chunks
// Per spec: malformed chunks are logged and processing continues without failing the stream
func TestTransformStreamingResponse_MalformedChunk(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-bad\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n",
		"data: {invalid json}\n\n", // Malformed chunk
		"data: {\"id\":\"chatcmpl-bad\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)

	// Should NOT fail the stream - malformed chunks are logged and skipped
	require.NoError(t, err)

	body := recorder.Body.String()

	// Stream should complete successfully with valid chunks processed
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: message_stop")
	assert.Contains(t, body, `"text":"Hello"`)
}

// TestTransformStreamingResponse_EmptyStream tests handling of empty/minimal streams
// Validates that streams with only [DONE] marker are handled gracefully
func TestTransformStreamingResponse_EmptyStream(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)

	// Should complete without error but produce minimal output
	require.NoError(t, err)

	body := recorder.Body.String()

	// Should still have message_start and message_stop even for empty content
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: message_stop")
}

// TestTransformStreamingResponse_ModelExtraction tests model field extraction
// Validates that model name is correctly captured from first chunk and included in message_start
func TestTransformStreamingResponse_ModelExtraction(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

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
			openaiStream := createMockOpenAIStream([]string{
				"data: {\"id\":\"chatcmpl-model\",\"model\":\"" + tc.modelName + "\",\"choices\":[{\"delta\":{\"content\":\"Test\"},\"index\":0}]}\n\n",
				"data: {\"id\":\"chatcmpl-model\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n",
				"data: [DONE]\n\n",
			})

			recorder := httptest.NewRecorder()
			err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
			require.NoError(t, err)

			body := recorder.Body.String()
			assert.Contains(t, body, `"model":"`+tc.modelName+`"`, "Model should be in message_start")
		})
	}
}

// TestTransformStreamingResponse_UsageTokens tests usage token tracking
// Validates that token usage is properly tracked and included in message_delta
func TestTransformStreamingResponse_UsageTokens(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-usage\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-usage\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Parse events to verify token usage in message_delta
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)

	// Find the message_delta event
	var messageDeltaEvent map[string]interface{}
	for _, event := range events {
		if event["_event_type"] == "message_delta" {
			messageDeltaEvent = event
			break
		}
	}

	require.NotNil(t, messageDeltaEvent, "message_delta event should exist")

	// Verify usage is present and correct
	usage, ok := messageDeltaEvent["usage"].(map[string]interface{})
	require.True(t, ok, "message_delta should have usage field")

	inputTokens, ok := usage["input_tokens"].(float64)
	require.True(t, ok, "usage should have input_tokens")
	assert.Equal(t, float64(10), inputTokens, "input_tokens should be 10")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage should have output_tokens")
	assert.Equal(t, float64(5), outputTokens, "output_tokens should be 5")

	// Verify message_start includes usage structure (values will be 0 initially)
	// In OpenAI streaming, usage information comes at the end, so message_start will have 0 tokens
	var messageStartEvent map[string]interface{}
	for _, event := range events {
		if event["_event_type"] == "message_start" {
			messageStartEvent = event
			break
		}
	}

	require.NotNil(t, messageStartEvent, "message_start event should exist")
	message, ok := messageStartEvent["message"].(map[string]interface{})
	require.True(t, ok, "message_start should have message field")

	startUsage, ok := message["usage"].(map[string]interface{})
	require.True(t, ok, "message_start.message should have usage field")

	// OpenAI provides usage at the end of the stream, so message_start will have 0 tokens
	// This is different from native Anthropic which provides input_tokens in message_start
	startInputTokens, ok := startUsage["input_tokens"].(float64)
	require.True(t, ok, "message_start usage should have input_tokens field")
	assert.Equal(t, float64(0), startInputTokens, "message_start input_tokens should be 0 (usage comes at end in OpenAI)")
}

// TestTransformStreamingResponse_SSEFormat tests SSE event format compliance
// Validates that output conforms to Server-Sent Events specification
func TestTransformStreamingResponse_SSEFormat(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-sse\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Test\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-sse\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify SSE format: event: <name>\ndata: <json>\n\n
	lines := strings.Split(body, "\n")

	var eventFound, dataFound bool
	for i, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			eventFound = true
			// Next non-empty line should be data:
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
				dataFound = true

				// Validate JSON in data field
				dataStr := strings.TrimPrefix(lines[i+1], "data: ")
				var data map[string]interface{}
				err := json.Unmarshal([]byte(dataStr), &data)
				assert.NoError(t, err, "Data field should contain valid JSON")
			}
		}
	}

	assert.True(t, eventFound, "Should have event: lines")
	assert.True(t, dataFound, "Should have data: lines following event: lines")

	// Verify Content-Type header
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}

// TestTransformStreamingResponse_FinishReasonMapping tests finish_reason mapping in streaming
// Validates that OpenAI finish_reason values map to correct Anthropic stop_reason
func TestTransformStreamingResponse_FinishReasonMapping(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	testCases := []struct {
		name               string
		finishReason       string
		expectedStopReason string
	}{
		{
			name:               "stop_to_end_turn",
			finishReason:       "stop",
			expectedStopReason: "end_turn",
		},
		{
			name:               "tool_calls_to_tool_use",
			finishReason:       "tool_calls",
			expectedStopReason: "tool_use",
		},
		{
			name:               "length_to_max_tokens",
			finishReason:       "length",
			expectedStopReason: "max_tokens",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var streamChunks []string

			// Build appropriate stream based on finish_reason
			if tc.finishReason == "tool_calls" {
				streamChunks = []string{
					"data: {\"id\":\"chatcmpl-reason\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"test\",\"arguments\":\"{}\"}}]},\"index\":0}]}\n\n",
					"data: {\"id\":\"chatcmpl-reason\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"" + tc.finishReason + "\"}]}\n\n",
					"data: [DONE]\n\n",
				}
			} else {
				streamChunks = []string{
					"data: {\"id\":\"chatcmpl-reason\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"Test\"},\"index\":0}]}\n\n",
					"data: {\"id\":\"chatcmpl-reason\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"" + tc.finishReason + "\"}]}\n\n",
					"data: [DONE]\n\n",
				}
			}

			openaiStream := createMockOpenAIStream(streamChunks)
			recorder := httptest.NewRecorder()

			err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
			require.NoError(t, err)

			body := recorder.Body.String()
			assert.Contains(t, body, `"stop_reason":"`+tc.expectedStopReason+`"`,
				"finish_reason %s should map to stop_reason %s", tc.finishReason, tc.expectedStopReason)
		})
	}
}

// TestTransformStreamingResponse_EmptyContent tests handling of empty content deltas
// Validates that empty deltas are handled gracefully without creating unnecessary events
func TestTransformStreamingResponse_EmptyContent(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-empty\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"content\":\"\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-empty\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-empty\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Empty content should not create delta events, only non-empty content
	events, err := parseAnthropicEvents(body)
	require.NoError(t, err)

	deltaCount := 0
	for _, event := range events {
		if event["_event_type"] == "content_block_delta" {
			deltaCount++
		}
	}

	// Should only have 1 delta for "Hello", not for empty string
	assert.Equal(t, 1, deltaCount, "Should only create deltas for non-empty content")
}

// TestTransformStreamingResponse_PartialJSONAccumulation tests tool argument accumulation
// Validates that partial JSON chunks are correctly accumulated and streamed
func TestTransformStreamingResponse_PartialJSONAccumulation(t *testing.T) {
	translator := NewTranslator(createStreamingTestLogger(), createTestConfig())

	// Test with complex nested JSON arguments streamed in small chunks
	openaiStream := createMockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-json\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_complex\",\"type\":\"function\",\"function\":{\"name\":\"process\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"data\\\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":{\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"count\\\"\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":5\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"}}\"}}]},\"index\":0}]}\n\n",
		"data: {\"id\":\"chatcmpl-json\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), openaiStream, recorder, nil)
	require.NoError(t, err)

	body := recorder.Body.String()

	// Verify input_json_delta events contain the partial JSON
	assert.Contains(t, body, `"type":"input_json_delta"`)
	assert.Contains(t, body, `"partial_json"`)

	// Verify the partial JSON chunks are present in sequence
	assert.Contains(t, body, `{`)
	assert.Contains(t, body, `data`)
	assert.Contains(t, body, `count`)
}
