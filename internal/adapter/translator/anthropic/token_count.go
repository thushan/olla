package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/translator"
)

// token estimation for claude code compatibility
func (t *Translator) CountTokens(ctx context.Context, r *http.Request) (*translator.TokenCountResponse, error) {
	// read and parse body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	// token counting using character estimation
	// simple algorithm: chars / 4
	tokenCount := estimateTokensFromRequest(&req)

	return &translator.TokenCountResponse{
		InputTokens:  tokenCount,
		OutputTokens: 0, // zero output tokens for count endpoint
		TotalTokens:  tokenCount,
	}, nil
}

// character-based token estimation
// simple algorithm from python reference: chars / 4
func estimateTokensFromRequest(req *AnthropicRequest) int {
	totalChars := 0

	// system prompt char counting
	// handles string and content block formats
	totalChars += countSystemChars(req.System)

	// count all message content
	for _, msg := range req.Messages {
		totalChars += countMessageChars(&msg)
	}

	tokenCount := totalChars / 4
	if tokenCount < 1 {
		tokenCount = 1
	}

	return tokenCount
}

// system prompt char counting
// handles string and content block formats
func countSystemChars(system interface{}) int {
	if system == nil {
		return 0
	}

	// string form handling
	if systemStr, ok := system.(string); ok {
		return len(systemStr)
	}

	// content block array handling
	if systemBlocks, ok := system.([]interface{}); ok {
		totalChars := 0
		for _, block := range systemBlocks {
			if blockMap, ok := block.(map[string]interface{}); ok {
				totalChars += countContentBlockChars(blockMap)
			}
		}
		return totalChars
	}

	return 0
}

// message char counting
// supports string and block arrays
func countMessageChars(msg *AnthropicMessage) int {
	totalChars := 0

	switch content := msg.Content.(type) {
	case string:
		// plain string content
		totalChars += len(content)

	case []interface{}:
		// untyped block arrays
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				totalChars += countContentBlockChars(blockMap)
			}
		}

	case []ContentBlock:
		// typed block arrays
		for _, block := range content {
			totalChars += countTypedContentBlockChars(&block)
		}
	}

	return totalChars
}

// untyped content block handling
// handles json block types
func countContentBlockChars(block map[string]interface{}) int {
	totalChars := 0

	blockType, _ := block["type"].(string)

	switch blockType {
	case contentTypeText:
		// text blocks have text field
		if text, ok := block["text"].(string); ok {
			totalChars += len(text)
		}

	case contentTypeToolResult:
		// tool results with content field
		if content, ok := block["content"].(string); ok {
			totalChars += len(content)
		} else if contentArray, ok := block["content"].([]interface{}); ok {
			// nested blocks in tool results
			for _, nestedBlock := range contentArray {
				if nestedMap, ok := nestedBlock.(map[string]interface{}); ok {
					totalChars += countContentBlockChars(nestedMap)
				}
			}
		}

	case contentTypeToolUse:
		// tool use blocks with name and input
		// count name in token estimate
		if name, ok := block["name"].(string); ok {
			totalChars += len(name)
		}
		// json input as serialized string
		if input, ok := block["input"].(map[string]interface{}); ok {
			if inputJSON, err := json.Marshal(input); err == nil {
				totalChars += len(inputJSON)
			}
		}
	}

	return totalChars
}

// typed content block handling
// handles typed contentblock structs
func countTypedContentBlockChars(block *ContentBlock) int {
	totalChars := 0

	switch block.Type {
	case contentTypeText:
		totalChars += len(block.Text)

	case contentTypeToolResult:
		// content as string or nested blocks
		if content, ok := block.Content.(string); ok {
			totalChars += len(content)
		}

	case contentTypeToolUse:
		totalChars += len(block.Name)
		// count input parameters
		if block.Input != nil {
			if inputJSON, err := json.Marshal(block.Input); err == nil {
				totalChars += len(inputJSON)
			}
		}
	}

	return totalChars
}
