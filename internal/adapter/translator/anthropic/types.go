package anthropic

import "fmt"

// AnthropicRequest represents an Anthropic API request
// Maps to the Anthropic Messages API format
type AnthropicRequest struct {
	ToolChoice    interface{}            `json:"tool_choice,omitempty"` // string or object
	System        interface{}            `json:"system,omitempty"`      // string or []ContentBlock
	Thinking      interface{}            `json:"thinking,omitempty"`    // Extended thinking configuration
	Temperature   *float64               `json:"temperature,omitempty"`
	TopP          *float64               `json:"top_p,omitempty"`
	TopK          *int                   `json:"top_k,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Model         string                 `json:"model"`
	Messages      []AnthropicMessage     `json:"messages"`
	StopSequences []string               `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool        `json:"tools,omitempty"`
	MaxTokens     int                    `json:"max_tokens"`
	Stream        bool                   `json:"stream,omitempty"`
}

// Validate checks that required fields are present and all fields have valid values
// Returns an error if validation fails with a descriptive message
func (r *AnthropicRequest) Validate() error {
	// Validate required fields
	if r.Model == "" {
		return fmt.Errorf("model field is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}

	// Validate optional fields have acceptable values
	if r.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative, got %d", r.MaxTokens)
	}
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2, got %f", *r.Temperature)
	}
	if r.TopP != nil && (*r.TopP < 0 || *r.TopP > 1) {
		return fmt.Errorf("top_p must be between 0 and 1, got %f", *r.TopP)
	}
	if r.TopK != nil && *r.TopK < 0 {
		return fmt.Errorf("top_k must be non-negative, got %d", *r.TopK)
	}

	return nil
}

// AnthropicMessage represents a message in the conversation
// Content can be either a simple string or an array of content blocks
type AnthropicMessage struct {
	Content interface{} `json:"content"` // string or []ContentBlock
	Role    string      `json:"role"`    // "user" or "assistant"
}

// ContentBlock represents different types of content in messages
// Anthropic uses a block-based content model for text, images, tool use, and tool results
type ContentBlock struct {
	Content   interface{}            `json:"content,omitempty"` // for tool_result
	Source    *ImageSource           `json:"source,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Type      string                 `json:"type"` // "text", "image", "tool_use", "tool_result"
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
}

// ImageSource represents image data in content blocks
// Supports both base64-encoded data and URLs
type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// AnthropicTool represents a tool definition
// Tools enable the model to call external functions
type AnthropicTool struct {
	InputSchema map[string]interface{} `json:"input_schema"` // JSON Schema for tool parameters
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
}

// ToolChoiceAuto represents automatic tool selection
type ToolChoiceAuto struct {
	Type string `json:"type"` // "auto"
}

// ToolChoiceAny represents required tool use (Anthropic's "any")
type ToolChoiceAny struct {
	Type string `json:"type"` // "any"
}

// ToolChoiceTool represents forced selection of a specific tool
type ToolChoiceTool struct {
	Type string `json:"type"` // "tool"
	Name string `json:"name"`
}

// AnthropicResponse represents an Anthropic API response
// Contains the assistant's reply with content blocks and usage stats
type AnthropicResponse struct {
	StopSequence *string        `json:"stop_sequence"`
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason,omitempty"` // "end_turn", "max_tokens", "tool_use"
	Content      []ContentBlock `json:"content"`
	Usage        AnthropicUsage `json:"usage"`
}

// AnthropicUsage represents token usage in Anthropic format
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent types for Anthropic SSE streaming

// MessageStart represents the start of a streaming message
type MessageStart struct {
	Type    string             `json:"type"` // "message_start"
	Message MessageStartDetail `json:"message"`
}

// MessageStartDetail contains initial message metadata
type MessageStartDetail struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"` // "message"
	Role    string         `json:"role"` // "assistant"
	Model   string         `json:"model"`
	Content []ContentBlock `json:"content"`
	Usage   AnthropicUsage `json:"usage"`
}

// ContentBlockStart represents the start of a content block
type ContentBlockStart struct {
	ContentBlock ContentBlock `json:"content_block"`
	Type         string       `json:"type"` // "content_block_start"
	Index        int          `json:"index"`
}

// ContentBlockDelta represents incremental content updates
type ContentBlockDelta struct {
	Delta interface{} `json:"delta"` // TextDelta or InputJSONDelta
	Type  string      `json:"type"`  // "content_block_delta"
	Index int         `json:"index"`
}

// TextDelta represents incremental text updates
type TextDelta struct {
	Type string `json:"type"` // "text_delta"
	Text string `json:"text"`
}

// InputJSONDelta represents incremental tool input JSON
type InputJSONDelta struct {
	Type        string `json:"type"` // "input_json_delta"
	PartialJSON string `json:"partial_json"`
}

// ContentBlockStop marks the end of a content block
type ContentBlockStop struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

// MessageDelta represents message-level updates (stop reason, final tokens)
type MessageDelta struct {
	Delta MessageDeltaContent `json:"delta"`
	Type  string              `json:"type"` // "message_delta"
	Usage AnthropicUsage      `json:"usage"`
}

// MessageDeltaContent contains stop reason information
type MessageDeltaContent struct {
	StopSequence *string `json:"stop_sequence"`
	StopReason   string  `json:"stop_reason,omitempty"`
}

// MessageStop marks the end of the streaming message
type MessageStop struct {
	Type string `json:"type"` // "message_stop"
}
