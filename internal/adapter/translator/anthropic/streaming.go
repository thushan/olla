package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/constants"
)

// StreamingState tracks the current streaming state
// Maintains message ID, content blocks being built, and buffers for partial data
type StreamingState struct {
	currentBlock     *ContentBlock
	toolCallBuffers  map[string]*strings.Builder // buffer for partial tool args
	messageID        string
	model            string
	lastFinishReason string // track the finish_reason from OpenAI
	contentBlocks    []ContentBlock
	currentIndex     int
	inputTokens      int
	outputTokens     int
	messageStartSent bool // track if message_start has been sent
}

// TransformStreamingResponse converts OpenAI SSE stream to Anthropic format
// Orchestrates the entire streaming process from OpenAI format to Anthropic SSE events
// Handles context cancellation and ensures proper cleanup
// Supports both synchronous (blocking Scanner) and asynchronous (non-blocking Reader) modes
func (t *Translator) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	// Set headers for SSE
	// Anthropic uses text/event-stream with no caching for real-time streaming
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Use http.ResponseController for modern flush handling (Go 1.20+)
	// Provides better error handling and cleaner API than type assertion
	rc := http.NewResponseController(w)

	state := &StreamingState{
		messageID:       t.generateMessageID(),
		contentBlocks:   make([]ContentBlock, 0, 4),
		toolCallBuffers: make(map[string]*strings.Builder),
	}

	// Choose streaming mode based on config
	// stream_async=false: synchronous Scanner (blocks, safer default for Claude Code agents)
	// stream_async=true: asynchronous Reader (allows parallel agent execution)
	var streamErr error
	if t.config.StreamAsync {
		streamErr = t.transformStreamingAsync(ctx, openaiStream, w, rc, state)
	} else {
		streamErr = t.transformStreamingSync(ctx, openaiStream, w, rc, state)
	}

	if streamErr != nil {
		return streamErr
	}

	// Ensure message_start is sent even if we had no chunks with model
	// This handles edge cases like empty streams
	if !state.messageStartSent {
		if err := t.writeEvent(w, "message_start", t.createMessageStart(state)); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return fmt.Errorf("flush failed: %w", err)
		}
		state.messageStartSent = true
	}

	// Send final events
	// Completes the stream with stop reason and final token counts
	if err := t.finalizeStream(state, w, rc); err != nil {
		return err
	}

	return nil
}

// transformStreamingSync processes the stream using synchronous Scanner
// This is the safer default that blocks until data is available
// Matches the original implementation behaviour
func (t *Translator) transformStreamingSync(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, rc *http.ResponseController, state *StreamingState) error {
	scanner := bufio.NewScanner(openaiStream)
	for scanner.Scan() {
		// Check for cancellation
		// Allows graceful shutdown if client disconnects
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if err := t.processStreamLine(line, state, w, rc); err != nil {
			t.logger.Error("Error processing stream line", "error", err)
			continue // Continue processing, don't fail entire stream
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}

// transformStreamingAsync processes the stream using asynchronous Reader
// This enables parallel agent execution in Claude Code by not blocking on reads
// Uses ReadString which will return immediately when newline is found
func (t *Translator) transformStreamingAsync(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, rc *http.ResponseController, state *StreamingState) error {
	reader := bufio.NewReader(openaiStream)

	for {
		// Check for cancellation before reading
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read until newline - non-blocking approach
		line, err := reader.ReadString('\n')

		// Process the line if we got any content
		if len(line) > 0 {
			// Trim the newline characters
			trimmedLine := strings.TrimRight(line, "\r\n")
			if trimmedLine != "" {
				if processErr := t.processStreamLine(trimmedLine, state, w, rc); processErr != nil {
					t.logger.Error("Error processing stream line", "error", processErr)
					// Continue processing, don't fail entire stream
				}
			}
		}

		// Handle read errors
		if err != nil {
			if err == io.EOF {
				// End of stream - normal completion
				break
			}
			// Other errors are fatal
			return fmt.Errorf("error reading stream: %w", err)
		}
	}

	return nil
}

// processStreamLine processes a single SSE line from OpenAI
// Parses the OpenAI chunk and routes to appropriate handlers for content or tool calls
func (t *Translator) processStreamLine(line string, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	// OpenAI format: "data: {...}"
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		// Log malformed chunks but don't fail the stream - graceful degradation
		// This allows partial responses to be delivered even if some chunks are corrupted
		t.logger.Warn("Malformed chunk encountered, skipping", "error", err, "data", data)
		return nil
	}

	// Extract model if not set
	// Model name is needed for the message_start event
	if state.model == "" {
		if model, ok := chunk["model"].(string); ok {
			state.model = model
		}
	}

	// Process choices
	choices, ok := chunk["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil
	}

	// Capture finish_reason when present
	// This is used to determine the final stop_reason in Anthropic format
	if finishReason, finishOk := choice["finish_reason"].(string); finishOk && finishReason != "" {
		state.lastFinishReason = finishReason
	}

	// Extract usage information if present in the chunk
	// OpenAI may include usage statistics at the chunk level, typically in the final chunk
	if usage, usageOk := chunk["usage"].(map[string]interface{}); usageOk {
		if promptTokens, promptOk := usage["prompt_tokens"].(float64); promptOk {
			state.inputTokens = int(promptTokens)
		}
		if completionTokens, completionsOk := usage["completion_tokens"].(float64); completionsOk {
			state.outputTokens = int(completionTokens)
		}
	}

	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Handle content delta
	if content, ok := delta["content"].(string); ok && content != "" {
		return t.handleContentDelta(content, state, w, rc)
	}

	// Handle tool calls delta
	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
		return t.handleToolCallsDelta(toolCalls, state, w, rc)
	}

	return nil
}

