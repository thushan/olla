package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
)

// Setup Helpers
// -------------

// newTestTranslator creates a configured translator for testing with error-level logging.
func newTestTranslator() *Translator {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLog := logger.NewPlainStyledLogger(log)
	return NewTranslator(styledLog, createStreamingTestConfig())
}

// createStreamingTestConfig creates a minimal config for streaming tests.
// Kept separate from newTestTranslator to allow config customization if needed.
func createStreamingTestConfig() config.AnthropicTranslatorConfig {
	return config.AnthropicTranslatorConfig{
		Enabled:        true,
		MaxMessageSize: 10 << 20, // 10MB
	}
}

// Mock Data Helpers
// -----------------

// mockOpenAIStream creates a mock OpenAI SSE stream from string chunks.
// Each chunk should be a complete SSE line (e.g., "data: {...}\n\n").
func mockOpenAIStream(chunks []string) io.Reader {
	var buf bytes.Buffer
	for _, chunk := range chunks {
		buf.WriteString(chunk)
	}
	return &buf
}

// textChunk creates an OpenAI SSE chunk containing text content.
// First chunk should include model name, subsequent chunks can omit it.
func textChunk(messageID, model, content string) string {
	if model != "" {
		return fmt.Sprintf("data: {\"id\":\"%s\",\"model\":\"%s\",\"choices\":[{\"delta\":{\"content\":\"%s\"},\"index\":0}]}\n\n",
			messageID, model, content)
	}
	return fmt.Sprintf("data: {\"id\":\"%s\",\"choices\":[{\"delta\":{\"content\":\"%s\"},\"index\":0}]}\n\n",
		messageID, content)
}

// toolStartChunk creates an OpenAI SSE chunk that starts a new tool call.
// This includes the tool ID, name, and initial (usually empty) arguments.
func toolStartChunk(messageID string, toolIndex int, toolID, toolName string) string {
	return fmt.Sprintf("data: {\"id\":\"%s\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":%d,\"id\":\"%s\",\"type\":\"function\",\"function\":{\"name\":\"%s\",\"arguments\":\"\"}}]},\"index\":0}]}\n\n",
		messageID, toolIndex, toolID, toolName)
}

// toolArgsChunk creates an OpenAI SSE chunk with tool argument data.
// Arguments are typically streamed in multiple chunks and concatenated.
func toolArgsChunk(messageID string, toolIndex int, args string) string {
	return fmt.Sprintf("data: {\"id\":\"%s\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":%d,\"function\":{\"arguments\":\"%s\"}}]},\"index\":0}]}\n\n",
		messageID, toolIndex, args)
}

// finishChunk creates an OpenAI SSE chunk indicating stream completion.
// Common finish reasons: "stop", "tool_calls", "length".
func finishChunk(messageID, finishReason string) string {
	return fmt.Sprintf("data: {\"id\":\"%s\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"%s\"}]}\n\n",
		messageID, finishReason)
}

// finishChunkWithUsage creates an OpenAI SSE chunk with finish reason and token usage.
func finishChunkWithUsage(messageID, finishReason string, promptTokens, completionTokens int) string {
	return fmt.Sprintf("data: {\"id\":\"%s\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"%s\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":%d,\"total_tokens\":%d}}\n\n",
		messageID, finishReason, promptTokens, completionTokens, promptTokens+completionTokens)
}

// doneChunk returns the standard OpenAI stream termination marker.
func doneChunk() string {
	return "data: [DONE]\n\n"
}

// modelChunk creates the first chunk with model information.
// Use this as the first chunk in a stream when you need to specify the model.
func modelChunk(messageID, model, content string) string {
	return textChunk(messageID, model, content)
}

// Transform Execution Helpers
// ---------------------------

// executeTransform runs the transformer and returns the recorder for inspection.
// This encapsulates the common pattern of creating a recorder, transforming, and checking errors.
func executeTransform(t *testing.T, translator *Translator, stream io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(context.Background(), stream, recorder, nil)
	require.NoError(t, err)
	return recorder
}

// executeTransformWithContext runs the transformer with a custom context.
func executeTransformWithContext(t *testing.T, translator *Translator, ctx context.Context, stream io.Reader) (*httptest.ResponseRecorder, error) {
	t.Helper()
	recorder := httptest.NewRecorder()
	err := translator.TransformStreamingResponse(ctx, stream, recorder, nil)
	return recorder, err
}

// Event Parsing Helpers
// ---------------------

// parseAnthropicEvents parses SSE output into structured event maps.
// Each event will have an "_event_type" field indicating the Anthropic event type.
func parseAnthropicEvents(t *testing.T, body string) []map[string]interface{} {
	t.Helper()
	events, err := parseAnthropicEventsWithErr(body)
	require.NoError(t, err)
	return events
}

