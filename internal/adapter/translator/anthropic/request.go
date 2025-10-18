package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/adapter/translator"
)

// TransformRequest converts an Anthropic API request to OpenAI format
// Reads the request body, parses it and transforms messages, tools and parameters
func (t *Translator) TransformRequest(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
	// Limit request body size to prevent DoS attacks
	limitedBody := io.LimitReader(r.Body, maxAnthropicRequestSize)
	defer r.Body.Close()

	// Parse Anthropic request using decoder for better memory efficiency and strict validation
	var anthropicReq AnthropicRequest
	decoder := json.NewDecoder(limitedBody)
	decoder.DisallowUnknownFields() // Reject requests with unknown fields

	if err := decoder.Decode(&anthropicReq); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}

	// Validate required fields and parameter ranges
	if err := anthropicReq.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Re-marshal to get the body bytes for the original body
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Convert to OpenAI format
	openaiReq := make(map[string]interface{})

	// Basic parameters - direct mapping
	openaiReq["model"] = anthropicReq.Model
	openaiReq["max_tokens"] = anthropicReq.MaxTokens
	openaiReq["stream"] = anthropicReq.Stream

	// Optional parameters
	if anthropicReq.Temperature != nil {
		openaiReq["temperature"] = *anthropicReq.Temperature
	}
	if anthropicReq.TopP != nil {
		openaiReq["top_p"] = *anthropicReq.TopP
	}
	if len(anthropicReq.StopSequences) > 0 {
		openaiReq["stop"] = anthropicReq.StopSequences
	}

	// Convert messages (includes system prompt injection)
	openaiMessages, err := t.convertMessages(anthropicReq.Messages, anthropicReq.System)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}
	openaiReq["messages"] = openaiMessages

	// Convert tools if present
	if len(anthropicReq.Tools) > 0 {
		openaiTools := t.convertTools(anthropicReq.Tools)
		openaiReq["tools"] = openaiTools

		// Convert tool_choice
		if anthropicReq.ToolChoice != nil {
			openaiToolChoice, tcErr := t.convertToolChoice(anthropicReq.ToolChoice)
			if tcErr != nil {
				return nil, fmt.Errorf("failed to convert tool_choice: %w", tcErr)
			}
			openaiReq["tool_choice"] = openaiToolChoice
		}
	}

	t.logger.Debug("Transformed Anthropic request to OpenAI",
		"model", anthropicReq.Model,
		"message_count", len(anthropicReq.Messages),
		"has_tools", len(anthropicReq.Tools) > 0,
		"streaming", anthropicReq.Stream)

	return &translator.TransformedRequest{
		OpenAIRequest: openaiReq,
		OriginalBody:  body,
		ModelName:     anthropicReq.Model,
		IsStreaming:   anthropicReq.Stream,
		TargetPath:    "/v1/chat/completions", // Backend API endpoint (proxy layer handles /olla prefix)
		Metadata: map[string]interface{}{
			"format": "anthropic",
		},
	}, nil
}

// convertMessages transforms Anthropic messages to OpenAI format
// Injects system prompt as the first message if present
func (t *Translator) convertMessages(anthropicMessages []AnthropicMessage, systemPrompt interface{}) ([]map[string]interface{}, error) {
	// Pre-allocate with space for system message
	openaiMessages := make([]map[string]interface{}, 0, len(anthropicMessages)+1)

	// Add system message first if present
	// OpenAI expects system prompts as the first message with role="system"
	// System can be either a string or an array of content blocks
	if systemPrompt != nil {
		systemContent := t.convertSystemPrompt(systemPrompt)
		if systemContent != nil {
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":    "system",
				"content": systemContent,
			})
		}
	}

	// Convert each Anthropic message
	for _, msg := range anthropicMessages {
		converted, err := t.convertSingleMessage(msg)
		if err != nil {
			return nil, err
		}
		openaiMessages = append(openaiMessages, converted...)
	}

	return openaiMessages, nil
}

// convertSingleMessage converts one Anthropic message to OpenAI format
// May produce multiple OpenAI messages (e.g., user message + tool result messages)
func (t *Translator) convertSingleMessage(msg AnthropicMessage) ([]map[string]interface{}, error) {
	result := make([]map[string]interface{}, 0, 2)

	// Handle simple string content
	if contentStr, ok := msg.Content.(string); ok {
		if contentStr != "" {
			result = append(result, map[string]interface{}{
				"role":    msg.Role,
				"content": contentStr,
			})
		}
		return result, nil
	}

	// Handle array content (complex case with blocks)
	// Anthropic uses content blocks for rich messages (text, images, tools)
	contentBlocks, ok := msg.Content.([]interface{})
	if !ok {
		// Try to parse as JSON array if it's a single map
		if contentMap, ok := msg.Content.(map[string]interface{}); ok {
			contentBlocks = []interface{}{contentMap}
		} else {
			return nil, fmt.Errorf("invalid content type: %T", msg.Content)
		}
	}

	// Process based on role
	// User messages may contain text and tool_result blocks
	// Assistant messages may contain text and tool_use blocks
	if msg.Role == "user" {
		userMsg, toolMsgs := t.convertUserMessage(contentBlocks)
		if userMsg != nil {
			result = append(result, userMsg)
		}
		result = append(result, toolMsgs...)
	} else if msg.Role == "assistant" {
		assistantMsg := t.convertAssistantMessage(contentBlocks)
		if assistantMsg != nil {
			result = append(result, assistantMsg)
		}
	}

	return result, nil
}