// ensureMessageStartSent sends message_start if not already sent
// This ensures message_start is always the first event before any content events
func (t *Translator) ensureMessageStartSent(state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	if !state.messageStartSent {
		if err := t.writeEvent(w, "message_start", t.createMessageStart(state)); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return fmt.Errorf("flush failed: %w", err)
		}
		state.messageStartSent = true
	}
	return nil
}

// handleContentDelta processes text content delta
// Starts a new content block if needed and sends appropriate Anthropic events
func (t *Translator) handleContentDelta(content string, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	// Ensure message_start is sent before any content events
	if err := t.ensureMessageStartSent(state, w, rc); err != nil {
		return err
	}

	// Start new content block if needed
	// Anthropic requires content_block_start before any deltas
	if state.currentBlock == nil || state.currentBlock.Type != contentTypeText {
		// Close previous block if it exists and is a different type
		if state.currentBlock != nil && state.currentBlock.Type != contentTypeText {
			if err := t.writeEvent(w, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": state.currentIndex,
			}); err != nil {
				return err
			}
			if err := rc.Flush(); err != nil {
				return fmt.Errorf("flush failed: %w", err)
			}
		}

		state.currentBlock = &ContentBlock{
			Type: contentTypeText,
			Text: "",
		}
		state.currentIndex = len(state.contentBlocks)
		state.contentBlocks = append(state.contentBlocks, *state.currentBlock)

		// Send content_block_start
		if err := t.writeEvent(w, "content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": state.currentIndex,
			"content_block": map[string]interface{}{
				"type": contentTypeText,
				"text": "",
			},
		}); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return fmt.Errorf("flush failed: %w", err)
		}
	}

	// Send content_block_delta
	// Each chunk of text is sent as a separate delta event for low latency
	if err := t.writeEvent(w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": state.currentIndex,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": content,
		},
	}); err != nil {
		return err
	}
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	// Update state
	// Track the accumulated text for debugging and validation
	state.currentBlock.Text += content
	state.contentBlocks[state.currentIndex] = *state.currentBlock

	return nil
}