// parseAnthropicEventsWithErr parses events but returns the error instead of failing.
// Use this when you want to test error handling.
func parseAnthropicEventsWithErr(body string) ([]map[string]interface{}, error) {
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

// Event Query Helpers
// -------------------

// findEventsByType returns all events matching the specified type.
func findEventsByType(events []map[string]interface{}, eventType string) []map[string]interface{} {
	var matches []map[string]interface{}
	for _, event := range events {
		if event["_event_type"] == eventType {
			matches = append(matches, event)
		}
	}
	return matches
}

// getEventByType returns the first event of the specified type, or nil if not found.
func getEventByType(events []map[string]interface{}, eventType string) map[string]interface{} {
	matches := findEventsByType(events, eventType)
	if len(matches) > 0 {
		return matches[0]
	}
	return nil
}

// countEventsByType returns the number of events of the specified type.
func countEventsByType(events []map[string]interface{}, eventType string) int {
	return len(findEventsByType(events, eventType))
}

// getEventTypes returns a list of all event types in order.
func getEventTypes(events []map[string]interface{}) []string {
	var types []string
	for _, event := range events {
		if eventType, ok := event["_event_type"].(string); ok {
			types = append(types, eventType)
		}
	}
	return types
}

// hasEventType checks if at least one event of the type exists.
func hasEventType(events []map[string]interface{}, eventType string) bool {
	return countEventsByType(events, eventType) > 0
}

// Validation Helpers
// ------------------

// assertEventSequence validates that events appear in the specified order.
// This requires exact match of the sequence.
func assertEventSequence(t *testing.T, events []map[string]interface{}, expectedTypes []string) {
	t.Helper()
	actualTypes := getEventTypes(events)
	assert.Equal(t, expectedTypes, actualTypes, "Event sequence should match expected order")
}

// assertEventCount validates the number of events of a specific type.
func assertEventCount(t *testing.T, events []map[string]interface{}, eventType string, expectedCount int) {
	t.Helper()
	actualCount := countEventsByType(events, eventType)
	assert.Equal(t, expectedCount, actualCount, "Event count for %s should be %d", eventType, expectedCount)
}

// assertHasEventType asserts that at least one event of the type exists.
func assertHasEventType(t *testing.T, events []map[string]interface{}, eventType string) {
	t.Helper()
	assert.True(t, hasEventType(events, eventType), "Should have at least one %s event", eventType)
}

// assertContentBlockCount validates the number of content blocks (start events).
func assertContentBlockCount(t *testing.T, events []map[string]interface{}, expectedCount int) {
	t.Helper()
	assertEventCount(t, events, "content_block_start", expectedCount)
}

// assertBlocksClosed validates that all started blocks are also stopped.
// This ensures proper block lifecycle management.
func assertBlocksClosed(t *testing.T, events []map[string]interface{}) {
	t.Helper()
	starts := countEventsByType(events, "content_block_start")
	stops := countEventsByType(events, "content_block_stop")
	assert.Equal(t, starts, stops, "All content blocks should be properly closed (starts=%d, stops=%d)", starts, stops)
}

// assertRequiredEvents checks that all essential Anthropic stream events are present.
// A valid stream must have: message_start, message_stop, and typically message_delta.
func assertRequiredEvents(t *testing.T, events []map[string]interface{}) {
	t.Helper()
	assertHasEventType(t, events, "message_start")
	assertHasEventType(t, events, "message_stop")
	// message_delta is optional if there's no finish reason or usage info
}

// String Assertion Helpers
// ------------------------

// assertContainsAll verifies that the body contains all expected substrings.
func assertContainsAll(t *testing.T, body string, substrings []string) {
	t.Helper()
	for _, s := range substrings {
		assert.Contains(t, body, s)
	}
}

// assertSSEFormat validates proper SSE formatting (event: name\ndata: json\n\n).
func assertSSEFormat(t *testing.T, body string) {
	t.Helper()
	lines := strings.Split(body, "\n")

	var eventFound, dataFound bool
	for i, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			eventFound = true
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
				dataFound = true
				dataStr := strings.TrimPrefix(lines[i+1], "data: ")
				var data map[string]interface{}
				err := json.Unmarshal([]byte(dataStr), &data)
				assert.NoError(t, err, "Data field should contain valid JSON")
			}
		}
	}

	assert.True(t, eventFound, "Should have event: lines")
	assert.True(t, dataFound, "Should have data: lines following event: lines")
}

// Field Extraction Helpers
// ------------------------

