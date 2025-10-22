package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/adapter/translator"
)

// token estimation for claude code compatibility
func (t *Translator) CountTokens(ctx context.Context, r *http.Request) (*translator.TokenCountResponse, error) {
	// bounded read to prevent OOM attacks
	// read up to maxMessageSize + 1 to detect oversized requests
	limitedReader := io.LimitReader(r.Body, t.maxMessageSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		// close the original body even on error
		_ = r.Body.Close()
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// detect oversized requests
	if int64(len(body)) > t.maxMessageSize {
		_ = r.Body.Close()
		return nil, fmt.Errorf("request body exceeds maximum size of %d bytes", t.maxMessageSize)
	}

	// reset body for downstream handlers to re-read
	r.Body = io.NopCloser(bytes.NewReader(body))

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
	totalChars := 0

	_ = forEachSystemBlock(system, func(block ContentBlock) error {
		if block.Type == contentTypeText {
			totalChars += len(block.Text)
		}
		// for other block types, convert to typed block and count
		if block.Type != "" && block.Type != contentTypeText {
			totalChars += countContentBlockChars(&block)
		}
		return nil
	})

	return totalChars
}

// forEachSystemBlock iterates over system prompt content blocks regardless of input format.
// This is a standalone version of the iterator that doesn't require a Translator instance.
// Handles: string, []ContentBlock, *[]ContentBlock, []interface{}
func forEachSystemBlock(system interface{}, fn func(block ContentBlock) error) error {
	if system == nil {
		return nil
	}

	if systemStr, ok := system.(string); ok {
		if systemStr != "" {
			return fn(ContentBlock{
				Type: contentTypeText,
				Text: systemStr,
			})
		}
		return nil
	}

	if systemBlocks, ok := system.([]ContentBlock); ok {
		for _, block := range systemBlocks {
			if err := fn(block); err != nil {
				return err
			}
		}
		return nil
	}

	if systemBlocksPtr, ok := system.(*[]ContentBlock); ok {
		if systemBlocksPtr != nil {
			for _, block := range *systemBlocksPtr {
				if err := fn(block); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if systemBlocks, ok := system.([]interface{}); ok {
		for _, block := range systemBlocks {
			if normalised, nok := normaliseContentBlock(block); nok {
				if err := fn(*normalised); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return nil
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
		// untyped block arrays - normalise then count
		for _, block := range content {
			if normalised, ok := normaliseContentBlock(block); ok {
				totalChars += countContentBlockChars(normalised)
			}
		}

	case []ContentBlock:
		// typed block arrays
		for i := range content {
			totalChars += countContentBlockChars(&content[i])
		}
	}

	return totalChars
}

// normaliseContentBlock converts various content block representations to a typed ContentBlock.
// This centralises the type conversion logic so character counting can work with a single type.
// Returns the normalised block and true on success, or an empty block and false on failure.
func normaliseContentBlock(block interface{}) (*ContentBlock, bool) {
	// already a typed ContentBlock pointer, return as-is
	if typedBlock, ok := block.(*ContentBlock); ok {
		return typedBlock, true
	}

	// already a typed ContentBlock value, return pointer
	if typedBlock, ok := block.(ContentBlock); ok {
		return &typedBlock, true
	}

	// untyped map needs conversion to ContentBlock
	blockMap, ok := block.(map[string]interface{})
	if !ok {
		return nil, false
	}

	// extract common fields from the map
	normalised := &ContentBlock{
		Type: extractString(blockMap, "type"),
		Text: extractString(blockMap, "text"),
		Name: extractString(blockMap, "name"),
	}

	// extract content field (can be string or nested structure)
	if content, exists := blockMap["content"]; exists {
		normalised.Content = content
	}

	// extract input field for tool_use blocks
	if input, exists := blockMap["input"]; exists {
		if inputMap, iok := input.(map[string]interface{}); iok {
			normalised.Input = inputMap
		}
	}

	return normalised, true
}

// extractString safely extracts a string value from a map, returning empty string if not found or wrong type
func extractString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// countContentBlockChars calculates character count for a single content block.
// This is the single source of truth for character counting logic across all content block types.
// Handles text, tool_use, tool_result, and image blocks consistently.
func countContentBlockChars(block *ContentBlock) int {
	if block == nil {
		return 0
	}

	switch block.Type {
	case contentTypeText:
		// text blocks contribute their text length
		return len(block.Text)

	case contentTypeToolResult:
		// tool results can have content as string or nested blocks
		return countToolResultContent(block.Content)

	case contentTypeToolUse:
		// tool use blocks contribute name length plus serialised input
		return len(block.Name) + countToolInput(block.Input)

	case contentTypeImage:
		// image blocks don't contribute to character count
		return 0

	default:
		return 0
	}
}

// countToolResultContent handles the various content formats in tool_result blocks.
// Content can be a string, []interface{}, []ContentBlock, or *[]ContentBlock.
func countToolResultContent(content interface{}) int {
	if content == nil {
		return 0
	}

	if contentStr, ok := content.(string); ok {
		return len(contentStr)
	}

	// untyped block array
	if contentArray, ok := content.([]interface{}); ok {
		totalChars := 0
		for _, nestedBlock := range contentArray {
			if normalized, ok := normaliseContentBlock(nestedBlock); ok {
				totalChars += countContentBlockChars(normalized)
			}
		}
		return totalChars
	}

	// typed block array
	if contentBlocks, ok := content.([]ContentBlock); ok {
		totalChars := 0
		for i := range contentBlocks {
			totalChars += countContentBlockChars(&contentBlocks[i])
		}
		return totalChars
	}

	// pointer to typed block array
	if contentBlocksPtr, ok := content.(*[]ContentBlock); ok {
		if contentBlocksPtr != nil {
			totalChars := 0
			for i := range *contentBlocksPtr {
				totalChars += countContentBlockChars(&(*contentBlocksPtr)[i])
			}
			return totalChars
		}
	}

	return 0
}

// countToolInput counts characters in tool input by marshalling to JSON.
// Returns 0 if input is nil or marshalling fails.
func countToolInput(input map[string]interface{}) int {
	if input == nil {
		return 0
	}

	if inputJSON, err := json.Marshal(input); err == nil {
		return len(inputJSON)
	}

	return 0
}
