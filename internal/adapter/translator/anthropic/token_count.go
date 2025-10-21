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
	if system == nil {
		return 0
	}

	// string form handling
	if systemStr, ok := system.(string); ok {
		return len(systemStr)
	}

	// typed content block array handling
	if systemBlocks, ok := system.([]ContentBlock); ok {
		totalChars := 0
		for _, block := range systemBlocks {
			totalChars += countTypedContentBlockChars(&block)
		}
		return totalChars
	}

	// pointer to typed content block array handling
	if systemBlocksPtr, ok := system.(*[]ContentBlock); ok {
		if systemBlocksPtr != nil {
			totalChars := 0
			for _, block := range *systemBlocksPtr {
				totalChars += countTypedContentBlockChars(&block)
			}
			return totalChars
		}
		return 0
	}

	// untyped content block array handling
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
		} else if contentBlocks, ok := block.Content.([]ContentBlock); ok {
			// typed content block array
			for _, nestedBlock := range contentBlocks {
				totalChars += countTypedContentBlockChars(&nestedBlock)
			}
		} else if contentBlocksPtr, ok := block.Content.(*[]ContentBlock); ok {
			// pointer to typed content block array
			if contentBlocksPtr != nil {
				for _, nestedBlock := range *contentBlocksPtr {
					totalChars += countTypedContentBlockChars(&nestedBlock)
				}
			}
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
