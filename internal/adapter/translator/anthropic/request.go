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

// convert anthropic format to openai, handles messages/tools/params
func (t *Translator) TransformRequest(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
	// limit request body to prevent DOS, uses configured max size
	limitedBody := io.LimitReader(r.Body, t.maxMessageSize)
	defer r.Body.Close()

	// use decoder for memory efficiency and strict validation
	var anthropicReq AnthropicRequest
	decoder := json.NewDecoder(limitedBody)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&anthropicReq); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}

	if err := anthropicReq.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// re-marshal to get body bytes for passthrough
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Log request to inspector if enabled
	if t.inspector.Enabled() {
		sessionID := t.getSessionID(r)
		if lerr := t.inspector.LogRequest(sessionID, anthropicReq.Model, body); lerr != nil {
			t.logger.Warn("Failed to log request to inspector", "error", lerr)
		}
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
		TargetPath:    "/v1/chat/completions",
		Metadata: map[string]interface{}{
			"format": "anthropic",
		},
	}, nil
}

// convert messages + inject system prompt if present
func (t *Translator) convertMessages(anthropicMessages []AnthropicMessage, systemPrompt interface{}) ([]map[string]interface{}, error) {
	openaiMessages := make([]map[string]interface{}, 0, len(anthropicMessages)+1)

	// openai wants system as first message, can be string or content blocks
	if systemPrompt != nil {
		systemContent := t.convertSystemPrompt(systemPrompt)
		if systemContent != nil {
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":    "system",
				"content": systemContent,
			})
		}
	}

	for _, msg := range anthropicMessages {
		converted, err := t.convertSingleMessage(msg)
		if err != nil {
			return nil, err
		}
		openaiMessages = append(openaiMessages, converted...)
	}

	return openaiMessages, nil
}

// convert single message, might produce multiple openai messages (eg user + tool results)
func (t *Translator) convertSingleMessage(msg AnthropicMessage) ([]map[string]interface{}, error) {
	result := make([]map[string]interface{}, 0, 2)

	if contentStr, ok := msg.Content.(string); ok {
		if contentStr != "" {
			result = append(result, map[string]interface{}{
				"role":    msg.Role,
				"content": contentStr,
			})
		}
		return result, nil
	}

	// anthropic uses content blocks for rich messages
	contentBlocks, ok := msg.Content.([]interface{})
	if !ok {
		if contentMap, ok := msg.Content.(map[string]interface{}); ok {
			contentBlocks = []interface{}{contentMap}
		} else {
			return nil, fmt.Errorf("invalid content type: %T", msg.Content)
		}
	}

	// user msgs can have text + tool results, assistant msgs have text + tool uses
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

// split user message into text + tool results (openai needs tool results as separate messages)
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
			// map tool_use_id to tool_call_id
			toolUseID, _ := blockMap["tool_use_id"].(string)

			// content can be string or structured, convert to string
			content := ""
			if contentStr, ok := blockMap["content"].(string); ok {
				content = contentStr
			} else if contentObj := blockMap["content"]; contentObj != nil {
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
			// TODO: image support later
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

// combine text + tool uses into single openai message
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

	// openai wants null content when only tool calls present
	if textContent != "" {
		msg["content"] = textContent
	} else if len(toolCalls) > 0 {
		msg["content"] = nil
	}

	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	return msg
}

// convert tool_use to openai tool_call format
func (t *Translator) convertToolUse(block map[string]interface{}) map[string]interface{} {
	id, _ := block["id"].(string)
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]interface{})

	if id == "" || name == "" {
		return nil
	}

	// openai wants args as json string
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

// convert system prompt, handles string or content blocks
func (t *Translator) convertSystemPrompt(systemPrompt interface{}) interface{} {
	// use the iterator to extract text from all content blocks
	var textParts []string

	// iterator handles all type conversions (string, []ContentBlock, *[]ContentBlock, []interface{})
	_ = t.forEachSystemContentBlock(systemPrompt, func(block ContentBlock) error {
		if block.Type == contentTypeText && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
		return nil
	})

	if len(textParts) == 0 {
		return nil
	}

	// join text blocks directly without separator
	return strings.Join(textParts, "")
}
