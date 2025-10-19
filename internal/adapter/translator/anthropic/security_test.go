package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestValidation_RequiredFields(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("missing_model", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			// Model: "", // missing required field
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model field is required")
	})

	t.Run("missing_messages", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			Messages:  []AnthropicMessage{}, // empty - required
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one message is required")
	})
}

func TestRequestValidation_ParameterRanges(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("negative_max_tokens", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: -100, // invalid
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_tokens must be at least 1")
	})

	t.Run("zero_max_tokens", func(t *testing.T) {
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 0, // invalid - must be at least 1
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_tokens must be at least 1")
	})

	t.Run("temperature_too_high", func(t *testing.T) {
		temp := 3.0 // > 2.0
		anthropicReq := AnthropicRequest{
			Model:       "claude-sonnet-4-20250929",
			MaxTokens:   1024,
			Temperature: &temp,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temperature must be between 0 and 2")
	})

	t.Run("temperature_negative", func(t *testing.T) {
		temp := -0.5
		anthropicReq := AnthropicRequest{
			Model:       "claude-sonnet-4-20250929",
			MaxTokens:   1024,
			Temperature: &temp,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temperature must be between 0 and 2")
	})

	t.Run("top_p_too_high", func(t *testing.T) {
		topP := 1.5 // > 1.0
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			TopP:      &topP,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "top_p must be between 0 and 1")
	})

	t.Run("top_p_negative", func(t *testing.T) {
		topP := -0.1
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			TopP:      &topP,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "top_p must be between 0 and 1")
	})

	t.Run("top_k_negative", func(t *testing.T) {
		topK := -10
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			TopK:      &topK,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "top_k must be non-negative")
	})
}

func TestRequestValidation_ValidParameters(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("valid_temperature_boundary", func(t *testing.T) {
		temp := 2.0 // exactly 2.0 should be valid
		anthropicReq := AnthropicRequest{
			Model:       "claude-sonnet-4-20250929",
			MaxTokens:   1024,
			Temperature: &temp,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.NoError(t, err)
	})

	t.Run("valid_top_p_boundary", func(t *testing.T) {
		topP := 1.0 // exactly 1.0 should be valid
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			TopP:      &topP,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.NoError(t, err)
	})

	t.Run("valid_zero_top_k", func(t *testing.T) {
		topK := 0 // zero should be valid
		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			TopK:      &topK,
			Messages: []AnthropicMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.NoError(t, err)
	})
}

func TestRequestSizeLimit(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("request_exceeding_10MB", func(t *testing.T) {
		// Create a request that exceeds 10MB
		largeContent := strings.Repeat("A", 11*1024*1024) // 11mb of 'a's

		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{Role: "user", Content: largeContent},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.Error(t, err)
		// The error should be a JSON parsing error because the body was truncated
		assert.Contains(t, err.Error(), "failed to parse Anthropic request")
	})

	t.Run("request_just_under_10MB", func(t *testing.T) {
		// Create a request just under 10mb (should succeed)
		// Account for JSON overhead
		safeContent := strings.Repeat("A", 9*1024*1024) // 9mb should be safe

		anthropicReq := AnthropicRequest{
			Model:     "claude-sonnet-4-20250929",
			MaxTokens: 1024,
			Messages: []AnthropicMessage{
				{Role: "user", Content: safeContent},
			},
		}

		body, err := json.Marshal(anthropicReq)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader(body)),
		}

		_, err = translator.TransformRequest(ctx, req)
		require.NoError(t, err)
	})
}

func TestUnknownFieldsRejection(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("request_with_unknown_field", func(t *testing.T) {
		// Create raw JSON with an unknown field
		rawJSON := `{
			"model": "claude-sonnet-4-20250929",
			"max_tokens": 1024,
			"messages": [
				{"role": "user", "content": "Hello"}
			],
			"unknown_field": "this should cause rejection"
		}`

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(rawJSON))),
		}

		_, err := translator.TransformRequest(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Anthropic request")
	})

	t.Run("request_without_unknown_fields", func(t *testing.T) {
		// Valid request should succeed
		rawJSON := `{
			"model": "claude-sonnet-4-20250929",
			"max_tokens": 1024,
			"messages": [
				{"role": "user", "content": "Hello"}
			]
		}`

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(rawJSON))),
		}

		_, err := translator.TransformRequest(ctx, req)
		require.NoError(t, err)
	})
}

func TestSecurityFeaturesCombined(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	ctx := context.Background()

	t.Run("invalid_params_with_unknown_fields", func(t *testing.T) {
		// Request with both unknown field and invalid temperature
		rawJSON := `{
			"model": "claude-sonnet-4-20250929",
			"max_tokens": 1024,
			"temperature": 5.0,
			"messages": [
				{"role": "user", "content": "Hello"}
			],
			"malicious_field": "attempt"
		}`

		req := &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(rawJSON))),
		}

		_, err := translator.TransformRequest(ctx, req)
		require.Error(t, err)
		// Should fail at JSON parsing stage due to unknown field
		assert.Contains(t, err.Error(), "failed to parse Anthropic request")
	})
}
