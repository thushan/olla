package inspector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

const (
	BodyInspectorName = "body"
	MaxBodySize       = 1024 * 1024 // 1MB max body size for inspection
)

type modelRequest struct {
	Model string `json:"model"`
}

// BodyInspector extracts model names from request bodies
type BodyInspector struct {
	logger      logger.StyledLogger
	bufferPool  *pool.Pool[*bytes.Buffer]
	maxBodySize int64
}

func NewBodyInspector(logger logger.StyledLogger) (*BodyInspector, error) {
	bufPool, err := pool.NewLitePool(func() *bytes.Buffer {
		return new(bytes.Buffer)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer pool: %w", err)
	}

	return &BodyInspector{
		logger:      logger,
		maxBodySize: MaxBodySize,
		bufferPool:  bufPool,
	}, nil
}

func (bi *BodyInspector) Name() string {
	return BodyInspectorName
}

func (bi *BodyInspector) Inspect(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		bi.logger.Debug("Skipping body inspection for non-JSON content", "content_type", contentType)
		return nil
	}

	if r.ContentLength > bi.maxBodySize {
		bi.logger.Debug("Skipping body inspection for large request", "content_length", r.ContentLength)
		return nil
	}

	buffer := bi.bufferPool.Get()
	defer func() {
		buffer.Reset()
		bi.bufferPool.Put(buffer)
	}()

	limitedReader := io.LimitReader(r.Body, bi.maxBodySize)

	if _, err := io.Copy(buffer, limitedReader); err != nil {
		bi.logger.Debug("Failed to read request body", "error", err)
		return nil
	}

	// Restore the body for downstream handlers by creating a new reader that combines
	// what we've already read with any remaining unread content
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buffer.Bytes()), r.Body))

	modelName := bi.extractModelName(buffer.Bytes())
	if modelName != "" {
		profile.ModelName = modelName
		bi.logger.Debug("Extracted model name from request body", "model", modelName)
	}

	// Detect required capabilities from the request
	capabilities := bi.detectRequiredCapabilities(buffer.Bytes())
	if capabilities != nil {
		// Only set capabilities if they require special features beyond basic chat
		if capabilities.VisionUnderstanding || capabilities.FunctionCalling ||
			capabilities.Embeddings || capabilities.CodeGeneration {
			profile.ModelCapabilities = capabilities
			bi.logger.Debug("Detected required capabilities from request",
				"vision", capabilities.VisionUnderstanding,
				"functions", capabilities.FunctionCalling,
				"embeddings", capabilities.Embeddings,
				"code", capabilities.CodeGeneration,
				"streaming", capabilities.StreamingSupport)
		}
	}

	return nil
}

func (bi *BodyInspector) extractModelName(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var req modelRequest
	if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
		return bi.normalizeModelName(req.Model)
	}

	// Fall back to flexible map-based extraction to handle non-standard formats
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	for key, value := range data {
		if strings.EqualFold(key, "model") {
			if modelStr, ok := value.(string); ok && modelStr != "" {
				return bi.normalizeModelName(modelStr)
			}
		}
	}

	// Some APIs specify model per-message rather than at the request level
	if messages, ok := data["messages"].([]interface{}); ok && len(messages) > 0 {
		if firstMsg, ok := messages[0].(map[string]interface{}); ok {
			if model, ok := firstMsg["model"].(string); ok && model != "" {
				return bi.normalizeModelName(model)
			}
		}
	}

	return ""
}

func (bi *BodyInspector) normalizeModelName(model string) string {
	model = strings.TrimSpace(model)
	model = strings.ToLower(model)
	// Model aliasing and tag handling is delegated to the registry layer
	return model
}

