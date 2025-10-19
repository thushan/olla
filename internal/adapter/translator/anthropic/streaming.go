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

// tracks state while streaming - buffers partial data, blocks in progress
type StreamingState struct {
	currentBlock     *ContentBlock
	toolCallBuffers  map[string]*strings.Builder
	messageID        string
	model            string
	lastFinishReason string
	contentBlocks    []ContentBlock
	currentIndex     int
	inputTokens      int
	outputTokens     int
	messageStartSent bool
}

// convert openai sse stream to anthropic format
func (t *Translator) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	// text/event-stream, no caching
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)

	state := &StreamingState{
		messageID:       t.generateMessageID(),
		contentBlocks:   make([]ContentBlock, 0, 4),
		toolCallBuffers: make(map[string]*strings.Builder),
	}

	// sync streaming for now (async needs more work for agent workflows)
	streamErr := t.transformStreamingSync(ctx, openaiStream, w, rc, state)

	if streamErr != nil {
		return streamErr
	}

	// send message_start even if stream was empty
	if !state.messageStartSent {
		if err := t.writeEvent(w, "message_start", t.createMessageStart(state)); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return fmt.Errorf("flush failed: %w", err)
		}
		state.messageStartSent = true
	}

	// send final events (stop reason + token counts)
	if err := t.finalizeStream(state, w, rc); err != nil {
		return err
	}

	return nil
}

// process stream using blocking scanner, safer and simpler
func (t *Translator) transformStreamingSync(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, rc *http.ResponseController, state *StreamingState) error {
	scanner := bufio.NewScanner(openaiStream)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if err := t.processStreamLine(line, state, w, rc); err != nil {
			t.logger.Error("Error processing stream line", "error", err)
			continue // keep going, don't fail entire stream on one bad line
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}

// process single sse line from openai, route to content or tool handlers
func (t *Translator) processStreamLine(line string, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		// log bad chunks but keep going, partial responses better than nothing
		t.logger.Warn("Malformed chunk encountered, skipping", "error", err, "data", data)
		return nil
	}

	// grab model name for message_start event
	if state.model == "" {
		if model, ok := chunk["model"].(string); ok {
			state.model = model
		}
	}

	choices, ok := chunk["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil
	}

	// capture finish_reason for later stop_reason mapping
	if finishReason, finishOk := choice["finish_reason"].(string); finishOk && finishReason != "" {
		state.lastFinishReason = finishReason
	}

	// grab usage stats if present (usually in final chunk)
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

	if content, ok := delta["content"].(string); ok && content != "" {
		return t.handleContentDelta(content, state, w, rc)
	}

	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
		return t.handleToolCallsDelta(toolCalls, state, w, rc)
	}

	return nil
}

// send message_start if we haven't already, needs to be first event
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

// process text delta, starts new block if needed
func (t *Translator) handleContentDelta(content string, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	if err := t.ensureMessageStartSent(state, w, rc); err != nil {
		return err
	}

	// start new text block if needed (anthropic wants block_start before deltas)
	if state.currentBlock == nil || state.currentBlock.Type != contentTypeText {
		// close previous block if different type
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

	// send delta event for each chunk
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

	// track accumulated text
	state.currentBlock.Text += content
	state.contentBlocks[state.currentIndex] = *state.currentBlock

	return nil
}

// process tool call deltas, buffers partial json args
func (t *Translator) handleToolCallsDelta(toolCalls []interface{}, state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
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

		// need buffer since args come in chunks but need complete json eventually
		toolID := fmt.Sprintf("tool_%d", toolIndex)
		if _, exists := state.toolCallBuffers[toolID]; !exists {
			state.toolCallBuffers[toolID] = &strings.Builder{}
		}

		function, ok := toolCall["function"].(map[string]interface{})
		if !ok {
			continue
		}

		// start block when we get id + name
		if id, ok := toolCall["id"].(string); ok {
			if name, ok := function["name"].(string); ok {
				state.currentBlock = &ContentBlock{
					Type: contentTypeToolUse,
					ID:   id,
					Name: name,
				}
				state.currentIndex = len(state.contentBlocks)
				state.contentBlocks = append(state.contentBlocks, *state.currentBlock)

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

		// buffer args chunks and send as partial_json
		if args, ok := function["arguments"].(string); ok && args != "" {
			state.toolCallBuffers[toolID].WriteString(args)

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

// send final events, parse tool buffers, determine stop_reason
func (t *Translator) finalizeStream(state *StreamingState, w http.ResponseWriter, rc *http.ResponseController) error {
	// close current block if still open
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

	// parse buffered json args into objects
	for toolID, builder := range state.toolCallBuffers {
		argsJSON := builder.String()
		if argsJSON != "" {
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(argsJSON), &input); err == nil {
				// find matching tool block and update it
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

	// map finish_reason to stop_reason (same logic as non-streaming)
	stopReason := mapFinishReasonToStopReason(state.lastFinishReason)

	// send delta with stop_reason + usage
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

	// final event
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

// create initial message_start event with metadata
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

// write sse event: event: <name>\ndata: <json>\n\n
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
