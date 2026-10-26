package inspector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

const (
	BodyInspectorName = "body"
	MaxBodySize       = 1024 * 1024 // 1MB max body size for full body inspection (capabilities etc.)

	// modelScanSize is the hard ceiling on bytes read when scanning for the top-level "model"
	// field in large requests (e.g. vision requests with base64 image payloads). The scan
	// stops as soon as the field is found, so for the common case where "model" appears near
	// the start only a handful of bytes are consumed. The ceiling exists to bound memory use
	// when "model" appears after large fields (e.g. a multi-megabyte "messages" array);
	// 8 MB accommodates typical vision payloads while staying well within practical limits.
	// Requests where "model" is absent or appears beyond this ceiling will not be routed by
	// model name - capability-based or default routing applies instead.
	modelScanSize = 8 * 1024 * 1024

	// peekWindow bounds how much leading whitespace we tolerate before deciding a body is not
	// a JSON object; it is also the bufio size, so it is the only per-request allocation added.
	peekWindow = 256
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

	// The body decides, not the header: curl -d sends form-urlencoded, fetch() with a string
	// body sends text/plain and plenty of scripts send nothing, yet all carry JSON. Skipping on
	// Content-Type left those requests model-less, so they were routed to any compatible host.
	// Multipart and encoded bodies are opaque to us (we never decompress), and anything that
	// does not open with '{' cannot hold a top-level model field.
	if bi.opaqueBody(r) {
		return nil
	}
	if !bi.looksLikeJSONObject(r) {
		return nil
	}

	// Large requests (e.g. vision with base64 images) still need the model name for routing,
	// but buffering the full body just to parse JSON is wasteful and would OOM for
	// multi-megabyte payloads. Instead stream-scan up to modelScanSize bytes (see inspectLarge).
	if r.ContentLength > bi.maxBodySize {
		bi.inspectLarge(r, profile)
		return nil
	}

	buffer, err := bi.bufferPool.Get()
	if err != nil {
		return fmt.Errorf("body inspector: buffer pool exhausted: %w", err)
	}
	defer func() {
		buffer.Reset()
		bi.bufferPool.Put(buffer)
	}()

	limitedReader := io.LimitReader(r.Body, bi.maxBodySize)

	if _, err := io.Copy(buffer, limitedReader); err != nil {
		bi.logger.Debug("Failed to read request body", "error", err)
		return nil
	}

	// An unknown length (chunked) body that filled the buffer may be arbitrarily larger, and a
	// model placed after a big field would be missed by parsing a truncated prefix. Hand it to
	// the bounded scan, replaying what we have already read.
	if r.ContentLength < 0 && int64(buffer.Len()) >= bi.maxBodySize {
		origBody := r.Body
		r.Body = readCloser{
			Reader: io.MultiReader(bytes.NewReader(append([]byte(nil), buffer.Bytes()...)), origBody),
			Closer: origBody,
		}
		bi.inspectLarge(r, profile)
		return nil
	}

	// Copy the buffer bytes before the deferred Reset()/Put() invalidates the slice.
	// buffer.Bytes() aliases the pool buffer's internal array; once Reset() runs the
	// underlying array can be reused for another request, corrupting r.Body reads.
	restored := append([]byte(nil), buffer.Bytes()...)

	// Restore the body for downstream handlers by creating a new reader that combines
	// what we've already read with any remaining unread content
	origBody := r.Body
	r.Body = readCloser{
		Reader: io.MultiReader(bytes.NewReader(restored), origBody),
		Closer: origBody,
	}

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
			// the path inspector may already have set what the path implies
			// (rerank, classification, native embed); keep it beside the body's
			if prior := profile.ModelCapabilities; prior != nil {
				capabilities.Rerank = prior.Rerank
				capabilities.Classification = prior.Classification
				// a rerank or classification path outranks the body's embeddings
				// guess: rerank bodies carry `input`/`texts`, and no model is
				// both an embedder and a reranker, so keeping both would
				// match nothing
				if prior.Rerank || prior.Classification {
					capabilities.Embeddings = prior.Embeddings
				} else {
					capabilities.Embeddings = capabilities.Embeddings || prior.Embeddings
				}
			}
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

// opaqueBody reports bodies we must not look inside: multipart (the model, if any, is a form
// part) and anything with a Content-Encoding, which we deliberately never decompress.
func (bi *BodyInspector) opaqueBody(r *http.Request) bool {
	ct := strings.ToLower(r.Header.Get(constants.HeaderContentType))
	if strings.HasPrefix(strings.TrimSpace(ct), "multipart/") {
		bi.logger.Debug("Skipping body inspection for multipart content", "content_type", ct)
		return true
	}
	if enc := r.Header.Get(constants.HeaderContentEncoding); enc != "" {
		bi.logger.Debug("Skipping body inspection for encoded content", "content_encoding", enc)
		return true
	}
	return false
}

// looksLikeJSONObject peeks at the first non-whitespace byte without consuming it. It asks for
// one more byte at a time so it never waits for data beyond what the decision needs. The body
// is always left readable from its first byte.
func (bi *BodyInspector) looksLikeJSONObject(r *http.Request) bool {
	origBody := r.Body
	br := bufio.NewReaderSize(origBody, peekWindow)
	r.Body = readCloser{Reader: br, Closer: origBody}

	for n := 1; n <= peekWindow; n++ {
		head, err := br.Peek(n)
		if len(head) < n {
			// EOF or read error before any decision byte: nothing to inspect.
			_ = err
			return false
		}
		switch c := head[n-1]; c {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			bi.logger.Debug("Skipping body inspection for non-object body")
			return false
		}
	}
	return false
}

// inspectLarge scans r.Body for the top-level "model" key using a TeeReader so every byte read
// is captured for restoration, with token-level iteration that skips large nested values
// without buffering them. This handles "messages" (with large base64 images) preceding "model".
func (bi *BodyInspector) inspectLarge(r *http.Request, profile *domain.RequestProfile) {
	// captured is a locally-allocated bytes.Buffer (not from a sync.Pool), so
	// captured.Bytes() is safe to use directly without an extra copy - unlike the
	// small-body path which must copy buffer.Bytes() to avoid pool aliasing
	// once the deferred Reset()/Put() runs.
	captured := &bytes.Buffer{}
	origBody := r.Body
	tee := io.TeeReader(io.LimitReader(origBody, modelScanSize), captured)

	modelName := extractTopLevelModelFieldFromReader(tee)

	// Restore body: bytes the decoder read (captured via TeeReader) + remaining original body.
	// The decoder may have stopped before the limit, so origBody still holds the unconsumed tail.
	// readCloser ensures Close() propagates to origBody to drain the connection and avoid leaks.
	r.Body = readCloser{
		Reader: io.MultiReader(bytes.NewReader(captured.Bytes()), origBody),
		Closer: origBody,
	}

	if modelName != "" {
		profile.ModelName = bi.normalizeModelName(modelName)
		bi.logger.Debug("Extracted model name from large request body scan",
			"model", profile.ModelName, "content_length", r.ContentLength)
	} else {
		bi.logger.Debug("Could not extract model name from large request body",
			"content_length", r.ContentLength)
	}
}

func (bi *BodyInspector) extractModelName(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	// Fast path: complete JSON - unmarshal directly.
	var req modelRequest
	if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
		return bi.normalizeModelName(req.Model)
	}

	// Streaming path: works on both complete and truncated JSON (e.g. a 64 KB prefix of a
	// multi-megabyte vision request). Scan only top-level tokens so we never descend into
	// the large base64 image string embedded in the messages array.
	if model := extractTopLevelModelFieldFromReader(bytes.NewReader(body)); model != "" {
		return bi.normalizeModelName(model)
	}

	// Fall back to flexible map-based extraction to handle non-standard formats.
	// This only succeeds for complete, valid JSON bodies.
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

// extractTopLevelModelFieldFromReader scans JSON from r (which may be a truncated stream)
// with a streaming decoder and returns the value of the first top-level "model" string key.
// Non-model field values are skipped using token-level iteration so large nested values
// (e.g. base64 image payloads in "messages") are never buffered into memory.
// This allows correct extraction even when "model" appears after large fields.
func extractTopLevelModelFieldFromReader(r io.Reader) string {
	dec := json.NewDecoder(r)

	// Consume the opening '{' of the top-level object.
	tok, err := dec.Token()
	if err != nil {
		return ""
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return ""
	}

	for dec.More() {
		// Read the key token. At this position the decoder always yields a string key.
		keyTok, err := dec.Token()
		if err != nil {
			return ""
		}

		key, ok := keyTok.(string)
		if !ok {
			// Malformed JSON - key position must be a string.
			return ""
		}

		if strings.EqualFold(key, "model") {
			// Read the value as a raw message; the value is small so allocation is fine.
			var val json.RawMessage
			if err := dec.Decode(&val); err != nil {
				return ""
			}
			var modelStr string
			if err := json.Unmarshal(val, &modelStr); err == nil && modelStr != "" {
				return modelStr
			}
			// Value wasn't a string - keep scanning.
			continue
		}

		// Skip the value for this key using token-level iteration so we never buffer
		// a large nested value (e.g. a messages array containing base64 images).
		if err := skipValueTokens(dec); err != nil {
			// Truncated JSON or I/O error - stop scanning.
			return ""
		}
	}

	return ""
}

// skipValueTokens advances dec past the next JSON value at the current read position.
// It uses token-level iteration to avoid allocating a buffer for the entire value,
// which is important when skipping large nested arrays or objects (e.g. vision payloads).
func skipValueTokens(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		// Scalar value (string, number, bool, null) - already consumed.
		return nil
	}
	if delim != '{' && delim != '[' {
		// Unexpected closing delimiter at value position - treat as error.
		return fmt.Errorf("unexpected closing delimiter %v", delim)
	}
	// Track open/close depth to handle arbitrarily nested structures.
	depth := 1
	for depth > 0 {
		tok, err = dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

func (bi *BodyInspector) normalizeModelName(model string) string {
	model = strings.TrimSpace(model)
	model = strings.ToLower(model)
	// Model aliasing and tag handling is delegated to the registry layer
	return model
}

// readCloser combines a reader (e.g. io.MultiReader) with the original body's Closer so that
// Close properly drains/releases the underlying connection rather than becoming a no-op.
type readCloser struct {
	io.Reader
	io.Closer
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
