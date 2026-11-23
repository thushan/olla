package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// buildTextStream constructs a synthetic OpenAI SSE stream with n text delta chunks
// followed by a finish chunk and the done marker. Used to exercise the content_block_delta
// hot path without the scanner/parse overhead dominating the benchmark.
func buildTextStream(n int, chunkText string) io.Reader {
	var sb strings.Builder
	// First chunk carries model name.
	sb.WriteString(textChunk("chatcmpl-bench", "claude-3-5-sonnet-20241022", chunkText))
	for i := 1; i < n; i++ {
		sb.WriteString(textChunk("chatcmpl-bench", "", chunkText))
	}
	sb.WriteString(finishChunk("chatcmpl-bench", "stop"))
	sb.WriteString(doneChunk())
	return strings.NewReader(sb.String())
}

// buildThinkingStream constructs a synthetic stream with n reasoning delta chunks
// followed by a single text chunk, finish, and done.
func buildThinkingStream(n int, chunkText string) io.Reader {
	var sb strings.Builder
	sb.WriteString(reasoningChunk("chatcmpl-bench-r", "deepseek-r1", chunkText))
	for i := 1; i < n; i++ {
		sb.WriteString(reasoningChunk("chatcmpl-bench-r", "", chunkText))
	}
	sb.WriteString(textChunk("chatcmpl-bench-r", "", "result"))
	sb.WriteString(finishChunk("chatcmpl-bench-r", "stop"))
	sb.WriteString(doneChunk())
	return strings.NewReader(sb.String())
}

// buildToolStream constructs a stream with a single tool call whose arguments arrive
// in n small chunks, exercising the input_json_delta hot path.
func buildToolStream(n int) io.Reader {
	var sb strings.Builder
	sb.WriteString(toolStartChunk("chatcmpl-bench-t", 0, "call_bench", "bench_tool"))
	for i := range n {
		sb.WriteString(toolArgsChunk("chatcmpl-bench-t", 0, fmt.Sprintf(`{"n":%d}`, i)))
	}
	sb.WriteString(finishChunk("chatcmpl-bench-t", "tool_calls"))
	sb.WriteString(doneChunk())
	return strings.NewReader(sb.String())
}

// BenchmarkStreaming_ContentDeltas measures allocations on the text_delta hot path.
// Run with: go test -bench=BenchmarkStreaming_ContentDeltas -benchmem -count=5
func BenchmarkStreaming_ContentDeltas(b *testing.B) {
	const chunks = 50 // representative of a medium-length response

	tr := newTestTranslator()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		stream := buildTextStream(chunks, "hello ")
		rec := httptest.NewRecorder()
		b.StartTimer()

		if err := tr.TransformStreamingResponse(ctx, stream, rec, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStreaming_ThinkingDeltas measures allocations on the thinking_delta hot path.
// Run with: go test -bench=BenchmarkStreaming_ThinkingDeltas -benchmem -count=5
func BenchmarkStreaming_ThinkingDeltas(b *testing.B) {
	const chunks = 50

	tr := newTestTranslator()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		stream := buildThinkingStream(chunks, "thinking step ")
		rec := httptest.NewRecorder()
		b.StartTimer()

		if err := tr.TransformStreamingResponse(ctx, stream, rec, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStreaming_ToolArgDeltas measures allocations on the input_json_delta hot path.
// Run with: go test -bench=BenchmarkStreaming_ToolArgDeltas -benchmem -count=5
func BenchmarkStreaming_ToolArgDeltas(b *testing.B) {
	const chunks = 50

	tr := newTestTranslator()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		stream := buildToolStream(chunks)
		rec := httptest.NewRecorder()
		b.StartTimer()

		if err := tr.TransformStreamingResponse(ctx, stream, rec, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteEvent_TextDelta isolates the writeEvent + JSON encode path for a single
// text_delta event. This directly measures the allocation savings from the pooled buffer
// and typed struct, independent of the incoming OpenAI chunk parse overhead.
// Run with: go test -bench=BenchmarkWriteEvent -benchmem -count=5
func BenchmarkWriteEvent_TextDelta(b *testing.B) {
	tr := newTestTranslator()
	rec := httptest.NewRecorder()
	evt := sseDeltaEvent{
		Delta: sseTextDelta{Text: "hello world", Type: "text_delta"},
		Index: 0,
		Type:  "content_block_delta",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		rec.Body.Reset()
		if err := tr.writeEvent(rec, "content_block_delta", evt); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteEvent_TextDelta_MapBaseline is a reference benchmark that uses the old
// map[string]interface{} approach so the before/after is visible in a single run.
// It is not wired into any production path - delete after measurements are taken.
func BenchmarkWriteEvent_TextDelta_MapBaseline(b *testing.B) {
	tr := newTestTranslator()
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		rec.Body.Reset()
		// Replicate exact old writeEvent logic: json.Marshal on a nested map + fmt.Fprintf.
		data := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": "hello world",
			},
		}
		if err := tr.writeEvent(rec, "content_block_delta", data); err != nil {
			b.Fatal(err)
		}
	}
}

// TestWriteEvent_ConcurrentPoolSafety exercises writeEvent from multiple goroutines
// simultaneously to confirm the pooled buffer is not aliased across callers.
// Run with: go test -run TestWriteEvent_ConcurrentPoolSafety -race
func TestWriteEvent_ConcurrentPoolSafety(t *testing.T) {
	t.Parallel()

	tr := newTestTranslator()
	ctx := context.Background()

	const goroutines = 20
	const chunksEach = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			stream := buildTextStream(chunksEach, fmt.Sprintf("goroutine %d ", id))
			rec := httptest.NewRecorder()
			if err := tr.TransformStreamingResponse(ctx, stream, rec, nil); err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
}

// BenchmarkProcessStreamLine_TextChunk isolates the per-line parse + dispatch cost
// for a single text delta chunk, after message_start has already been sent.
// This directly measures the typed-struct decode savings on the hot path, independent
// of scanner, HTTP, and SSE write overhead.
// Run with: go test -bench=BenchmarkProcessStreamLine_TextChunk -benchmem -count=5
func BenchmarkProcessStreamLine_TextChunk(b *testing.B) {
	tr := newTestTranslator()

	line := `data: {"id":"chatcmpl-b","model":"claude-3-5-sonnet-20241022","choices":[{"delta":{"content":"hello"},"index":0}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// Recreate recorder and state each iteration so alloc counts reflect a
		// single-call steady state and mutable fields don't bleed between runs.
		rec := httptest.NewRecorder()
		rc := http.NewResponseController(rec)
		state := &StreamingState{
			messageID:        "msg-bench",
			contentBlocks:    make([]ContentBlock, 0, 4),
			toolCallBuffers:  make(map[int]*strings.Builder),
			toolIndexToBlock: make(map[int]int),
			messageStartSent: true,
		}
		// Prime a text block so the hot path skips block_start overhead.
		state.currentBlock = &ContentBlock{Type: contentTypeText, Text: ""}
		state.currentIndex = 0
		state.contentBlocks = append(state.contentBlocks, *state.currentBlock)

		if err := tr.processStreamLine(line, state, rec, rc); err != nil {
			b.Fatal(err)
		}
	}
}
