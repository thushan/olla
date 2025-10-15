package anthropic

import (
	"fmt"
)

// convertTools transforms Anthropic tool definitions to OpenAI function format
// Maps input_schema directly to parameters (both use JSON Schema)
// Returns only the converted tools since this conversion cannot fail
func (t *Translator) convertTools(anthropicTools []AnthropicTool) []map[string]interface{} {
	openaiTools := make([]map[string]interface{}, 0, len(anthropicTools))

	for _, tool := range anthropicTools {
		openaiTool := map[string]interface{}{
			"type": openAITypeFunction,
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema, // Direct mapping - both use JSON Schema
			},
		}
		openaiTools = append(openaiTools, openaiTool)
	}

	return openaiTools
}

// convertToolChoice transforms Anthropic tool_choice to OpenAI format
// Handles both string and object forms with semantic mapping:
// - "auto" -> "auto" (let model decide)
// - "any" -> "required" (force tool use)
// - {"type": "tool", "name": "X"} -> {"type": "function", "function": {"name": "X"}}
func (t *Translator) convertToolChoice(toolChoice interface{}) (interface{}, error) {
	// Handle string form
	if choiceStr, ok := toolChoice.(string); ok {
		switch choiceStr {
		case toolChoiceAuto:
			return openAIToolChoiceAuto, nil
		case toolChoiceAny:
			// Anthropic's "any" means require a tool call
			// OpenAI uses "required" for this behaviour
			return openAIToolChoiceRequired, nil
		case toolChoiceNone:
			return openAIToolChoiceNone, nil
		default:
			// Unknown string, default to auto
			return openAIToolChoiceAuto, nil
		}
	}

	// Handle object form
	if choiceMap, ok := toolChoice.(map[string]interface{}); ok {
		choiceType, _ := choiceMap["type"].(string)

		switch choiceType {
		case toolChoiceAuto:
			return openAIToolChoiceAuto, nil
		case toolChoiceAny:
			// Semantic mapping: Anthropic "any" -> OpenAI "required"
			return openAIToolChoiceRequired, nil
		case toolChoiceTool:
			// Force specific tool selection
			toolName, ok := choiceMap["name"].(string)
			if !ok {
				return nil, fmt.Errorf("tool_choice type 'tool' requires 'name' field")
			}
			return map[string]interface{}{
				"type": openAITypeFunction,
				"function": map[string]interface{}{
					"name": toolName,
				},
			}, nil
		default:
			// Unknown type, default to auto
			return openAIToolChoiceAuto, nil
		}
	}

	// Safe default for unknown formats
	return openAIToolChoiceAuto, nil
}
