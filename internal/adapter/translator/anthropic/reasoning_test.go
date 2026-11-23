package anthropic

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Streaming tests
// ---------------------------------------------------------------------------

// TestStreaming_Reasoning_ReasoningField verifies that "reasoning" deltas produce
// the full thinking block sequence before the text block.
func TestStreaming_Reasoning_ReasoningField(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		reasoningChunk("chatcmpl-r1", "deepseek-r1", "Let me think"),
		reasoningChunk("chatcmpl-r1", "", " about this"),
		textChunk("chatcmpl-r1", "", "The answer is 42"),
		finishChunk("chatcmpl-r1", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// thinking block must start before text block
	assertThinkingBlockTransitionOrder(t, events)

	// thinking block opened with correct type
	starts := findEventsByType(events, "content_block_start")
	require.GreaterOrEqual(t, len(starts), 2)
	thinkingType, _ := getContentBlockType(starts[0])
	assert.Equal(t, contentTypeThinking, thinkingType, "first block must be thinking")

	textType, _ := getContentBlockType(starts[1])
	assert.Equal(t, contentTypeText, textType, "second block must be text")

	// both blocks must be closed
	assertBlocksClosed(t, events)
	assertContentBlockCount(t, events, 2)

	// thinking text must appear in thinking_delta events
	assert.Contains(t, body, `"type":"thinking_delta"`)
	assert.Contains(t, body, `"thinking":"Let me think"`)
	assert.Contains(t, body, `"thinking":" about this"`)

	// text must appear in text_delta
	assertTextContent(t, body, "The answer is 42")
}

// TestStreaming_Reasoning_ReasoningContentField verifies that "reasoning_content"
// (vLLM / SGLang / DeepSeek naming) produces identical output to "reasoning".
func TestStreaming_Reasoning_ReasoningContentField(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		reasoningContentChunk("chatcmpl-rc1", "qwq-32b", "Step 1: analyse"),
		reasoningContentChunk("chatcmpl-rc1", "", " the problem"),
		textChunk("chatcmpl-rc1", "", "Done."),
		finishChunk("chatcmpl-rc1", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	assertThinkingBlockTransitionOrder(t, events)
	assertBlocksClosed(t, events)
	assertContentBlockCount(t, events, 2)

	assert.Contains(t, body, `"type":"thinking_delta"`)
	assertThinkingContent(t, body, "Step 1: analyse")
	assertThinkingContent(t, body, " the problem")
	assertTextContent(t, body, "Done.")
}

// TestStreaming_Reasoning_OnlyReasoningNoContent verifies that a reasoning-only
// response (no text content) still produces a complete, closed thinking block
// plus the standard message_delta / message_stop tail.
func TestStreaming_Reasoning_OnlyReasoningNoContent(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		reasoningChunk("chatcmpl-ro1", "deepseek-r1", "Internal monologue"),
		finishChunk("chatcmpl-ro1", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// exactly one content block: the thinking block
	assertContentBlockCount(t, events, 1)
	assertBlocksClosed(t, events)

	starts := findEventsByType(events, "content_block_start")
	require.Len(t, starts, 1)
	bt, _ := getContentBlockType(starts[0])
	assert.Equal(t, contentTypeThinking, bt)

	// full sequence must be present
	assertRequiredEvents(t, events)
	assert.Contains(t, body, `"thinking":"Internal monologue"`)
	assertStopReason(t, events, "end_turn")
}

// TestStreaming_Reasoning_NoReasoningUnchanged is a regression guard: when there is
// no reasoning field the output must be byte-identical to the pre-reasoning behaviour.
func TestStreaming_Reasoning_NoReasoningUnchanged(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		textChunk("chatcmpl-nr1", "some-model", "Hello world"),
		finishChunk("chatcmpl-nr1", "stop"),
		doneChunk(),
	})

	recorder := executeTransform(t, tr, stream)
	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// no thinking blocks at all
	assert.NotContains(t, body, contentTypeThinking)
	assert.NotContains(t, body, "thinking_delta")

	assertContentBlockCount(t, events, 1)
	starts := findEventsByType(events, "content_block_start")
	bt, _ := getContentBlockType(starts[0])
	assert.Equal(t, contentTypeText, bt)

	assertTextContent(t, body, "Hello world")
	assertBlocksClosed(t, events)
}

// TestStreaming_Reasoning_BlockOrderExactSequence pins the full SSE event sequence
// for reasoning + content to ensure the thinking block is fully closed before
// the text block opens (Anthropic spec requirement).
func TestStreaming_Reasoning_BlockOrderExactSequence(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		reasoningChunk("chatcmpl-seq", "deepseek-r1", "thinking..."),
		textChunk("chatcmpl-seq", "", "answer"),
		finishChunk("chatcmpl-seq", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))
	events := parseAnthropicEvents(t, recorder.Body.String())

	assertEventSequence(t, events, []string{
		"message_start",
		"content_block_start", // index 0, thinking
		"content_block_delta", // thinking_delta
		"content_block_stop",  // index 0 — thinking closed before text opens
		"content_block_start", // index 1, text
		"content_block_delta", // text_delta
		"content_block_stop",  // index 1
		"message_delta",
		"message_stop",
	})

	// verify indices
	starts := findEventsByType(events, "content_block_start")
	idx0, _ := getContentBlockIndex(starts[0])
	assert.Equal(t, 0, idx0)
	idx1, _ := getContentBlockIndex(starts[1])
	assert.Equal(t, 1, idx1)
}

