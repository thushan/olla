package anthropic

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
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
// If JSON parsing fails, logs the error and uses empty input for graceful degradation
// Returns (contentBlock, nil) for successful/graceful cases, or (nil, nil) for malformed tool calls
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
		// Log detailed error for debugging but use empty input for graceful degradation
		// This prevents a single malformed tool call from breaking the entire response
		t.logger.Warn("Failed to parse tool arguments, using empty input",
			"tool", name,
			"tool_id", id,
			"error", err,
			"raw_arguments", argsStr)
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

// base58Alphabet is the character set used for base58 encoding
// Excludes visually similar characters (0, O, I, l) to reduce transcription errors
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// generateMessageID creates a unique message ID matching Anthropic's format
// Anthropic uses the format "msg_01" followed by base58-encoded random bytes
// resulting in IDs like "msg_01XYZ..." with total length around 27-29 characters
func (t *Translator) generateMessageID() string {
	// Generate 16 random bytes for the ID suffix
	// This provides 128 bits of entropy, ensuring uniqueness
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to a simpler format if crypto/rand fails
		// This should never happen in practice but provides safety
		t.logger.Warn("Failed to generate random bytes for message ID", "error", err)
		return fmt.Sprintf("msg_01fallback%d", big.NewInt(0).SetBytes(randomBytes[:8]).Uint64())
	}

	// Encode to base58 for a compact, human-friendly representation
	encoded := encodeBase58(randomBytes)

	// Anthropic's format starts with "msg_01" prefix
	return fmt.Sprintf("msg_01%s", encoded)
}

// encodeBase58 converts bytes to base58 string
// Base58 encoding produces shorter, more readable IDs than hex or base64
// and avoids ambiguous characters
func encodeBase58(input []byte) string {
	// Convert bytes to a big integer
	num := new(big.Int).SetBytes(input)

	// Handle zero case
	if num.Sign() == 0 {
		return string(base58Alphabet[0])
	}

	// Encode using base58
	var encoded []byte
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	for num.Cmp(zero) > 0 {
		num.DivMod(num, base, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}

	// Add leading '1' for each leading zero byte
	// This preserves the length information from leading zeros
	for _, b := range input {
		if b == 0 {
			encoded = append(encoded, base58Alphabet[0])
		} else {
			break
		}
	}

	// Reverse the result (base58 is big-endian)
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	return string(encoded)
}
