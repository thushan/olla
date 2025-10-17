package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/translator"
)

// CountTokens implements the TokenCounter interface for Anthropic
// Provides token estimation for the /v1/messages/count_tokens endpoint
// This is critical for Claude Code compatibility which requires token counting
func (t *Translator) CountTokens(ctx context.Context, r *http.Request) (*translator.TokenCountResponse, error) {
	// Read and parse the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	// Count tokens using character-based estimation
	// Matches the Python reference implementation: chars / 4
	tokenCount := estimateTokensFromRequest(&req)

	return &translator.TokenCountResponse{
		InputTokens:  tokenCount,
		OutputTokens: 0, // Count endpoint only estimates input tokens
		TotalTokens:  tokenCount,
	}, nil
}

// estimateTokensFromRequest counts characters and estimates tokens
// Uses the simple algorithm from the Python reference: total_chars / 4
// This matches Anthropic's behaviour in the proxy reference implementation
func estimateTokensFromRequest(req *AnthropicRequest) int {
	totalChars := 0

	// Count system prompt characters
	if req.System != "" {
		totalChars += len(req.System)
	}

	// Count all message content
	for _, msg := range req.Messages {
		totalChars += countMessageChars(&msg)
	}

	// Use character-based estimation (chars / 4)
	// Ensure minimum of 1 token to avoid returning zero for empty requests
	tokenCount := totalChars / 4
	if tokenCount < 1 {
		tokenCount = 1
	}

	return tokenCount
}

// countMessageChars counts characters in a message
// Handles both string content and content block arrays
func countMessageChars(msg *AnthropicMessage) int {
	totalChars := 0

	switch content := msg.Content.(type) {
	case string:
		// Simple string content
		totalChars += len(content)

	case []interface{}:
		// Array of content blocks
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				totalChars += countContentBlockChars(blockMap)
			}
		}

	case []ContentBlock:
		// Typed content blocks
		for _, block := range content {
			totalChars += countTypedContentBlockChars(&block)
		}
	}

	return totalChars
}

// countContentBlockChars counts characters in an untyped content block map
// Handles the various content block types from JSON parsing
func countContentBlockChars(block map[string]interface{}) int {
	totalChars := 0

	blockType, _ := block["type"].(string)

	switch blockType {
	case contentTypeText:
		// Text blocks have a "text" field
		if text, ok := block["text"].(string); ok {
			totalChars += len(text)
		}

	case contentTypeToolResult:
		// Tool results have a "content" field (string or array)
		if content, ok := block["content"].(string); ok {
			totalChars += len(content)
		} else if contentArray, ok := block["content"].([]interface{}); ok {
			// Nested content blocks in tool results
			for _, nestedBlock := range contentArray {
				if nestedMap, ok := nestedBlock.(map[string]interface{}); ok {
					totalChars += countContentBlockChars(nestedMap)
				}
			}
		}

	case contentTypeToolUse:
		// Tool use blocks have "name" and "input" fields
		// Count the name as part of the token estimate
		if name, ok := block["name"].(string); ok {
			totalChars += len(name)
		}
		// Input is typically JSON, count it as serialized string
		if input, ok := block["input"].(map[string]interface{}); ok {
			if inputJSON, err := json.Marshal(input); err == nil {
				totalChars += len(inputJSON)
			}
		}
	}

	return totalChars
}

// countTypedContentBlockChars counts characters in a typed content block
// Handles strongly-typed ContentBlock structs
func countTypedContentBlockChars(block *ContentBlock) int {
	totalChars := 0

	switch block.Type {
	case contentTypeText:
		totalChars += len(block.Text)

	case contentTypeToolResult:
		// Content can be string or nested blocks
		if content, ok := block.Content.(string); ok {
			totalChars += len(content)
		}

	case contentTypeToolUse:
		totalChars += len(block.Name)
		// Count input parameters
		if block.Input != nil {
			if inputJSON, err := json.Marshal(block.Input); err == nil {
				totalChars += len(inputJSON)
			}
		}
	}

	return totalChars
}