// detectRequiredCapabilities analyzes the request body to determine what capabilities are needed
func (bi *BodyInspector) detectRequiredCapabilities(body []byte) *domain.ModelCapabilities {
	if len(body) == 0 {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	caps := &domain.ModelCapabilities{
		// Default capabilities - most models support these
		ChatCompletion:   true,
		TextGeneration:   true,
		StreamingSupport: true,
	}

	// Check for streaming preference
	if stream, ok := data["stream"].(bool); ok {
		caps.StreamingSupport = stream
	}

	// Check for function/tool calling
	if tools, ok := data["tools"]; ok && tools != nil {
		caps.FunctionCalling = true
	}
	if functions, ok := data["functions"]; ok && functions != nil {
		caps.FunctionCalling = true
	}
	if toolChoice, ok := data["tool_choice"]; ok && toolChoice != nil {
		caps.FunctionCalling = true
	}
	if functionCall, ok := data["function_call"]; ok && functionCall != nil {
		caps.FunctionCalling = true
	}

	// Check for vision requirements in messages
	if messages, ok := data["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				// Check content for vision elements
				if content, ok := msgMap["content"]; ok {
					if bi.hasVisionContent(content) {
						caps.VisionUnderstanding = true
					}
				}
			}
		}
	}

	// Check for embeddings endpoint
	if input, ok := data["input"]; ok && input != nil {
		// If there's an "input" field, this is likely an embeddings request
		caps.Embeddings = true
		caps.ChatCompletion = false
		caps.TextGeneration = false
	}

	// Check for code generation hints
	if bi.hasCodeGenerationHints(data) {
		caps.CodeGeneration = true
	}

	return caps
}

// hasVisionContent checks if the content contains image data
func (bi *BodyInspector) hasVisionContent(content interface{}) bool {
	switch v := content.(type) {
	case string:
		// Simple text content, no vision
		return false
	case []interface{}:
		// Multi-modal content array
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)
				if itemType == "image_url" || itemType == "image" {
					return true
				}
				// Check for base64 image data
				if itemType == "text" {
					if text, ok := itemMap["text"].(string); ok && strings.HasPrefix(text, "data:image/") {
						return true
					}
				}
			}
		}
	case map[string]interface{}:
		// Check if it's an image object
		if imgType, ok := v["type"].(string); ok && (imgType == "image_url" || imgType == "image") {
			return true
		}
	}
	return false
}

// hasCodeGenerationHints checks for indicators that code generation is needed
func (bi *BodyInspector) hasCodeGenerationHints(data map[string]interface{}) bool {
	// Check for code-related parameters
	if bi.hasCodeParameters(data) {
		return true
	}

	// Check system prompts for code indicators
	return bi.hasCodeInSystemPrompt(data)
}

// hasCodeParameters checks for explicit code-related parameters
func (bi *BodyInspector) hasCodeParameters(data map[string]interface{}) bool {
	if lang, ok := data["language"].(string); ok && lang != "" {
		return true
	}
	if codeModel, ok := data["code_model"].(bool); ok && codeModel {
		return true
	}
	return false
}

// hasCodeInSystemPrompt checks for code keywords in system prompts
func (bi *BodyInspector) hasCodeInSystemPrompt(data map[string]interface{}) bool {
	messages, ok := data["messages"].([]interface{})
	if !ok {
		return false
	}

	for _, msg := range messages {
		if bi.isSystemMessageWithCodeKeywords(msg) {
			return true
		}
	}
	return false
}

// isSystemMessageWithCodeKeywords checks if a message is a system message containing code keywords
func (bi *BodyInspector) isSystemMessageWithCodeKeywords(msg interface{}) bool {
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		return false
	}

	role, _ := msgMap["role"].(string)
	if role != "system" {
		return false
	}

	content, ok := msgMap["content"].(string)
	if !ok {
		return false
	}

	return bi.containsCodeKeywords(content)
}

// containsCodeKeywords checks if content contains code-related keywords
func (bi *BodyInspector) containsCodeKeywords(content string) bool {
	lowerContent := strings.ToLower(content)
	codeKeywords := []string{"code", "programming", "function", "class", "debug", "implement"}
	for _, keyword := range codeKeywords {
		if strings.Contains(lowerContent, keyword) {
			return true
		}
	}
	return false
}
