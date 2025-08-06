package core

import (
	"context"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/constants"
)

// AutoDetectStreamingMode works out whether to stream or buffer a response
// LLMs benefit from streaming (users see tokens as they're generated),
// but binary files need buffering to avoid corruption and stalls
func AutoDetectStreamingMode(ctx context.Context, resp *http.Response, profile string) bool {
	// Force either if configured
	if profile == constants.ConfigurationProxyProfileStandard {
		return false
	}
	if profile == constants.ConfigurationProxyProfileStreaming {
		return true
	}

	// Auto mode - make an intelligent choice based on response content
	contentType := strings.ToLower(resp.Header.Get(constants.HeaderContentType))

	// if we know the streaming format, get streamed immediately
	if isStreamingContentType(contentType) {
		return true
	}

	// respect client preferences from the original request
	if streamVal := ctx.Value("stream"); streamVal != nil {
		if stream, ok := streamVal.(bool); ok && stream {
			return true
		}
	}

	// binary content needs buffering to ensure intact delivery
	if isBinaryContentType(contentType) {
		return false
	}

	// fallback to streaming for text-based responses otherwise
	return true
}

var streamingTypes = []string{
	"text/event-stream",
	"application/x-ndjson",
	"application/stream+json",
	"application/json-seq",
	"text/plain; charset=utf-8", // Common fallback for LLM streaming
}

// isStreamingContentType identifies known streaming formats.
// some APIs seem to be explicitly signaling streaming intent through content-type
// headers and we should respect that
func isStreamingContentType(contentType string) bool {
	for _, st := range streamingTypes {
		if strings.Contains(contentType, st) {
			return true
		}
	}
	return false
}

var binaryPrefixes = []string{
	"image/",
	"video/",
	"audio/",
	"application/pdf",
	"application/zip",
	"application/gzip",
	"application/x-tar",
	"application/x-rar",
	"application/x-7z",
	"font/",
	"model/", // 3D models, CAD files
}
var binaryTypes = []string{
	"application/octet-stream",
	"application/vnd.ms-excel",
	"application/vnd.openxmlformats-officedocument",
	"application/msword",
	"application/vnd.ms-powerpoint",
}

// isBinaryContentType identifies content that shouldn't be streamed.
// Binary files need to arrive complete and intact. Streaming a PDF
// or image byte-by-byte would be inefficient and potentially corrupt the file.
func isBinaryContentType(contentType string) bool {

	// try category-based checks first - most efficient & common
	for _, prefix := range binaryPrefixes {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}

	// these binary formats that might not match the prefixes
	for _, bt := range binaryTypes {
		if strings.Contains(contentType, bt) {
			return true
		}
	}

	return false
}
