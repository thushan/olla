package anthropic

import (
	"fmt"
)

// convertTools transforms Anthropic tool definitions to OpenAI function format
// Maps input_schema directly to parameters (both use JSON Schema)
func (t *Translator) convertTools(anthropicTools []AnthropicTool) ([]map[string]interface{}, error) {
	openaiTools := make([]map[string]interface{}, 0, len(anthropicTools))

	for _, tool := range anthropicTools {
		openaiTool := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema, // Direct mapping - both use JSON Schema
			},
		}
		openaiTools = append(openaiTools, openaiTool)
	}

	return openaiTools, nil
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
		case "auto":
			return "auto", nil
		case "any":
			// Anthropic's "any" means require a tool call
			// OpenAI uses "required" for this behaviour
			return "required", nil
		case "none":
			return "none", nil
		default:
			// Unknown string, default to auto
			return "auto", nil
		}
	}

	// Handle object form
	if choiceMap, ok := toolChoice.(map[string]interface{}); ok {
		choiceType, _ := choiceMap["type"].(string)

		switch choiceType {
		case "auto":
			return "auto", nil
		case "any":
			// Semantic mapping: Anthropic "any" -> OpenAI "required"
			return "required", nil
		case "tool":
			// Force specific tool selection
			toolName, ok := choiceMap["name"].(string)
			if !ok {
				return nil, fmt.Errorf("tool_choice type 'tool' requires 'name' field")
			}
			return map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": toolName,
				},
			}, nil
		default:
			// Unknown type, default to auto
			return "auto", nil
		}
	}

	// Safe default for unknown formats
	return "auto", nil
}
