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
			contentType: "image/png",
			want:        true,
		},
		{
			name:        "explicit buffered profile overrides content type",
			profile:     constants.ConfigurationProxyProfileStandard,
			contentType: "text/event-stream",
			want:        false,
		},

		// Auto mode recognises known streaming formats
		{
			name:        "auto mode detects SSE streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "text/event-stream",
			want:        true,
		},
		{
			name:        "auto mode detects NDJSON streaming",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/x-ndjson",
			want:        true,
		},
		{
			name:        "auto mode detects streaming JSON",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/stream+json",
			want:        true,
		},
		{
			name:        "auto mode detects JSON sequence",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/json-seq",
			want:        true,
		},
		{
			name:        "auto mode detects text/plain streaming (some LLMs)",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "text/plain; charset=utf-8",
			want:        true,
		},

		// Binary files get buffered in auto mode to prevent corruption
		{
			name:        "auto mode buffers PNG images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "image/png",
			want:        false,
		},
		{
			name:        "auto mode buffers JPEG images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "image/jpeg",
			want:        false,
		},
		{
			name:        "auto mode buffers WebP images",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "image/webp",
			want:        false,
		},
		{
			name:        "auto mode buffers videos",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "video/mp4",
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
			contentType: "application/pdf",
			want:        false,
		},
		{
			name:        "auto mode buffers ZIP files",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/zip",
			want:        false,
		},
		{
			name:        "auto mode buffers octet-stream",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/octet-stream",
			want:        false,
		},
		{
			name:        "auto mode buffers Excel files",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/vnd.ms-excel",
			want:        false,
		},
		{
			name:        "auto mode buffers Word documents",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
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
			contentType: "application/json",
			want:        true,
		},
		{
			name:        "auto mode streams HTML",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "text/html",
			want:        true,
		},
		{
			name:        "auto mode streams XML",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/xml",
			want:        true,
		},
		{
			name:        "auto mode streams plain text",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "text/plain",
			want:        true,
		},

		// Client preferences from original request context
		{
			name:        "context stream=true forces streaming for JSON",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/json",
			ctxStream:   true,
			want:        true,
		},
		{
			name:        "context stream=false doesn't force buffering in auto mode",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "application/json",
			ctxStream:   false,
			want:        true, // Text still streams regardless
		},
		{
			name:        "context stream=true forces streaming even for images in auto mode",
			profile:     constants.ConfigurationProxyProfileAuto,
			contentType: "image/png",
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
				resp.Header.Set("Content-Type", tt.contentType)
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
		{"SSE", "text/event-stream", true},
		{"SSE with charset", "text/event-stream; charset=utf-8", true},
		{"NDJSON", "application/x-ndjson", true},
		{"Stream JSON", "application/stream+json", true},
		{"JSON Seq", "application/json-seq", true},
		{"Plain text streaming", "text/plain; charset=utf-8", true},
		{"Regular JSON", "application/json", false},
		{"HTML", "text/html", false},
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
		{"PNG", "image/png", true},
		{"JPEG", "image/jpeg", true},
		{"WebP", "image/webp", true},
		{"SVG", "image/svg+xml", true},

		{"MP4", "video/mp4", true},
		{"WebM", "video/webm", true},
		{"MP3", "audio/mpeg", true},
		{"WAV", "audio/wav", true},

		// Office documents and PDFs
		{"PDF", "application/pdf", true},
		{"Word", "application/msword", true},
		{"Word OOXML", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"Excel", "application/vnd.ms-excel", true},
		{"PowerPoint", "application/vnd.ms-powerpoint", true},

		// Compressed archives
		{"ZIP", "application/zip", true},
		{"GZIP", "application/gzip", true},
		{"TAR", "application/x-tar", true},
		{"RAR", "application/x-rar", true},
		{"7Z", "application/x-7z-compressed", true},

		// Miscellaneous binary formats
		{"Octet stream", "application/octet-stream", true},
		{"Font WOFF", "font/woff", true},
		{"Font WOFF2", "font/woff2", true},
		{"3D Model", "model/gltf-binary", true},

		// Text-based formats that should stream
		{"JSON", "application/json", false},
		{"HTML", "text/html", false},
		{"Plain text", "text/plain", false},
		{"XML", "application/xml", false},
		{"JavaScript", "application/javascript", false},
		{"CSS", "text/css", false},
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
