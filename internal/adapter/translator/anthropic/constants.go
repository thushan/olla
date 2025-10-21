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
	// defaultSessionID is used when no session ID is provided in the request headers
	// Prevents logging failures when X-Session-ID and X-Request-ID are both absent
	defaultSessionID = "default"
)
