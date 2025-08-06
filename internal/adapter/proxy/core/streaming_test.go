package core

import (
	"context"
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

/**
Tests inspired by Scout's streaming detection logic & tests

Keep these in sync so they're similar to avoid regressions
*/

func TestDetectStreamingMode(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		contentType string
		ctxStream   interface{} // Value for "stream" in context
		want        bool
	}{
		// Profile takes precedence - when ops explicitly configure streaming/buffering
		{
			name:        "explicit streaming profile overrides content type",
			profile:     constants.ConfigurationProxyProfileStreaming,
			contentType: constants.ContentTypeImagePNG,
			want:        true,
		},
		{
			name:        "explicit buffered profile overrides content type",
			profile:     constants.ConfigurationProxyProfileBuffered,
			contentType: constants.ContentTypeEventStream,
			want:        false,
		},

		// Auto mode recognises known streaming formats
		{
			name:        "auto mode detects SSE streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeEventStream,
			want:        true,
		},
		{
			name:        "auto mode detects NDJSON streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeNDJSON,
			want:        true,
		},
		{
			name:        "auto mode detects streaming JSON",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeStreamJSON,
			want:        true,
		},
		{
			name:        "auto mode detects JSON sequence",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeJSONSeq,
			want:        true,
		},
		{
			name:        "auto mode detects text/plain streaming (some LLMs)",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeTextUTF8,
			want:        true,
		},

		// Binary files get buffered in auto mode to prevent corruption
		{
			name:        "auto mode buffers PNG images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeImagePNG,
			want:        false,
		},
		{
			name:        "auto mode buffers JPEG images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeImageJPEG,
			want:        false,
		},
		{
			name:        "auto mode buffers WebP images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeImageWebP,
			want:        false,
		},
		{
			name:        "auto mode buffers videos",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeVideoMP4,
			want:        false,
		},
		{
			name:        "auto mode buffers audio",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "audio/mpeg",
			want:        false,
		},
		{
			name:        "auto mode buffers PDFs",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypePDF,
			want:        false,
		},
		{
			name:        "auto mode buffers ZIP files",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeZIP,
			want:        false,
		},
		{
			name:        "auto mode buffers octet-stream",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeOctetStream,
			want:        false,
		},
		{
			name:        "auto mode buffers Excel files",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeExcel,
			want:        false,
		},
		{
			name:        "auto mode buffers Word documents",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeWordDOCX,
			want:        false,
		},
		{
			name:        "auto mode buffers fonts",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "font/woff2",
			want:        false,
		},
		{
			name:        "auto mode buffers 3D models",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "model/gltf-binary",
			want:        false,
		},

		// Text formats stream by default - better UX for LLM responses
		{
			name:        "auto mode streams JSON by default",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeJSON,
			want:        true,
		},
		{
			name:        "auto mode streams HTML",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeHTML,
			want:        true,
		},
		{
			name:        "auto mode streams XML",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeXML,
			want:        true,
		},
		{
			name:        "auto mode streams plain text",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeText,
			want:        true,
		},

		// Client preferences from original request context
		{
			name:        "context stream=true forces streaming for JSON",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeJSON,
			ctxStream:   true,
			want:        true,
		},
		{
			name:        "context stream=false doesn't force buffering in auto mode",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeJSON,
			ctxStream:   false,
			want:        true, // Text still streams regardless
		},
		{
			name:        "context stream=true forces streaming even for images in auto mode",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: constants.ContentTypeImagePNG,
			ctxStream:   true,
			want:        true,
		},

		// Edge cases and defensive programming
		{
			name:        "empty content type defaults to streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "",
			want:        true,
		},
		{
			name:        "unknown content type defaults to streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/x-custom-type",
			want:        true,
		},
		{
			name:        "content type with parameters",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/json; charset=utf-8",
			want:        true,
		},
		{
			name:        "case sensitivity check",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "IMAGE/PNG",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.ctxStream != nil {
				ctx = context.WithValue(ctx, "stream", tt.ctxStream)
			}

			resp := &http.Response{
				Header: http.Header{},
			}
			if tt.contentType != "" {
				resp.Header.Set(constants.HeaderContentType, tt.contentType)
			}

			got := AutoDetectStreamingMode(ctx, resp, tt.profile)
			if got != tt.want {
				t.Errorf("AutoDetectStreamingMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsStreamingContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"SSE", constants.ContentTypeEventStream, true},
		{"SSE with charset", constants.ContentTypeEventStream + "; charset=utf-8", true},
		{"NDJSON", constants.ContentTypeNDJSON, true},
		{"Stream JSON", constants.ContentTypeStreamJSON, true},
		{"JSON Seq", constants.ContentTypeJSONSeq, true},
		{"Plain text streaming", constants.ContentTypeTextUTF8, true},
		{"Regular JSON", constants.ContentTypeJSON, false},
		{"HTML", constants.ContentTypeHTML, false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStreamingContentType(tt.contentType); got != tt.want {
				t.Errorf("isStreamingContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestIsBinaryContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		// Image formats - always binary
		{"PNG", constants.ContentTypeImagePNG, true},
		{"JPEG", constants.ContentTypeImageJPEG, true},
		{"WebP", constants.ContentTypeImageWebP, true},
		{"SVG", constants.ContentTypeImageSVG, true},

		{"MP4", constants.ContentTypeVideoMP4, true},
		{"WebM", constants.ContentTypeVideoWebM, true},
		{"MP3", "audio/mpeg", true},
		{"WAV", "audio/wav", true},

		// Office documents and PDFs
		{"PDF", constants.ContentTypePDF, true},
		{"Word", constants.ContentTypeWordDOC, true},
		{"Word OOXML", constants.ContentTypeWordDOCX, true},
		{"Excel", constants.ContentTypeExcel, true},
		{"PowerPoint", constants.ContentTypePowerPoint, true},

		// Compressed archives
		{"ZIP", constants.ContentTypeZIP, true},
		{"GZIP", constants.ContentTypeGZIP, true},
		{"TAR", constants.ContentTypeTAR, true},
		{"RAR", constants.ContentTypeRAR, true},
		{"7Z", constants.ContentType7Z, true},

		// Miscellaneous binary formats
		{"Octet stream", constants.ContentTypeOctetStream, true},
		{"Font WOFF", "font/woff", true},
		{"Font WOFF2", "font/woff2", true},
		{"3D Model", "model/gltf-binary", true},

		// Text-based formats that should stream
		{"JSON", constants.ContentTypeJSON, false},
		{"HTML", constants.ContentTypeHTML, false},
		{"Plain text", constants.ContentTypeText, false},
		{"XML", constants.ContentTypeXML, false},
		{"JavaScript", constants.ContentTypeJavaScript, false},
		{"CSS", constants.ContentTypeCSS, false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinaryContentType(tt.contentType); got != tt.want {
				t.Errorf("isBinaryContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
