package anthropic

const (
	contentTypeText       = "text"
	contentTypeToolUse    = "tool_use"
	contentTypeToolResult = "tool_result"
	contentTypeImage      = "image"
)

const (
	toolChoiceAuto = "auto"
	toolChoiceAny  = "any"
	toolChoiceNone = "none"
	toolChoiceTool = "tool"
)

const (
	openAIToolChoiceRequired = "required"
	openAIToolChoiceAuto     = "auto"
	openAIToolChoiceNone     = "none"
)

const (
	openAITypeFunction = "function"
)

const (
	// [TF] added to avoid memory exhaustion for some massive requests
	maxAnthropicRequestSize = 10 << 20 // 10MB, TODO: make configurable
)
