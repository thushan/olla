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
	if !strings.Contains(contentType, "application/json") {
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
