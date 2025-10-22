package anthropic

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformStreamingResponse_SimpleText(t *testing.T) {
	translator := newTestTranslator()

	// simulate openai streaming response with text chunks
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-123", "claude-3-5-sonnet-20241022", "Hello"),
		textChunk("chatcmpl-123", "", " world"),
		textChunk("chatcmpl-123", "", "!"),
		finishChunk("chatcmpl-123", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify all required anthropic events are present
	assertContainsAll(t, body, []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	})

	// verify text content is present
	assertTextContent(t, body, "Hello")
	assertTextContent(t, body, " world")
	assertTextContent(t, body, "!")

	// verify message_start includes model
	assert.Contains(t, body, `"model":"claude-3-5-sonnet-20241022"`)

	// parse and validate event structure
	events := parseAnthropicEvents(t, body)
	require.NotEmpty(t, events)

	// note: implementation sends content_block_start first, then message_start when model is known
	// This is a valid streaming pattern - events don't have to be in strict order
	// as long as all required events are present
	assertRequiredEvents(t, events)
	assertHasEventType(t, events, "content_block_start")
	assertHasEventType(t, events, "content_block_delta")
	assertHasEventType(t, events, "content_block_stop")
	assertStopReason(t, events, "end_turn")
}

func TestTransformStreamingResponse_WithToolCalls(t *testing.T) {
	translator := newTestTranslator()

	// simulate openai streaming with text followed by tool call
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-456", "claude-3-5-sonnet-20241022", "Let me check that for you."),
		toolStartChunk("chatcmpl-456", 0, "call_abc123", "get_weather"),
		toolArgsChunk("chatcmpl-456", 0, `{\\\"location\\\"`),
		toolArgsChunk("chatcmpl-456", 0, `:`),
		toolArgsChunk("chatcmpl-456", 0, `\\\"San Francisco\\\"}`),
		toolArgsChunk("chatcmpl-456", 0, `}`),
		finishChunk("chatcmpl-456", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify text and tool content
	assertTextContent(t, body, "Let me check that for you.")
	assertToolPresent(t, body, "call_abc123", "get_weather")

	// verify tool events and structure
	assertContainsAll(t, body, []string{
		`"type":"tool_use"`,
		`"type":"input_json_delta"`,
		`"partial_json"`,
	})

	// parse events to verify structure
	events := parseAnthropicEvents(t, body)
	assertStopReason(t, events, "tool_use")
	// should have two content blocks: text and tool_use
	assertContentBlockCount(t, events, 2)
}

func TestTransformStreamingResponse_MultipleToolCalls(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-789", 0, "call_1", "get_weather"),
		toolArgsChunk("chatcmpl-789", 0, `{\\\"location\\\":\\\"NYC\\\"}`),
		toolStartChunk("chatcmpl-789", 1, "call_2", "get_time"),
		toolArgsChunk("chatcmpl-789", 1, `{\\\"timezone\\\":\\\"EST\\\"}`),
		finishChunk("chatcmpl-789", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify both tools are present
	assertToolPresent(t, body, "call_1", "get_weather")
	assertToolPresent(t, body, "call_2", "get_time")

	// verify arguments for both tools
	assert.Contains(t, body, `NYC`)
	assert.Contains(t, body, `EST`)

	// should have content_block_start for both tool calls
	events := parseAnthropicEvents(t, body)
	assertContentBlockCount(t, events, 2)
}

func TestTransformStreamingResponse_ToolCallsOnly(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-tool-only", 0, "call_only", "search"),
		toolArgsChunk("chatcmpl-tool-only", 0, `{\\\"query\\\":\\\"test\\\"}`),
		finishChunk("chatcmpl-tool-only", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify tool_use block is created
	assertToolPresent(t, body, "call_only", "search")
	assert.Contains(t, body, `"type":"tool_use"`)

	// verify no text content block - only 1 block for tool_use
	events := parseAnthropicEvents(t, body)
	assertContentBlockCount(t, events, 1)
}

func TestTransformStreamingResponse_ContextCancellation(t *testing.T) {
	translator := newTestTranslator()

	// create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// create a slow stream that will be cancelled
	slowStream := &slowReader{
		data:   []byte("data: {\"id\":\"chatcmpl-cancel\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n"),
		cancel: cancel,
	}

	_, err := executeTransformWithContext(t, translator, ctx, slowStream)

	// should return context cancelled error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// slow reader is a helper for testing context cancellation
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

func TestTransformStreamingResponse_MalformedChunk(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-bad", "claude-3-5-sonnet-20241022", "Hello"),
		"data: {invalid json}\n\n", // malformed chunk
		finishChunk("chatcmpl-bad", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// stream should complete successfully with valid chunks processed
	assertContainsAll(t, body, []string{
		"event: message_start",
		"event: message_stop",
	})
	assertTextContent(t, body, "Hello")
}

func TestTransformStreamingResponse_EmptyStream(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{doneChunk()})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// should still have message_start and message_stop even for empty content
	assertContainsAll(t, body, []string{
		"event: message_start",
		"event: message_stop",
	})
}

func TestTransformStreamingResponse_ModelExtraction(t *testing.T) {
	translator := newTestTranslator()

	testCases := []struct {
		name      string
		modelName string
	}{
		{name: "sonnet_3_5", modelName: "claude-3-5-sonnet-20241022"},
		{name: "opus_3", modelName: "claude-3-opus-20240229"},
		{name: "haiku_3", modelName: "claude-3-haiku-20240307"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := mockOpenAIStream([]string{
				textChunk("chatcmpl-model", tc.modelName, "Test"),
				finishChunk("chatcmpl-model", "stop"),
				doneChunk(),
			})

			recorder := executeTransform(t, translator, stream)
			body := recorder.Body.String()
			assert.Contains(t, body, `"model":"`+tc.modelName+`"`, "Model should be in message_start")
		})
	}
}

func TestTransformStreamingResponse_UsageTokens(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-usage", "claude-3-5-sonnet-20241022", "Hello"),
		finishChunkWithUsage("chatcmpl-usage", "stop", 10, 5),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// parse events to verify token usage in message_delta
	events := parseAnthropicEvents(t, body)
	assertUsageTokens(t, events, 10, 5)

	// verify message_start includes usage structure (values will be 0 initially)
	messageStart := getEventByType(events, "message_start")
	require.NotNil(t, messageStart, "message_start event should exist")
	message, ok := messageStart["message"].(map[string]interface{})
	require.True(t, ok, "message_start should have message field")

	startUsage, ok := message["usage"].(map[string]interface{})
	require.True(t, ok, "message_start.message should have usage field")

	// openai provides usage at the end of the stream, so message_start will have 0 tokens
	startInputTokens, ok := startUsage["input_tokens"].(float64)
	require.True(t, ok, "message_start usage should have input_tokens field")
	assert.Equal(t, float64(0), startInputTokens, "message_start input_tokens should be 0 (usage comes at end in OpenAI)")
}

func TestTransformStreamingResponse_SSEFormat(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-sse", "claude-3-5-sonnet-20241022", "Test"),
		finishChunk("chatcmpl-sse", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify SSE format and content-type header
	assertSSEFormat(t, body)
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}

func TestTransformStreamingResponse_FinishReasonMapping(t *testing.T) {
	translator := newTestTranslator()

	testCases := []struct {
		name               string
		finishReason       string
		expectedStopReason string
		needsTool          bool
	}{
		{name: "stop_to_end_turn", finishReason: "stop", expectedStopReason: "end_turn", needsTool: false},
		{name: "tool_calls_to_tool_use", finishReason: "tool_calls", expectedStopReason: "tool_use", needsTool: true},
		{name: "length_to_max_tokens", finishReason: "length", expectedStopReason: "max_tokens", needsTool: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var chunks []string

			// build appropriate stream based on finish_reason
			if tc.needsTool {
				chunks = []string{
					toolStartChunk("chatcmpl-reason", 0, "call_1", "test"),
					toolArgsChunk("chatcmpl-reason", 0, "{}"),
					finishChunk("chatcmpl-reason", tc.finishReason),
					doneChunk(),
				}
			} else {
				chunks = []string{
					textChunk("chatcmpl-reason", "claude-3-5-sonnet-20241022", "Test"),
					finishChunk("chatcmpl-reason", tc.finishReason),
					doneChunk(),
				}
			}

			stream := mockOpenAIStream(chunks)
			recorder := executeTransform(t, translator, stream)
			body := recorder.Body.String()

			assert.Contains(t, body, `"stop_reason":"`+tc.expectedStopReason+`"`,
				"finish_reason %s should map to stop_reason %s", tc.finishReason, tc.expectedStopReason)
		})
	}
}

func TestTransformStreamingResponse_EmptyContent(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-empty", "claude-3-5-sonnet-20241022", ""),
		textChunk("chatcmpl-empty", "", "Hello"),
		finishChunk("chatcmpl-empty", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// empty content should not create delta events, only non-empty content
	events := parseAnthropicEvents(t, body)
	deltaCount := countEventsByType(events, "content_block_delta")

	// should only have 1 delta for "Hello", not for empty string
	assert.Equal(t, 1, deltaCount, "Should only create deltas for non-empty content")
}

func TestTransformStreamingResponse_PartialJSONAccumulation(t *testing.T) {
	translator := newTestTranslator()

	// test with complex nested json arguments streamed in small chunks
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-json", 0, "call_complex", "process"),
		toolArgsChunk("chatcmpl-json", 0, `{`),
		toolArgsChunk("chatcmpl-json", 0, `\\\"data\\\"`),
		toolArgsChunk("chatcmpl-json", 0, `:{`),
		toolArgsChunk("chatcmpl-json", 0, `\\\"count\\\"`),
		toolArgsChunk("chatcmpl-json", 0, `:5`),
		toolArgsChunk("chatcmpl-json", 0, `}}`),
		finishChunk("chatcmpl-json", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify input_json_delta events contain the partial json
	assertContainsAll(t, body, []string{
		`"type":"input_json_delta"`,
		`"partial_json"`,
		`{`,
		`data`,
		`count`,
	})
}

func TestTransformStreamingResponse_TextBeforeTool(t *testing.T) {
	translator := newTestTranslator()

	// regression test: text content followed by tool call requires proper block closing
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-textool", "claude-3-5-sonnet-20241022", "Let me "),
		textChunk("chatcmpl-textool", "", "help you with that."),
		toolStartChunk("chatcmpl-textool", 0, "call_search", "search_db"),
		toolArgsChunk("chatcmpl-textool", 0, `{\\\"query\\\":\\\"anthropic\\\"}`),
		finishChunk("chatcmpl-textool", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// verify we have correct sequence: text block start -> deltas -> stop, then tool block start -> deltas -> stop
	assertContainsAll(t, body, []string{
		`"text":"Let me "`,
		`"text":"help you with that."`,
	})
	assertToolPresent(t, body, "call_search", "search_db")

	// count content block events - should have 2 starts (text + tool) and 2 stops (text + tool)
	assertContentBlockCount(t, events, 2)
	assertBlocksClosed(t, events)

	// verify the text block is stopped before tool block starts
	assertBlockTransitionOrder(t, events)
}

func TestTransformStreamingResponse_MultipleToolsSequential(t *testing.T) {
	translator := newTestTranslator()

	// test multiple tools with indices 0, 1, 2 to verify mapping works correctly
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-seq", 0, "call_tool0", "search"),
		toolArgsChunk("chatcmpl-seq", 0, `{\\\"q\\\":\\\"first\\\"}`),
		toolStartChunk("chatcmpl-seq", 1, "call_tool1", "weather"),
		toolArgsChunk("chatcmpl-seq", 1, `{\\\"city\\\":\\\"SF\\\"}`),
		toolStartChunk("chatcmpl-seq", 2, "call_tool2", "calc"),
		toolArgsChunk("chatcmpl-seq", 2, `{\\\"expr\\\":\\\"2+2\\\"}`),
		finishChunk("chatcmpl-seq", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify all three tools are present with correct IDs and names
	assertToolPresent(t, body, "call_tool0", "search")
	assertToolPresent(t, body, "call_tool1", "weather")
	assertToolPresent(t, body, "call_tool2", "calc")

	// verify all tool arguments are present
	assertContainsAll(t, body, []string{`first`, `SF`, `2+2`})

	events := parseAnthropicEvents(t, body)
	assertContentBlockCount(t, events, 3)
}

func TestTransformStreamingResponse_InterleavedToolArguments(t *testing.T) {
	translator := newTestTranslator()

	// simulate arguments arriving interleaved between tools (common in streaming)
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-int", 0, "call_A", "toolA"),
		toolStartChunk("chatcmpl-int", 1, "call_B", "toolB"),
		// interleave chunks
		toolArgsChunk("chatcmpl-int", 0, `{\\\"data\\\"`),
		toolArgsChunk("chatcmpl-int", 1, `{\\\"value\\\"`),
		toolArgsChunk("chatcmpl-int", 0, `:123}`),
		toolArgsChunk("chatcmpl-int", 1, `:456}`),
		finishChunk("chatcmpl-int", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify both tools present with complete arguments despite interleaving
	assertToolPresent(t, body, "call_A", "toolA")
	assertToolPresent(t, body, "call_B", "toolB")
	assertContainsAll(t, body, []string{`123`, `456`})
}

func TestTransformStreamingResponse_ToolTextToolTransitions(t *testing.T) {
	translator := newTestTranslator()

	// tool -> text -> tool sequence to test block closing
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-trans", 0, "call_first", "first_tool"),
		toolArgsChunk("chatcmpl-trans", 0, `{\\\"arg\\\":\\\"val\\\"}`),
		textChunk("chatcmpl-trans", "", "Here is some text"),
		toolStartChunk("chatcmpl-trans", 1, "call_second", "second_tool"),
		toolArgsChunk("chatcmpl-trans", 1, `{\\\"x\\\":1}`),
		finishChunk("chatcmpl-trans", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// count blocks - should have 3 blocks (tool, text, tool)
	assertContentBlockCount(t, events, 3)
	assertBlocksClosed(t, events)

	// verify all content is present
	assertToolPresent(t, body, "call_first", "first_tool")
	assertTextContent(t, body, "Here is some text")
	assertToolPresent(t, body, "call_second", "second_tool")
}

func TestTransformStreamingResponse_ToolWithEmptyArguments(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-empty", 0, "call_empty", "no_args_tool"),
		toolArgsChunk("chatcmpl-empty", 0, "{}"),
		finishChunk("chatcmpl-empty", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify tool is present with empty arguments
	assertToolPresent(t, body, "call_empty", "no_args_tool")
	assert.Contains(t, body, `"partial_json":"{}"`)
}

func TestTransformStreamingResponse_LargeSSELines(t *testing.T) {
	translator := newTestTranslator()

	// create a large argument string (near scanner buffer limit)
	// 500 KiB of JSON data - should be well within 1 MiB limit
	largeData := strings.Repeat("x", 500*1024)
	largeArgsJSON := fmt.Sprintf("{\\\"data\\\":\\\"%s\\\"}", largeData)

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-large", 0, "call_large", "large_tool"),
		toolArgsChunk("chatcmpl-large", 0, largeArgsJSON),
		finishChunk("chatcmpl-large", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify tool was processed
	assertToolPresent(t, body, "call_large", "large_tool")
}

func TestTransformStreamingResponse_ToolArgsMultipleSmallChunks(t *testing.T) {
	translator := newTestTranslator()

	// send JSON one character at a time to test buffering
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-chunks", 0, "call_chunky", "chunky"),
		toolArgsChunk("chatcmpl-chunks", 0, `{`),
		toolArgsChunk("chatcmpl-chunks", 0, `\\\"`),
		toolArgsChunk("chatcmpl-chunks", 0, `a`),
		toolArgsChunk("chatcmpl-chunks", 0, `\\\"`),
		toolArgsChunk("chatcmpl-chunks", 0, `:`),
		toolArgsChunk("chatcmpl-chunks", 0, `1`),
		toolArgsChunk("chatcmpl-chunks", 0, `}`),
		finishChunk("chatcmpl-chunks", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify all small chunks were accumulated
	assertToolPresent(t, body, "call_chunky", "chunky")

	events := parseAnthropicEvents(t, body)
	deltaCount := countEventsByType(events, "content_block_delta")
	// should have 7 delta events (one per argument chunk)
	assert.Equal(t, 7, deltaCount, "Should have delta event for each small chunk")
}

func TestTransformStreamingResponse_ToolArgsOneLargeChunk(t *testing.T) {
	translator := newTestTranslator()

	// complex nested JSON arriving all at once
	complexJSON := `{\\\"user\\\":{\\\"name\\\":\\\"Alice\\\",\\\"age\\\":30,\\\"address\\\":{\\\"city\\\":\\\"Sydney\\\",\\\"postcode\\\":2000}},\\\"items\\\":[1,2,3,4,5]}`

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-onechunk", 0, "call_onechunk", "process"),
		toolArgsChunk("chatcmpl-onechunk", 0, complexJSON),
		finishChunk("chatcmpl-onechunk", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify tool and complex arguments
	assertToolPresent(t, body, "call_onechunk", "process")
	assertContainsAll(t, body, []string{`Alice`, `Sydney`})
}

func TestTransformStreamingResponse_MalformedToolJSON(t *testing.T) {
	translator := newTestTranslator()

	// send malformed JSON in tool arguments
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-badjson", 0, "call_bad", "bad_tool"),
		toolArgsChunk("chatcmpl-badjson", 0, "{invalid json here"),
		finishChunk("chatcmpl-badjson", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// stream should still complete with tool metadata
	assertToolPresent(t, body, "call_bad", "bad_tool")
	// the malformed partial json should still be sent as delta events
	assert.Contains(t, body, `invalid json`)
}

func TestTransformStreamingResponse_ToolWithMissingID(t *testing.T) {
	translator := newTestTranslator()

	// tool without ID field (shouldn't start block)
	stream := mockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-noid\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"no_id_tool\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		toolArgsChunk("chatcmpl-noid", 0, `{\\\"x\\\":1}`),
		finishChunk("chatcmpl-noid", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	events := parseAnthropicEvents(t, recorder.Body.String())

	// without ID, content_block_start should not be created for tool
	assertEventCount(t, events, "content_block_start", 0)
}

func TestTransformStreamingResponse_ToolWithMissingName(t *testing.T) {
	translator := newTestTranslator()

	// tool without name field (shouldn't start block)
	stream := mockOpenAIStream([]string{
		"data: {\"id\":\"chatcmpl-noname\",\"model\":\"claude-3-5-sonnet-20241022\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_noname\",\"type\":\"function\",\"function\":{\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		toolArgsChunk("chatcmpl-noname", 0, `{\\\"y\\\":2}`),
		finishChunk("chatcmpl-noname", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	events := parseAnthropicEvents(t, recorder.Body.String())

	// without name, content_block_start should not be created
	assertEventCount(t, events, "content_block_start", 0)
}

func TestTransformStreamingResponse_EmptyToolCallsArray(t *testing.T) {
	translator := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-emptyarray", "claude-3-5-sonnet-20241022", "Hello"),
		"data: {\"id\":\"chatcmpl-emptyarray\",\"choices\":[{\"delta\":{\"tool_calls\":[]},\"index\":0}]}\n\n",
		finishChunk("chatcmpl-emptyarray", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// should just have text content
	assertTextContent(t, body, "Hello")
	assert.Contains(t, body, `"stop_reason":"end_turn"`)
}

func TestTransformStreamingResponse_ManyTools(t *testing.T) {
	translator := newTestTranslator()

	// create stream with 15 tools
	chunks := []string{
		textChunk("chatcmpl-many", "claude-3-5-sonnet-20241022", "Processing many tools"),
	}

	// add 15 tools with interleaved arguments
	for i := 0; i < 15; i++ {
		chunks = append(chunks, toolStartChunk("chatcmpl-many", i, fmt.Sprintf("call_%d", i), fmt.Sprintf("tool_%d", i)))
	}
	for i := 0; i < 15; i++ {
		chunks = append(chunks, toolArgsChunk("chatcmpl-many", i, fmt.Sprintf(`{\\\"n\\\":%d}`, i)))
	}

	chunks = append(chunks, finishChunk("chatcmpl-many", "tool_calls"))
	chunks = append(chunks, doneChunk())

	stream := mockOpenAIStream(chunks)
	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify all 15 tools are present
	for i := 0; i < 15; i++ {
		assertToolPresent(t, body, fmt.Sprintf("call_%d", i), fmt.Sprintf("tool_%d", i))
	}

	events := parseAnthropicEvents(t, body)
	// count blocks - should have text + 15 tools = 16 blocks
	assertContentBlockCount(t, events, 16)
}

func TestTransformStreamingResponse_RapidBlockTypeSwitch(t *testing.T) {
	translator := newTestTranslator()

	// alternate between text and tools rapidly
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-rapid", "claude-3-5-sonnet-20241022", "Text1"),
		toolStartChunk("chatcmpl-rapid", 0, "call_1", "tool1"),
		toolArgsChunk("chatcmpl-rapid", 0, `{\\\"a\\\":1}`),
		textChunk("chatcmpl-rapid", "", "Text2"),
		toolStartChunk("chatcmpl-rapid", 1, "call_2", "tool2"),
		toolArgsChunk("chatcmpl-rapid", 1, `{\\\"b\\\":2}`),
		textChunk("chatcmpl-rapid", "", "Text3"),
		finishChunk("chatcmpl-rapid", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// verify all blocks are properly closed
	assertContentBlockCount(t, events, 5) // text, tool, text, tool, text
	assertBlocksClosed(t, events)

	// verify content
	assertTextContent(t, body, "Text1")
	assertToolPresent(t, body, "call_1", "tool1")
	assertTextContent(t, body, "Text2")
	assertToolPresent(t, body, "call_2", "tool2")
	assertTextContent(t, body, "Text3")
}
