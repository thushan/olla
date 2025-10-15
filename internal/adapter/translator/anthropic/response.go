package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// TransformResponse converts OpenAI response to Anthropic format
// Takes the OpenAI response structure and maps it to Anthropic's message format
// including content blocks, tool calls, and usage statistics
func (t *Translator) TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
	// Parse OpenAI response
	respMap, ok := openaiResp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid OpenAI response type: %T", openaiResp)
	}

	// Extract choices
	choices, ok := respMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no message in choice")
	}

	// Extract finish_reason from the OpenAI choice
	// This is critical for proper stop_reason mapping to Anthropic format
	finishReason := ""
	if fr, ok := choice["finish_reason"].(string); ok {
		finishReason = fr
	}

	// Build Anthropic response
	anthropicResp := AnthropicResponse{
		ID:    t.generateMessageID(),
		Type:  "message",
		Role:  "assistant",
		Model: t.extractModel(respMap),
	}

	// Convert content with finish_reason for proper stop_reason mapping
	content, stopReason := t.convertResponseContent(message, finishReason)
	anthropicResp.Content = content
	anthropicResp.StopReason = stopReason
	anthropicResp.StopSequence = nil

	// Convert usage
	if usage, ok := respMap["usage"].(map[string]interface{}); ok {
		anthropicResp.Usage = t.convertUsage(usage)
	}

	t.logger.Debug("Transformed OpenAI response to Anthropic",
		"content_blocks", len(content),
		"stop_reason", stopReason,
		"input_tokens", anthropicResp.Usage.InputTokens,
		"output_tokens", anthropicResp.Usage.OutputTokens)

	return anthropicResp, nil
}

// convertResponseContent converts message content and tool calls
// Processes both text content and tool_calls from OpenAI format to Anthropic content blocks
// The finish_reason from OpenAI is used to properly map to Anthropic's stop_reason
// Returns the content blocks and the appropriate stop reason
func (t *Translator) convertResponseContent(message map[string]interface{}, finishReason string) ([]ContentBlock, string) {
	var content []ContentBlock

	// Handle text content
	if textContent, ok := message["content"].(string); ok && textContent != "" {
		content = append(content, ContentBlock{
			Type: contentTypeText,
			Text: textContent,
		})
	}

	// Handle tool calls
	if toolCalls, ok := message["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			toolCall, ok := tc.(map[string]interface{})
			if !ok {
				continue
			}

			toolUse := t.convertToToolUse(toolCall)
			if toolUse != nil {
				content = append(content, *toolUse)
			}
		}
	}

	// If no content, add empty text block
	// Anthropic requires at least one content block in the response
	if len(content) == 0 {
		content = append(content, ContentBlock{
			Type: contentTypeText,
			Text: "",
		})
	}

	// Map OpenAI finish_reason to Anthropic stop_reason
	// This ensures proper termination signalling to clients
	stopReason := mapFinishReasonToStopReason(finishReason)

	return content, stopReason
}

// mapFinishReasonToStopReason converts OpenAI finish_reason to Anthropic stop_reason
// Centralised mapping used by both regular and streaming responses for consistency
func mapFinishReasonToStopReason(finishReason string) string {
	switch finishReason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return contentTypeToolUse
	case "length":
		return "max_tokens"
	default:
		// Default to end_turn for unknown or empty finish reasons
		return "end_turn"
	}
}

// convertToToolUse converts OpenAI tool_call to Anthropic tool_use
// Parses the JSON string arguments from OpenAI into a structured object for Anthropic
func (t *Translator) convertToToolUse(toolCall map[string]interface{}) *ContentBlock {
	id, _ := toolCall["id"].(string)
	function, ok := toolCall["function"].(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := function["name"].(string)
	argsStr, _ := function["arguments"].(string)

	// Parse arguments JSON string to object
	// OpenAI sends tool arguments as a JSON string, Anthropic expects a structured object
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(argsStr), &input); err != nil {
		t.logger.Warn("Failed to parse tool arguments", "error", err)
		input = make(map[string]interface{})
	}

	return &ContentBlock{
		Type:  contentTypeToolUse,
		ID:    id,
		Name:  name,
		Input: input,
	}
}

// convertUsage transforms OpenAI usage to Anthropic format
// Maps prompt_tokens to input_tokens and completion_tokens to output_tokens
func (t *Translator) convertUsage(usage map[string]interface{}) AnthropicUsage {
	promptTokens := 0
	completionTokens := 0

	if pt, ok := usage["prompt_tokens"].(float64); ok {
		promptTokens = int(pt)
	}
	if ct, ok := usage["completion_tokens"].(float64); ok {
		completionTokens = int(ct)
	}

	return AnthropicUsage{
		InputTokens:  promptTokens,
		OutputTokens: completionTokens,
	}
}

// extractModel gets model name from response
// Falls back to "unknown" if not present in the response
func (t *Translator) extractModel(resp map[string]interface{}) string {
	if model, ok := resp["model"].(string); ok {
		return model
	}
	return "unknown"
}

// generateMessageID creates a unique message ID
// Uses UUID v4 truncated to 16 characters to match Anthropic's ID format
func (t *Translator) generateMessageID() string {
	return fmt.Sprintf("msg_%s", uuid.New().String()[:16])
}