// convertUserMessage processes user message content blocks
// Separates text content from tool_result blocks
// Returns a user message and separate tool messages (OpenAI requires separate messages for tool results)
func (t *Translator) convertUserMessage(blocks []interface{}) (map[string]interface{}, []map[string]interface{}) {
	var textParts []string
	var toolResults []map[string]interface{}

	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)
		switch blockType {
		case contentTypeText:
			if text, ok := blockMap["text"].(string); ok && text != "" {
				textParts = append(textParts, text)
			}
		case contentTypeToolResult:
			// Tool results become separate messages in OpenAI format
			// Map tool_use_id --> tool_call_id
			toolUseID, _ := blockMap["tool_use_id"].(string)

			// Content can be string or structured - convert to string
			content := ""
			if contentStr, ok := blockMap["content"].(string); ok {
				content = contentStr
			} else if contentObj := blockMap["content"]; contentObj != nil {
				// If content is structured, serialise to JSON
				if contentBytes, err := json.Marshal(contentObj); err == nil {
					content = string(contentBytes)
				}
			}

			toolResults = append(toolResults, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolUseID,
				"content":      content,
			})
		case contentTypeImage:
			// TODO: Phase 2 - Image support
			t.logger.Debug("Image content not yet supported in Phase 1")
		}
	}

	var userMsg map[string]interface{}
	if len(textParts) > 0 {
		userMsg = map[string]interface{}{
			"role":    "user",
			"content": strings.Join(textParts, ""),
		}
	}

	return userMsg, toolResults
}

// convertAssistantMessage processes assistant message content blocks
// Combines text content and tool_use blocks into a single OpenAI message
func (t *Translator) convertAssistantMessage(blocks []interface{}) map[string]interface{} {
	msg := map[string]interface{}{
		"role": "assistant",
	}

	var textContent string
	var toolCalls []map[string]interface{}

	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)
		switch blockType {
		case contentTypeText:
			if text, ok := blockMap["text"].(string); ok {
				textContent += text
			}
		case contentTypeToolUse:
			toolCall := t.convertToolUse(blockMap)
			if toolCall != nil {
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	// Set content - OpenAI expects null when only tool calls present
	if textContent != "" {
		msg["content"] = textContent
	} else if len(toolCalls) > 0 {
		msg["content"] = nil
	}

	// Set tool calls
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	return msg
}

// convertToolUse converts an Anthropic tool_use block to OpenAI tool_call format
// Maps tool IDs and serialises input parameters to JSON string
func (t *Translator) convertToolUse(block map[string]interface{}) map[string]interface{} {
	id, _ := block["id"].(string)
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]interface{})

	if id == "" || name == "" {
		return nil
	}

	// Convert input to JSON string (OpenAI expects arguments as JSON string)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.logger.Warn("Failed to marshal tool input", "error", err)
		inputJSON = []byte("{}")
	}

	return map[string]interface{}{
		"id":   id,
		"type": openAITypeFunction,
		"function": map[string]interface{}{
			"name":      name,
			"arguments": string(inputJSON),
		},
	}
}

// convertSystemPrompt converts Anthropic system prompt to OpenAI format
// Handles both string and array of content blocks formats
func (t *Translator) convertSystemPrompt(systemPrompt interface{}) interface{} {
	// Handle string form (simple case)
	if systemStr, ok := systemPrompt.(string); ok {
		if systemStr == "" {
			return nil
		}
		return systemStr
	}

	// Handle array form (content blocks)
	// Anthropic supports system prompts as arrays of content blocks
	if systemBlocks, ok := systemPrompt.([]interface{}); ok {
		var textParts []string
		for _, block := range systemBlocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, _ := blockMap["type"].(string)
			if blockType == contentTypeText {
				if text, ok := blockMap["text"].(string); ok && text != "" {
					textParts = append(textParts, text)
				}
			}
		}

		if len(textParts) > 0 {
			return strings.Join(textParts, "")
		}
	}

	// No valid content found
	return nil
}
