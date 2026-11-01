package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
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

	// Interleaved tool starts followed by interleaved args. Under the single-active-block
	// invariant, toolStart(1) closes block 0 before opening block 1. Args for tool 0
	// arriving after its block was stopped trigger an SSE error event and abort the stream;
	// corrupt tool input is worse than a clean failure the client can retry.
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-int", 0, "call_A", "toolA"),
		toolStartChunk("chatcmpl-int", 1, "call_B", "toolB"),
		// args for tool 0 arrive after its block was stopped by tool 1's start
		toolArgsChunk("chatcmpl-int", 0, `{\\\"data\\\"`),
		toolArgsChunk("chatcmpl-int", 1, `{\\\"value\\\"`),
		toolArgsChunk("chatcmpl-int", 0, `:123}`),
		toolArgsChunk("chatcmpl-int", 1, `:456}`),
		finishChunk("chatcmpl-int", "tool_calls"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), stream, recorder, nil)
	// stream must be aborted with the sentinel error
	require.ErrorIs(t, err, errInterleavedToolArguments)

	body := recorder.Body.String()

	// Both tool starts must have been emitted before the abort.
	assertToolPresent(t, body, "call_A", "toolA")
	assertToolPresent(t, body, "call_B", "toolB")

	// SSE error event must be on the wire.
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"type":"error"`)
	assert.Contains(t, body, `"type":"api_error"`)

	// message_delta and message_stop must NOT appear after the error event.
	assert.NotContains(t, body, "event: message_delta")
	assert.NotContains(t, body, "event: message_stop")
}

// TestTransformStreamingResponse_PreInitBufferedArgsFlush verifies that args
// arriving before the id+name chunk are emitted as the first input_json_delta
// immediately after content_block_start, making the pre-init case lossless.
func TestTransformStreamingResponse_PreInitBufferedArgsFlush(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// Construct a stream where the args chunk for tool 0 arrives before the
	// toolStart (id+name) chunk.  This simulates a backend that delivers function
	// arguments in the first delta before the function metadata chunk.
	preInitArgs := `{\\\"prefix\\\"`
	laterArgs := `:\\\"value\\\"}`

	// Build the raw SSE bytes manually so we can put the args chunk first.
	stream := mockOpenAIStream([]string{
		toolArgsChunk("chatcmpl-preinit", 0, preInitArgs), // args arrive before id+name
		toolStartChunk("chatcmpl-preinit", 0, "call_pre", "pre_tool"),
		toolArgsChunk("chatcmpl-preinit", 0, laterArgs),
		finishChunk("chatcmpl-preinit", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, tr, stream)
	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// Tool block must be present.
	assertToolPresent(t, body, "call_pre", "pre_tool")

	// Both the pre-init and later args must appear in input_json_delta events.
	assert.Contains(t, body, "prefix")
	assert.Contains(t, body, "value")

	// There must be at least two input_json_delta events: one for the pre-init
	// flush and one (or more) for the later args.
	deltas := findEventsByType(events, "content_block_delta")
	require.GreaterOrEqual(t, len(deltas), 2, "expected at least 2 deltas: pre-init flush + later args")

	// All deltas must reference the single tool block (index 0).
	for _, d := range deltas {
		idx, ok := getContentBlockIndex(d)
		require.True(t, ok)
		assert.Equal(t, 0, idx, "all deltas should reference block 0")
	}

	assertBlocksClosed(t, events)
}

// TestTransformStreamingResponse_MessageStartCacheFields verifies that message_start
// carries the cache_creation_input_tokens and cache_read_input_tokens fields (both 0).
func TestTransformStreamingResponse_MessageStartCacheFields(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-cache", "claude-3-5-sonnet-20241022", "Hi"),
		finishChunk("chatcmpl-cache", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	start := getEventByType(events, "message_start")
	require.NotNil(t, start)
	message, ok := start["message"].(map[string]interface{})
	require.True(t, ok)
	usage, ok := message["usage"].(map[string]interface{})
	require.True(t, ok)

	_, hasCacheCreate := usage["cache_creation_input_tokens"]
	_, hasCacheRead := usage["cache_read_input_tokens"]
	assert.True(t, hasCacheCreate, "message_start usage must include cache_creation_input_tokens")
	assert.True(t, hasCacheRead, "message_start usage must include cache_read_input_tokens")
	assert.Equal(t, float64(0), usage["cache_creation_input_tokens"])
	assert.Equal(t, float64(0), usage["cache_read_input_tokens"])

	// stop_reason and stop_sequence must be present in the message object
	_, hasStopReason := message["stop_reason"]
	_, hasStopSeq := message["stop_sequence"]
	assert.True(t, hasStopReason, "message_start message must include stop_reason")
	assert.True(t, hasStopSeq, "message_start message must include stop_sequence")
}

// TestTransformStreamingResponse_MessageDeltaCacheFields verifies that message_delta
// carries the cache_creation_input_tokens and cache_read_input_tokens fields (both 0).
func TestTransformStreamingResponse_MessageDeltaCacheFields(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-deltacache", "claude-3-5-sonnet-20241022", "Hi"),
		finishChunkWithUsage("chatcmpl-deltacache", "stop", 5, 3),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	delta := getEventByType(events, "message_delta")
	require.NotNil(t, delta)
	usage, ok := delta["usage"].(map[string]interface{})
	require.True(t, ok)

	_, hasCacheCreate := usage["cache_creation_input_tokens"]
	_, hasCacheRead := usage["cache_read_input_tokens"]
	assert.True(t, hasCacheCreate, "message_delta usage must include cache_creation_input_tokens")
	assert.True(t, hasCacheRead, "message_delta usage must include cache_read_input_tokens")
	assert.Equal(t, float64(0), usage["cache_creation_input_tokens"])
	assert.Equal(t, float64(0), usage["cache_read_input_tokens"])
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

	// create stream with 15 tools, sending each start then its own args immediately
	// (sequential, not interleaved) so no block is already stopped when args arrive.
	chunks := []string{
		textChunk("chatcmpl-many", "claude-3-5-sonnet-20241022", "Processing many tools"),
	}

	for i := range 15 {
		chunks = append(chunks, toolStartChunk("chatcmpl-many", i, fmt.Sprintf("call_%d", i), fmt.Sprintf("tool_%d", i)))
		chunks = append(chunks, toolArgsChunk("chatcmpl-many", i, fmt.Sprintf(`{\\\"n\\\":%d}`, i)))
	}

	chunks = append(chunks, finishChunk("chatcmpl-many", "tool_calls"))
	chunks = append(chunks, doneChunk())

	stream := mockOpenAIStream(chunks)
	recorder := executeTransform(t, translator, stream)
	body := recorder.Body.String()

	// verify all 15 tools are present
	for i := range 15 {
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

// TestTransformStreamingResponse_MessageStartInputTokensSeeded verifies that when
// the handler injects a pre-computed token estimate into context (via
// ContextInputTokensKey), the message_start event carries that non-zero value.
//
// vLLM and lmdeploy both populate real input_tokens in message_start from the first
// upstream chunk; Olla's translation path seeds the value from the request body
// estimator to match that behaviour without buffering the stream.
//
// output_tokens in message_start must remain 0 per the Anthropic spec.
func TestTransformStreamingResponse_MessageStartInputTokensSeeded(t *testing.T) {
	trans := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-seeded", "claude-3-5-sonnet-20241022", "Hello"),
		finishChunk("chatcmpl-seeded", "stop"),
		doneChunk(),
	})

	// Simulate the handler injecting a pre-computed estimate into context.
	const seedTokens = 42
	ctx := context.WithValue(context.Background(), constants.ContextInputTokensKey, seedTokens)

	recorder := httptest.NewRecorder()
	err := trans.TransformStreamingResponse(ctx, stream, recorder, nil)
	require.NoError(t, err)

	events := parseAnthropicEvents(t, recorder.Body.String())

	messageStart := getEventByType(events, "message_start")
	require.NotNil(t, messageStart, "message_start event must be present")

	message, ok := messageStart["message"].(map[string]interface{})
	require.True(t, ok, "message_start must have a message field")

	usage, ok := message["usage"].(map[string]interface{})
	require.True(t, ok, "message_start.message must have a usage field")

	inputTokens, ok := usage["input_tokens"].(float64)
	require.True(t, ok, "usage must have input_tokens")
	assert.Equal(t, float64(seedTokens), inputTokens,
		"message_start input_tokens must reflect the seeded estimate")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage must have output_tokens")
	assert.Equal(t, float64(0), outputTokens,
		"message_start output_tokens must be 0 per Anthropic spec")
}

// TestTransformStreamingResponse_MessageStartNoSeedIsZero confirms baseline behaviour:
// without a seeded estimate in context, message_start input_tokens is 0.
// This covers the passthrough path and any caller that does not inject the key.
func TestTransformStreamingResponse_MessageStartNoSeedIsZero(t *testing.T) {
	trans := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-noseed", "claude-3-5-sonnet-20241022", "Hi"),
		finishChunk("chatcmpl-noseed", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	err := trans.TransformStreamingResponse(context.Background(), stream, recorder, nil)
	require.NoError(t, err)

	events := parseAnthropicEvents(t, recorder.Body.String())
	messageStart := getEventByType(events, "message_start")
	require.NotNil(t, messageStart)

	message := messageStart["message"].(map[string]interface{})
	usage := message["usage"].(map[string]interface{})
	inputTokens := usage["input_tokens"].(float64)
	assert.Equal(t, float64(0), inputTokens, "no seed in context should give 0 input_tokens in message_start")
}

// TestTransformStreamingResponse_TwoSequentialToolCallsExactSequence pins the full SSE
// event order for a stream containing two back-to-back tool calls. Every
// content_block_start must be paired with a content_block_stop and indices must
// correspond to the correct block throughout.
func TestTransformStreamingResponse_TwoSequentialToolCallsExactSequence(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-seq2", 0, "call_seq_0", "tool_alpha"),
		toolArgsChunk("chatcmpl-seq2", 0, `{`),
		toolArgsChunk("chatcmpl-seq2", 0, `\\\"x\\\":1}`),
		toolStartChunk("chatcmpl-seq2", 1, "call_seq_1", "tool_beta"),
		toolArgsChunk("chatcmpl-seq2", 1, `{`),
		toolArgsChunk("chatcmpl-seq2", 1, `\\\"y\\\":2}`),
		finishChunk("chatcmpl-seq2", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, tr, stream)
	events := parseAnthropicEvents(t, recorder.Body.String())

	// Exact event sequence required by the Anthropic SSE spec.
	assertEventSequence(t, events, []string{
		"message_start",
		"content_block_start", // index 0, tool_alpha
		"content_block_delta", // index 0, partial_json {
		"content_block_delta", // index 0, partial_json \"x\":1}
		"content_block_stop",  // index 0
		"content_block_start", // index 1, tool_beta
		"content_block_delta", // index 1, partial_json {
		"content_block_delta", // index 1, partial_json \"y\":2}
		"content_block_stop",  // index 1
		"message_delta",
		"message_stop",
	})

	starts := findEventsByType(events, "content_block_start")
	stops := findEventsByType(events, "content_block_stop")
	require.Len(t, starts, 2, "two tool blocks must be started")
	require.Len(t, stops, 2, "both tool blocks must be stopped")

	// Block 0 is tool_alpha at index 0.
	idx0, _ := getContentBlockIndex(starts[0])
	assert.Equal(t, 0, idx0)
	bt0, _ := getContentBlockType(starts[0])
	assert.Equal(t, contentTypeToolUse, bt0)

	// Block 1 is tool_beta at index 1.
	idx1, _ := getContentBlockIndex(starts[1])
	assert.Equal(t, 1, idx1)
	bt1, _ := getContentBlockType(starts[1])
	assert.Equal(t, contentTypeToolUse, bt1)

	// Each stop event carries the correct index.
	stopIdx0, _ := getContentBlockIndex(stops[0])
	assert.Equal(t, 0, stopIdx0)
	stopIdx1, _ := getContentBlockIndex(stops[1])
	assert.Equal(t, 1, stopIdx1)

	// Deltas must carry the owning block's index, not the current block at emit time.
	deltas := findEventsByType(events, "content_block_delta")
	require.Len(t, deltas, 4)
	di0, _ := getContentBlockIndex(deltas[0])
	assert.Equal(t, 0, di0, "first delta must be for block 0")
	di1, _ := getContentBlockIndex(deltas[1])
	assert.Equal(t, 0, di1, "second delta must be for block 0")
	di2, _ := getContentBlockIndex(deltas[2])
	assert.Equal(t, 1, di2, "third delta must be for block 1")
	di3, _ := getContentBlockIndex(deltas[3])
	assert.Equal(t, 1, di3, "fourth delta must be for block 1")

	assertStopReason(t, events, "tool_use")
}

// TestTransformStreamingResponse_TextThenToolExactSequence pins the event order when
// a text block transitions into a tool call. The text block must be stopped before
// the tool block starts; this is already guarded by assertBlockTransitionOrder but
// this test also verifies the exact full sequence.
func TestTransformStreamingResponse_TextThenToolExactSequence(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-txttool", "claude-3-5-sonnet-20241022", "Thinking..."),
		toolStartChunk("chatcmpl-txttool", 0, "call_after_text", "lookup"),
		toolArgsChunk("chatcmpl-txttool", 0, `{\\\"k\\\":\\\"v\\\"}`),
		finishChunk("chatcmpl-txttool", "tool_calls"),
		doneChunk(),
	})

	recorder := executeTransform(t, tr, stream)
	events := parseAnthropicEvents(t, recorder.Body.String())

	assertEventSequence(t, events, []string{
		"message_start",
		"content_block_start", // index 0, text
		"content_block_delta", // text_delta "Thinking..."
		"content_block_stop",  // index 0 stopped before tool starts
		"content_block_start", // index 1, tool_use
		"content_block_delta", // input_json_delta
		"content_block_stop",  // index 1
		"message_delta",
		"message_stop",
	})

	assertBlocksClosed(t, events)
	assertBlockTransitionOrder(t, events)

	// text block is index 0, tool block is index 1
	starts := findEventsByType(events, "content_block_start")
	textIdx, _ := getContentBlockIndex(starts[0])
	assert.Equal(t, 0, textIdx)
	toolIdx, _ := getContentBlockIndex(starts[1])
	assert.Equal(t, 1, toolIdx)
}

// TestTransformStreamingResponse_LateArgDeltaDropped verifies that a late delta
// arriving for a tool whose block has already been stopped aborts the stream with
// errInterleavedToolArguments and emits a spec-valid SSE error event. The stream
// must not emit message_delta or message_stop after the error.
func TestTransformStreamingResponse_LateArgDeltaDropped(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// tool 0 starts and gets an arg, then tool 1 starts (which closes tool 0),
	// then a late arg arrives for tool 0 -- must abort the stream.
	stream := mockOpenAIStream([]string{
		toolStartChunk("chatcmpl-late", 0, "call_early", "first_tool"),
		toolArgsChunk("chatcmpl-late", 0, `{\\\"a\\\":1}`),
		toolStartChunk("chatcmpl-late", 1, "call_late_target", "second_tool"),
		toolArgsChunk("chatcmpl-late", 0, `{\\\"late\\\":true}`), // late for tool 0 -- aborts
		toolArgsChunk("chatcmpl-late", 1, `{\\\"b\\\":2}`),
		finishChunk("chatcmpl-late", "tool_calls"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	err := tr.TransformStreamingResponse(context.Background(), stream, recorder, nil)
	require.ErrorIs(t, err, errInterleavedToolArguments)

	body := recorder.Body.String()

	// SSE error event must be present and no finalise events must follow.
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"type":"api_error"`)
	assert.NotContains(t, body, "event: message_delta")
	assert.NotContains(t, body, "event: message_stop")

	// Both tool block starts (emitted before the abort) must be present.
	assertToolPresent(t, body, "call_early", "first_tool")
	assertToolPresent(t, body, "call_late_target", "second_tool")
}

// TestTransformStreamingResponse_UsageOnlyFinalChunk verifies that a terminal chunk
// with choices:[] and a populated usage object -- the format emitted by vLLM and
// other backends that honour include_usage=true -- is correctly captured and reflected
// in the message_delta usage. Previously the empty-choices guard returned early before
// reading chunk.Usage, so message_delta kept the pre-stream estimate or showed
// output_tokens:0.
func TestTransformStreamingResponse_UsageOnlyFinalChunk(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// Stream layout:
	//   1. text chunk (carries model name)
	//   2. finish chunk (no usage, just the finish reason)
	//   3. usage-only chunk with choices:[] (the backend's include_usage terminal chunk)
	//   4. [DONE]
	//
	// The usage in step 3 must win over any estimate seeded earlier.
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-uonly", "claude-3-5-sonnet-20241022", "Hi"),
		finishChunk("chatcmpl-uonly", "stop"),
		usageOnlyChunk("chatcmpl-uonly", 37, 14),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	// Real token counts must appear in message_delta, not 0 or a stale estimate.
	assertUsageTokens(t, events, 37, 14)
}

// TestTransformStreamingResponse_UsageOnlyChunkOverridesEstimate verifies that the
// real backend usage in a terminal choices:[] chunk overwrites a non-zero estimate
// that was seeded into context before the stream started. This guards against the
// common case where the estimate is close but not exact.
func TestTransformStreamingResponse_UsageOnlyChunkOverridesEstimate(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-override", "claude-3-5-sonnet-20241022", "Hello"),
		finishChunk("chatcmpl-override", "stop"),
		usageOnlyChunk("chatcmpl-override", 55, 20),
		doneChunk(),
	})

	// Seed a non-zero estimate that differs from the real usage.
	const seedTokens = 10
	ctx := context.WithValue(context.Background(), constants.ContextInputTokensKey, seedTokens)

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(ctx, stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	// Real counts from the usage-only chunk must replace the seeded estimate.
	assertUsageTokens(t, events, 55, 20)
}

// TestTransformStreamingResponse_RepeatedToolIDNameDoesNotReinit verifies that when
// a backend repeats the tool id + name on continuation chunks (alongside args), only
// a single content_block_start is emitted for that tool. Re-initialising would close
// the active block and open a duplicate, replaying already-buffered args.
func TestTransformStreamingResponse_RepeatedToolIDNameDoesNotReinit(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// First chunk: id + name + partial args (some backends send all three together).
	// Second chunk: id + name AGAIN + more args (the bug trigger).
	// Third chunk: args only (normal continuation).
	firstChunk := `data: {"id":"chatcmpl-repeat","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_rep","type":"function","function":{"name":"rep_tool","arguments":"{\\\"a\\\":"}}]},"index":0}]}` + "\n\n"
	repeatChunk := `data: {"id":"chatcmpl-repeat","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_rep","type":"function","function":{"name":"rep_tool","arguments":"1,"}}]},"index":0}]}` + "\n\n"
	finalChunk := `data: {"id":"chatcmpl-repeat","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\\"b\\\":2}"}}]},"index":0}]}` + "\n\n"

	stream := mockOpenAIStream([]string{
		firstChunk,
		repeatChunk,
		finalChunk,
		finishChunk("chatcmpl-repeat", "tool_calls"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// Exactly one tool block must have been started, not two.
	assertContentBlockCount(t, events, 1)
	assertBlocksClosed(t, events)

	// The single tool block must carry the correct id and name.
	assertToolPresent(t, body, "call_rep", "rep_tool")

	// All input_json_delta events must reference block index 0 - no spurious block 1.
	deltas := findEventsByType(events, "content_block_delta")
	require.NotEmpty(t, deltas, "expected at least one input_json_delta")
	for _, d := range deltas {
		idx, ok := getContentBlockIndex(d)
		require.True(t, ok)
		assert.Equal(t, 0, idx, "all deltas must reference block 0")
	}

	// Three arg chunks must have produced at least three input_json_delta events.
	// This confirms args from all chunks were emitted, not dropped on the repeated id+name.
	require.GreaterOrEqual(t, len(deltas), 3, "expected one delta per arg chunk")
}

// TestTransformStreamingResponse_SynthesisedOutputTokensWhenNoBackendUsage verifies
// that when the backend emits no usage in its stream, finalizeStream synthesises a
// non-zero output_tokens estimate from the accumulated content length (chars/4, min 1).
// This prevents Anthropic clients (including Claude Code) from receiving output_tokens:0.
func TestTransformStreamingResponse_SynthesisedOutputTokensWhenNoBackendUsage(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// Stream with several content chunks but no usage chunk at all - the common case
	// for most OpenAI-compatible backends that do not support include_usage.
	content := "Hello, world! This is synthesised." // 34 chars -> 34/4 = 8
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-synth", "claude-3-5-sonnet-20241022", content),
		finishChunk("chatcmpl-synth", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	delta := getEventByType(events, "message_delta")
	require.NotNil(t, delta, "message_delta must be present")

	usage, ok := delta["usage"].(map[string]interface{})
	require.True(t, ok, "message_delta must have usage")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage must have output_tokens")

	// Must be non-zero and approximately match chars/4 for the accumulated content.
	assert.Greater(t, outputTokens, float64(0),
		"output_tokens must be synthesised to a non-zero value when backend sends no usage")
	assert.InDelta(t, float64(len(content)/4), outputTokens, 2,
		"synthesised output_tokens should roughly match chars/4 of the accumulated content")
}

// TestTransformStreamingResponse_RealUsageWinsOverSynthesis verifies that when the
// backend does provide usage via a terminal choices:[] chunk, the real completion_tokens
// value is used in message_delta rather than the chars/4 estimate.
func TestTransformStreamingResponse_RealUsageWinsOverSynthesis(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	const realCompletionTokens = 20

	// The usageOnlyChunk emits a choices:[] terminal chunk -- the same format used by
	// vLLM and the ollamock terminal usage chunk. The translator must write 20, not the
	// estimate derived from the accumulated content.
	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-realusage", "claude-3-5-sonnet-20241022", "Hello world"),
		finishChunk("chatcmpl-realusage", "stop"),
		usageOnlyChunk("chatcmpl-realusage", 10, realCompletionTokens),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	delta := getEventByType(events, "message_delta")
	require.NotNil(t, delta, "message_delta must be present")

	usage, ok := delta["usage"].(map[string]interface{})
	require.True(t, ok, "message_delta must have usage")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage must have output_tokens")

	// Real backend value must win; synthesis must not fire when state.outputTokens != 0.
	assert.Equal(t, float64(realCompletionTokens), outputTokens,
		"real backend output_tokens must take precedence over the chars/4 estimate")
}

// TestTransformStreamingResponse_SynthesisedOutputTokensEmptyContent verifies that a
// stream with no text or thinking content (e.g. a tool-only response with no visible
// text) synthesises output_tokens:0, which is correct -- the client-visible content
// length is genuinely zero and a min-1 clamp would be misleading.
// The min-1 clamp is only applied when charCount > 0.
func TestTransformStreamingResponse_SynthesisedOutputTokensEmptyContent(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// Only a finish chunk, no content or usage -- simulates the truly-nothing case.
	stream := mockOpenAIStream([]string{
		finishChunk("chatcmpl-empty-synth", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	delta := getEventByType(events, "message_delta")
	require.NotNil(t, delta, "message_delta must be present")

	usage, ok := delta["usage"].(map[string]interface{})
	require.True(t, ok, "message_delta must have usage")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage must have output_tokens")

	// No content, no real usage -> output_tokens stays 0 (no spurious min-1 clamp).
	assert.Equal(t, float64(0), outputTokens,
		"output_tokens must be 0 when there is no content and no backend usage")
}

// TestTransformStreamingResponse_SynthesisedOutputTokensThinkingContent verifies that
// thinking (reasoning) block text is included in the synthesis estimate alongside
// regular text content, ensuring chain-of-thought responses get a plausible token count.
func TestTransformStreamingResponse_SynthesisedOutputTokensThinkingContent(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	// Reasoning chunk followed by a short text chunk, no backend usage.
	thinking := "Let me reason through this carefully."
	visible := "The answer is 42."
	stream := mockOpenAIStream([]string{
		reasoningChunk("chatcmpl-think-synth", "claude-3-5-sonnet-20241022", thinking),
		textChunk("chatcmpl-think-synth", "", visible),
		finishChunk("chatcmpl-think-synth", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	events := parseAnthropicEvents(t, recorder.Body.String())

	delta := getEventByType(events, "message_delta")
	require.NotNil(t, delta, "message_delta must be present")

	usage, ok := delta["usage"].(map[string]interface{})
	require.True(t, ok, "message_delta must have usage")

	outputTokens, ok := usage["output_tokens"].(float64)
	require.True(t, ok, "usage must have output_tokens")

	// Both thinking and visible content contribute to the estimate, so the token
	// count must be at least 1 and greater than visible text alone.
	totalChars := len(thinking) + len(visible)
	minExpected := float64(1)
	maxExpected := float64(totalChars/4 + 2) // +2 tolerance for integer division
	assert.GreaterOrEqual(t, outputTokens, minExpected,
		"synthesised output_tokens must be >= 1 when thinking+text content exists")
	assert.LessOrEqual(t, outputTokens, maxExpected,
		"synthesised output_tokens must not far exceed (thinking+text chars)/4")
}

// errAfterReader serves its payload and then fails, like a backend socket dying
// mid-stream (io.Pipe's CloseWithError surfaces the same way).
type errAfterReader struct {
	r   io.Reader
	err error
}

func (e *errAfterReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		return n, e.err
	}
	return n, err
}

// A read error means the response is truncated: the translator must surface it
// and must not finalise with message_delta/message_stop, which would make the
// partial reply look complete to the client.
func TestTransformStreamingResponse_ReadErrorDoesNotFinalise(t *testing.T) {
	translator := newTestTranslator()
	cause := fmt.Errorf("backend reset: %w", io.ErrUnexpectedEOF)
	stream := &errAfterReader{
		r: mockOpenAIStream([]string{
			textChunk("chatcmpl-1", "m", "Hello"),
			textChunk("chatcmpl-1", "", " wor"),
		}),
		err: cause,
	}

	recorder, err := executeTransformWithContext(t, translator, context.Background(), stream)

	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	body := recorder.Body.String()
	assert.Contains(t, body, "Hello", "content before the failure is still delivered")
	assert.NotContains(t, body, "event: message_stop")
	assert.NotContains(t, body, "event: message_delta")
}

func TestTransformStreamingResponse_ReadErrorBeforeAnyChunk(t *testing.T) {
	translator := newTestTranslator()
	recorder, err := executeTransformWithContext(t, translator, context.Background(),
		&errAfterReader{r: strings.NewReader(""), err: io.ErrClosedPipe})

	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.NotContains(t, recorder.Body.String(), "message_stop")
}