// handleToolCallsDelta processes tool calls delta
// Buffers partial JSON arguments and sends Anthropic tool_use events
func (t *Translator) handleToolCallsDelta(toolCalls []interface{}, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	// Ensure message_start is sent before any content events
	if err := t.ensureMessageStartSent(state, w, rc); err != nil {
		return err
	}

	for _, tc := range toolCalls {
		toolCall, ok := tc.(map[string]interface{})
		if !ok {
			continue
		}

		index, _ := toolCall["index"].(float64)
		toolIndex := int(index)

		// Get or create tool call buffer
		// Buffer is needed because arguments stream in chunks and must be complete JSON
		toolID := fmt.Sprintf("tool_%d", toolIndex)
		if _, exists := state.toolCallBuffers[toolID]; !exists {
			state.toolCallBuffers[toolID] = &strings.Builder{}
		}

		function, ok := toolCall["function"].(map[string]interface{})
		if !ok {
			continue
		}

		// Handle tool call start
		// When we first see a tool call with ID and name, start the block
		if id, ok := toolCall["id"].(string); ok {
			if name, ok := function["name"].(string); ok {
				// Start new tool use block
				state.currentBlock = &ContentBlock{
					Type: contentTypeToolUse,
					ID:   id,
					Name: name,
				}
				state.currentIndex = len(state.contentBlocks)
				state.contentBlocks = append(state.contentBlocks, *state.currentBlock)

				// Send content_block_start
				if err := t.writeEvent(w, "content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": state.currentIndex,
					"content_block": map[string]interface{}{
						"type": contentTypeToolUse,
						"id":   id,
						"name": name,
					},
				}); err != nil {
					return err
				}
				if err := rc.Flush(); err != nil {
					return fmt.Errorf("flush failed: %w", err)
				}
			}
		}

		// Handle arguments delta
		// Arguments come as JSON string chunks that must be buffered and sent as partial_json
		if args, ok := function["arguments"].(string); ok && args != "" {
			state.toolCallBuffers[toolID].WriteString(args)

			// Send content_block_delta with partial JSON
			// Anthropic expects input_json_delta for streaming tool arguments
			if err := t.writeEvent(w, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": state.currentIndex,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": args,
				},
			}); err != nil {
				return err
			}
			if err := rc.Flush(); err != nil {
				return fmt.Errorf("flush failed: %w", err)
			}
		}
	}

	return nil
}

// finalizeStream sends final events and completes the stream
// Processes buffered tool arguments and determines stop_reason from OpenAI finish_reason
func (t *Translator) finalizeStream(state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	// Send content_block_stop for current block if active
	// Anthropic requires explicit stop event for each block
	if state.currentBlock != nil {
		if err := t.writeEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": state.currentIndex,
		}); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return fmt.Errorf("flush failed: %w", err)
		}
	}

	// Parse tool call buffers into input objects
	// Convert accumulated JSON strings into structured objects
	for toolID, builder := range state.toolCallBuffers {
		argsJSON := builder.String()
		if argsJSON != "" {
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(argsJSON), &input); err == nil {
				// Find the corresponding tool use block and update it
				// This updates the state for debugging/validation purposes
				for i := range state.contentBlocks {
					if state.contentBlocks[i].Type == contentTypeToolUse &&
						fmt.Sprintf("tool_%d", i) == toolID {
						state.contentBlocks[i].Input = input
						break
					}
				}
			}
		}
	}

	// Determine stop reason using the centralised mapping function
	// This ensures consistency with non-streaming responses
	stopReason := mapFinishReasonToStopReason(state.lastFinishReason)

	// Send message_delta
	// Contains the final stop_reason and cumulative token usage
	if err := t.writeEvent(w, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"input_tokens":  state.inputTokens,
			"output_tokens": state.outputTokens,
		},
	}); err != nil {
		return err
	}
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	// Send message_stop
	// Final event to indicate stream completion
	if err := t.writeEvent(w, "message_stop", map[string]interface{}{
		"type": "message_stop",
	}); err != nil {
		return err
	}
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	return nil
}

// createMessageStart creates the initial message_start event
// Contains the message metadata and initial (empty) content array
func (t *Translator) createMessageStart(state *StreamingState) map[string]interface{} {
	return map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":      state.messageID,
			"type":    "message",
			"role":    "assistant",
			"model":   state.model,
			"content": []interface{}{},
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": 0,
			},
		},
	}
}

// writeEvent writes an SSE event in Anthropic format
// Format: event: <name>\ndata: <json>\n\n
func (t *Translator) writeEvent(w http.ResponseWriter, event string, data interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, dataJSON); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}