// TestStreaming_Reasoning_MultipleChunksAccumulated verifies that multiple reasoning
// chunks produce separate thinking_delta events (one per chunk) rather than being
// collapsed, preserving the streaming semantics.
func TestStreaming_Reasoning_MultipleChunksAccumulated(t *testing.T) {
	t.Parallel()
	tr := newTestTranslator()

	stream := mockOpenAIStream([]string{
		reasoningChunk("chatcmpl-multi", "deepseek-r1", "chunk1"),
		reasoningChunk("chatcmpl-multi", "", "chunk2"),
		reasoningChunk("chatcmpl-multi", "", "chunk3"),
		textChunk("chatcmpl-multi", "", "result"),
		finishChunk("chatcmpl-multi", "stop"),
		doneChunk(),
	})

	recorder := httptest.NewRecorder()
	require.NoError(t, tr.TransformStreamingResponse(context.Background(), stream, recorder, nil))

	body := recorder.Body.String()
	events := parseAnthropicEvents(t, body)

	// three thinking_delta events (one per chunk), one text_delta
	deltas := findEventsByType(events, "content_block_delta")
	thinkingDeltas := 0
	textDeltas := 0
	for _, d := range deltas {
		delta, ok := d["delta"].(map[string]any)
		if !ok {
			continue
		}
		switch delta["type"] {
		case "thinking_delta":
			thinkingDeltas++
		case "text_delta":
			textDeltas++
		}
	}
	assert.Equal(t, 3, thinkingDeltas, "one thinking_delta per reasoning chunk")
	assert.Equal(t, 1, textDeltas, "one text_delta for content")

	// all blocks closed, thinking first
	assertBlocksClosed(t, events)
	assertThinkingBlockTransitionOrder(t, events)
}

// ---------------------------------------------------------------------------
// Non-streaming tests
// ---------------------------------------------------------------------------

// TestResponse_Reasoning_ReasoningField verifies that "reasoning" in a non-streaming
// OpenAI message maps to a leading thinking block in the Anthropic response.
func TestResponse_Reasoning_ReasoningField(t *testing.T) {
	t.Parallel()
	tr := mustNewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]any{
		"id":    "chatcmpl-nr",
		"model": "deepseek-r1",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":      "assistant",
					"reasoning": "First I consider the options.",
					"content":   "The answer is 7.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     float64(10),
			"completion_tokens": float64(5),
		},
	}

	result, err := tr.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	resp, ok := result.(AnthropicResponse)
	require.True(t, ok)
	require.Len(t, resp.Content, 2, "thinking block + text block")

	assert.Equal(t, contentTypeThinking, resp.Content[0].Type)
	assert.Equal(t, "First I consider the options.", resp.Content[0].Thinking)
	assert.Empty(t, resp.Content[0].Text, "thinking block must not set Text")

	assert.Equal(t, contentTypeText, resp.Content[1].Type)
	assert.Equal(t, "The answer is 7.", resp.Content[1].Text)
}

// TestResponse_Reasoning_ReasoningContentField verifies the "reasoning_content"
// field name variant (vLLM / SGLang / DeepSeek) maps to a thinking block.
func TestResponse_Reasoning_ReasoningContentField(t *testing.T) {
	t.Parallel()
	tr := mustNewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]any{
		"id":    "chatcmpl-rcc",
		"model": "qwq-32b",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":              "assistant",
					"reasoning_content": "Step 1: identify the pattern.",
					"content":           "Pattern identified.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{},
	}

	result, err := tr.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	resp, ok := result.(AnthropicResponse)
	require.True(t, ok)
	require.Len(t, resp.Content, 2)

	assert.Equal(t, contentTypeThinking, resp.Content[0].Type)
	assert.Equal(t, "Step 1: identify the pattern.", resp.Content[0].Thinking)

	assert.Equal(t, contentTypeText, resp.Content[1].Type)
	assert.Equal(t, "Pattern identified.", resp.Content[1].Text)
}

// TestResponse_Reasoning_NoReasoningUnchanged is a regression guard for non-streaming:
// without reasoning fields the content array must not gain a thinking block.
func TestResponse_Reasoning_NoReasoningUnchanged(t *testing.T) {
	t.Parallel()
	tr := mustNewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]any{
		"id":    "chatcmpl-norr",
		"model": "llama3",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Simple answer.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{},
	}

	result, err := tr.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	resp, ok := result.(AnthropicResponse)
	require.True(t, ok)
	require.Len(t, resp.Content, 1, "no thinking block when no reasoning field")
	assert.Equal(t, contentTypeText, resp.Content[0].Type)
	assert.Equal(t, "Simple answer.", resp.Content[0].Text)
}

// TestResponse_Reasoning_ReasoningOnlyNoContent verifies that a message with
// reasoning but no content body still yields a valid (single thinking block) response.
func TestResponse_Reasoning_ReasoningOnlyNoContent(t *testing.T) {
	t.Parallel()
	tr := mustNewTranslator(createResponseTestLogger(), createTestConfig())

	openaiResp := map[string]any{
		"id":    "chatcmpl-ron",
		"model": "deepseek-r1",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":      "assistant",
					"reasoning": "Internal thoughts only.",
					// no "content" field
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{},
	}

	result, err := tr.TransformResponse(context.Background(), openaiResp, nil)
	require.NoError(t, err)

	resp, ok := result.(AnthropicResponse)
	require.True(t, ok)

	// convertResponseContent falls back to a single empty text block when content is absent;
	// with reasoning it should have thinking + the empty text fallback.
	thinkingFound := false
	for _, block := range resp.Content {
		if block.Type == contentTypeThinking {
			thinkingFound = true
			assert.Equal(t, "Internal thoughts only.", block.Thinking)
		}
	}
	assert.True(t, thinkingFound, "thinking block must be present")
}