// extractField safely extracts a nested field from an event map.
// Example: extractField(event, "message", "usage", "input_tokens")
func extractField(event map[string]interface{}, path ...string) (interface{}, bool) {
	current := event
	for i, key := range path {
		if i == len(path)-1 {
			val, ok := current[key]
			return val, ok
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

// getUsageTokens extracts token counts from a message_delta event.
func getUsageTokens(event map[string]interface{}) (inputTokens, outputTokens float64, ok bool) {
	usage, found := event["usage"].(map[string]interface{})
	if !found {
		return 0, 0, false
	}

	input, hasInput := usage["input_tokens"].(float64)
	output, hasOutput := usage["output_tokens"].(float64)

	return input, output, hasInput && hasOutput
}

// getStopReason extracts the stop_reason from a message_delta event.
func getStopReason(event map[string]interface{}) (string, bool) {
	delta, ok := event["delta"].(map[string]interface{})
	if !ok {
		return "", false
	}
	reason, ok := delta["stop_reason"].(string)
	return reason, ok
}

// getContentBlockType extracts the type from a content_block_start event.
func getContentBlockType(event map[string]interface{}) (string, bool) {
	block, ok := event["content_block"].(map[string]interface{})
	if !ok {
		return "", false
	}
	blockType, ok := block["type"].(string)
	return blockType, ok
}

// getContentBlockIndex extracts the index from a content block event.
func getContentBlockIndex(event map[string]interface{}) (int, bool) {
	index, ok := event["index"].(float64)
	if !ok {
		return 0, false
	}
	return int(index), true
}

// Complex Validation Helpers
// --------------------------

// assertToolPresent validates that a tool with the given ID and name appears in the stream.
func assertToolPresent(t *testing.T, body string, toolID, toolName string) {
	t.Helper()
	assert.Contains(t, body, fmt.Sprintf(`"id":"%s"`, toolID))
	assert.Contains(t, body, fmt.Sprintf(`"name":"%s"`, toolName))
}

// assertTextContent validates that text content appears in the stream.
func assertTextContent(t *testing.T, body string, content string) {
	t.Helper()
	assert.Contains(t, body, fmt.Sprintf(`"text":"%s"`, content))
}

// assertStopReason validates the stop_reason in message_delta.
func assertStopReason(t *testing.T, events []map[string]interface{}, expectedReason string) {
	t.Helper()
	messageDelta := getEventByType(events, "message_delta")
	require.NotNil(t, messageDelta, "message_delta event should exist")

	reason, ok := getStopReason(messageDelta)
	require.True(t, ok, "message_delta should have stop_reason")
	assert.Equal(t, expectedReason, reason, "stop_reason should match expected")
}

// assertModelInMessageStart validates that the model appears in message_start event.
func assertModelInMessageStart(t *testing.T, events []map[string]interface{}, expectedModel string) {
	t.Helper()
	messageStart := getEventByType(events, "message_start")
	require.NotNil(t, messageStart, "message_start event should exist")

	message, ok := messageStart["message"].(map[string]interface{})
	require.True(t, ok, "message_start should have message field")

	model, ok := message["model"].(string)
	require.True(t, ok, "message should have model field")
	assert.Equal(t, expectedModel, model, "model should match expected")
}

// assertUsageTokens validates token usage in message_delta event.
func assertUsageTokens(t *testing.T, events []map[string]interface{}, expectedInput, expectedOutput int) {
	t.Helper()
	messageDelta := getEventByType(events, "message_delta")
	require.NotNil(t, messageDelta, "message_delta event should exist")

	input, output, ok := getUsageTokens(messageDelta)
	require.True(t, ok, "message_delta should have usage with input/output tokens")
	assert.Equal(t, float64(expectedInput), input, "input_tokens should match")
	assert.Equal(t, float64(expectedOutput), output, "output_tokens should match")
}

// assertBlockTransitionOrder validates that text block stops before tool block starts.
// This is critical for proper content block lifecycle when transitioning types.
func assertBlockTransitionOrder(t *testing.T, events []map[string]interface{}) {
	t.Helper()

	// Find the text block stop and tool block start indices
	textStopIndex := -1
	toolStartIndex := -1

	for i, event := range events {
		if event["_event_type"] == "content_block_stop" {
			// First stop is typically the text block (index 0)
			if index, ok := getContentBlockIndex(event); ok && index == 0 && textStopIndex == -1 {
				textStopIndex = i
			}
		}
		if event["_event_type"] == "content_block_start" {
			blockType, ok := getContentBlockType(event)
			if ok && blockType == contentTypeToolUse && toolStartIndex == -1 {
				toolStartIndex = i
			}
		}
	}

	if textStopIndex > -1 && toolStartIndex > -1 {
		assert.Less(t, textStopIndex, toolStartIndex, "Text block should stop before tool block starts")
	}
}
