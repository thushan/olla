package anthropic

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/thushan/olla/internal/util"
)

// convert openai response to anthropic format
// maps choices/messages to content blocks, handles tools and token usage
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

	// grab finish_reason for stop_reason mapping
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

	// convert content and figure out stop_reason
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

	// Log response to inspector if enabled
	if t.inspector.Enabled() {
		sessionID := original.Header.Get(t.inspector.GetSessionHeader())
		if sessionID == "" {
			sessionID = original.Header.Get("X-Request-ID")
			if sessionID == "" {
				sessionID = "default"
			}
		}
		if respBytes, err := json.Marshal(anthropicResp); err == nil {
			if err := t.inspector.LogResponse(sessionID, respBytes); err != nil {
				t.logger.Warn("Failed to log response to inspector", "error", err)
			}
		}
	}

	return anthropicResp, nil
}

// parse text and tool_calls from openai message into content blocks
// finish_reason determines what stop_reason to use
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

	// anthropic needs at least one block, even if empty
	if len(content) == 0 {
		content = append(content, ContentBlock{
			Type: contentTypeText,
			Text: "",
		})
	}

	stopReason := mapFinishReasonToStopReason(finishReason)

	return content, stopReason
}

// map openai finish_reason to anthropic stop_reason
// shared between normal and streaming paths
func mapFinishReasonToStopReason(finishReason string) string {
	switch finishReason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return contentTypeToolUse
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}

// convert openai tool_call to anthropic tool_use block
// parses json args string into an object, logs errors but doesn't fail
func (t *Translator) convertToToolUse(toolCall map[string]interface{}) *ContentBlock {
	id, _ := toolCall["id"].(string)
	function, ok := toolCall["function"].(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := function["name"].(string)
	argsStr, _ := function["arguments"].(string)

	// openai sends args as json string, we need it as an object
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(argsStr), &input); err != nil {
		// use empty input if json is bad, don't fail the whole response
		t.logger.Warn("Failed to parse tool arguments, using empty input",
			"tool", name,
			"tool_id", id,
			"error", err,
			"raw_arguments", util.TruncateString(argsStr, util.DefaultTruncateLengthPII),
			"raw_arguments_len", len(argsStr))
		input = make(map[string]interface{})
	}

	return &ContentBlock{
		Type:  contentTypeToolUse,
		ID:    id,
		Name:  name,
		Input: input,
	}
}

// map openai token counts to anthropic names
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

// grab model name from response, default to "unknown"
func (t *Translator) extractModel(resp map[string]interface{}) string {
	if model, ok := resp["model"].(string); ok {
		return model
	}
	return "unknown"
}

// base58 charset, skips confusing chars like 0/O and I/l
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// generate msg_01... ids like anthropic does
// 16 random bytes encoded as base58 gives ~27-29 char ids
func (t *Translator) generateMessageID() string {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		// crypto/rand shouldn't fail but just in case
		t.logger.Warn("Failed to generate random bytes for message ID", "error", err)
		return fmt.Sprintf("msg_01fallback%d", big.NewInt(0).SetBytes(randomBytes[:8]).Uint64())
	}

	encoded := encodeBase58(randomBytes)
	return fmt.Sprintf("msg_01%s", encoded)
}

// encode bytes to base58, shorter and less ambigious than hex
func encodeBase58(input []byte) string {
	num := new(big.Int).SetBytes(input)

	if num.Sign() == 0 {
		return string(base58Alphabet[0])
	}
	var encoded []byte
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	for num.Cmp(zero) > 0 {
		num.DivMod(num, base, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}

	// preserve leading zeros as '1' chars
	for _, b := range input {
		if b == 0 {
			encoded = append(encoded, base58Alphabet[0])
		} else {
			break
		}
	}

	// reverse since we built it backwards
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	return string(encoded)
}
