package anthropic

// Content type constants define the different types of content blocks
// in Anthropic's content block model for messages
const (
	// contentTypeText represents text content in message blocks
	contentTypeText = "text"

	// contentTypeToolUse represents tool usage (function calls) in message blocks
	contentTypeToolUse = "tool_use"

	// contentTypeToolResult represents tool results in message blocks
	contentTypeToolResult = "tool_result"

	// contentTypeImage represents image content in message blocks (Phase 2)
	contentTypeImage = "image"
)

// Tool choice constants define the different tool selection modes
// that control how the model decides whether and which tools to use
const (
	// toolChoiceAuto lets the model decide whether to use tools
	toolChoiceAuto = "auto"

	// toolChoiceAny forces the model to use at least one tool (maps to OpenAI's "required")
	toolChoiceAny = "any"

	// toolChoiceNone prevents the model from using any tools
	toolChoiceNone = "none"

	// toolChoiceTool forces selection of a specific tool
	toolChoiceTool = "tool"
)

// OpenAI-specific constants for tool choice mapping
const (
	// openAIToolChoiceRequired forces tool usage in OpenAI format
	// Maps from Anthropic's "any" tool choice
	openAIToolChoiceRequired = "required"

	// openAIToolChoiceAuto lets model decide in OpenAI format
	openAIToolChoiceAuto = "auto"

	// openAIToolChoiceNone prevents tool usage in OpenAI format
	openAIToolChoiceNone = "none"
)

// Function type constant for OpenAI tool definitions
const (
	// openAITypeFunction is the type identifier for OpenAI function tools
	openAITypeFunction = "function"
)
