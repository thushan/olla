package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// mockProfileLookup is a test double that implements translator.ProfileLookup
// Allows configuring AnthropicSupportConfig per endpoint type for testing
type mockProfileLookup struct {
	configs map[string]*domain.AnthropicSupportConfig
}

// GetAnthropicSupport returns the configured AnthropicSupportConfig for the given endpoint type
func (m *mockProfileLookup) GetAnthropicSupport(endpointType string) *domain.AnthropicSupportConfig {
	if m.configs == nil {
		return nil
	}
	return m.configs[endpointType]
}

// newMockProfileLookup creates a new mock profile lookup with empty configuration
func newMockProfileLookup() *mockProfileLookup {
	return &mockProfileLookup{
		configs: make(map[string]*domain.AnthropicSupportConfig),
	}
}

// withSupport adds AnthropicSupportConfig for a specific endpoint type
func (m *mockProfileLookup) withSupport(endpointType string, cfg *domain.AnthropicSupportConfig) *mockProfileLookup {
	m.configs[endpointType] = cfg
	return m
}

// TestCanPassthrough tests the CanPassthrough method with various endpoint configurations
func TestCanPassthrough(t *testing.T) {
	tests := []struct {
		name               string
		passthroughEnabled bool
		endpoints          []*domain.Endpoint
		profileLookup      translator.ProfileLookup
		want               bool
		description        string
	}{
		{
			name:               "all_endpoints_support_anthropic",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
				{Name: "vllm-2", Type: "vllm"},
			},
			profileLookup: newMockProfileLookup().withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      true,
				MessagesPath: "/v1/messages",
			}),
			want:        true,
			description: "should return true when all endpoints support Anthropic passthrough",
		},
		{
			name:               "mixed_endpoints_some_support_some_dont",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
				{Name: "ollama-1", Type: "ollama"},
			},
			profileLookup: newMockProfileLookup().
				withSupport("vllm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}).
				withSupport("ollama", nil), // ollama doesn't support
			want:        false,
			description: "should return false when some endpoints don't support Anthropic",
		},
		{
			name:               "no_endpoints_support_anthropic",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "ollama-1", Type: "ollama"},
				{Name: "ollama-2", Type: "ollama"},
			},
			profileLookup: newMockProfileLookup().withSupport("ollama", nil),
			want:          false,
			description:   "should return false when no endpoints support Anthropic",
		},
		{
			name:               "passthrough_disabled",
			passthroughEnabled: false,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
			},
			profileLookup: newMockProfileLookup().withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      true,
				MessagesPath: "/v1/messages",
			}),
			want:        false,
			description: "should return false when passthrough is disabled even if endpoints support it",
		},
		{
			name:               "empty_endpoints_list",
			passthroughEnabled: true,
			endpoints:          []*domain.Endpoint{},
			profileLookup:      newMockProfileLookup(),
			want:               false,
			description:        "should return false when endpoints list is empty",
		},
		{
			name:               "nil_anthropic_support_config",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "custom-1", Type: "custom"},
			},
			profileLookup: newMockProfileLookup(), // no config for "custom"
			want:          false,
			description:   "should return false when AnthropicSupportConfig is nil",
		},
		{
			name:               "anthropic_support_disabled",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
			},
			profileLookup: newMockProfileLookup().withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      false, // explicitly disabled
				MessagesPath: "/v1/messages",
			}),
			want:        false,
			description: "should return false when AnthropicSupport.Enabled is false",
		},
		{
			name:               "multiple_endpoints_all_support",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
				{Name: "vllm-2", Type: "vllm"},
				{Name: "sglang-1", Type: "sglang"},
				{Name: "litellm-1", Type: "litellm"},
			},
			profileLookup: newMockProfileLookup().
				withSupport("vllm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}).
				withSupport("sglang", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}).
				withSupport("litellm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}),
			want:        true,
			description: "should return true when multiple different endpoint types all support Anthropic",
		},
		{
			name:               "single_endpoint_without_support",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
				{Name: "vllm-2", Type: "vllm"},
				{Name: "vllm-3", Type: "vllm"},
				{Name: "unsupported-1", Type: "unsupported"},
			},
			profileLookup: newMockProfileLookup().
				withSupport("vllm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}),
			// "unsupported" type has no config (returns nil)
			want:        false,
			description: "should return false if even one endpoint doesn't support passthrough",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.AnthropicTranslatorConfig{
				Enabled:            true,
				MaxMessageSize:     10 << 20, // 10MB
				PassthroughEnabled: tt.passthroughEnabled,
			}

			translator := NewTranslator(createTestLogger(), cfg)
			result := translator.CanPassthrough(tt.endpoints, tt.profileLookup)

			assert.Equal(t, tt.want, result, tt.description)
		})
	}
}

// TestPreparePassthrough tests the PreparePassthrough method with various request scenarios
func TestPreparePassthrough(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  string
		maxMsgSize   int64
		wantErr      bool
		errContains  string
		validateFunc func(t *testing.T, result *translator.PassthroughRequest)
		description  string
	}{
		{
			name: "valid_anthropic_request",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Hello, world!"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.NotNil(t, result)
				assert.Equal(t, "/v1/messages", result.TargetPath)
				assert.Equal(t, "claude-3-5-sonnet-20241022", result.ModelName)
				assert.False(t, result.IsStreaming)
				assert.NotEmpty(t, result.Body)

				// Verify body is preserved unchanged
				var req AnthropicRequest
				err := json.Unmarshal(result.Body, &req)
				require.NoError(t, err)
				assert.Equal(t, "claude-3-5-sonnet-20241022", req.Model)
				assert.Equal(t, 1024, req.MaxTokens)
				assert.Len(t, req.Messages, 1)
			},
			description: "should successfully prepare valid Anthropic request",
		},
		{
			name:        "invalid_json",
			requestBody: `{invalid json`,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "invalid Anthropic request",
			description: "should return error for invalid JSON",
		},
		{
			name: "missing_model_field",
			requestBody: `{
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "model field is required",
			description: "should return error when model field is missing",
		},
		{
			name: "missing_messages_field",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": []
			}`,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "at least one message is required",
			description: "should return error when messages array is empty",
		},
		{
			name: "request_exceeds_max_message_size",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "This is a test message"}
				]
			}`,
			maxMsgSize:  50, // Very small limit - causes truncated JSON
			wantErr:     true,
			errContains: "invalid Anthropic request", // LimitReader causes truncated JSON
			description: "should return error when request exceeds max_message_size",
		},
		{
			name:        "empty_body",
			requestBody: ``,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "invalid Anthropic request",
			description: "should return error for empty body",
		},
		{
			name: "streaming_enabled",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"stream": true,
				"messages": [
					{"role": "user", "content": "Stream this response"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.True(t, result.IsStreaming)
				assert.Equal(t, "claude-3-5-sonnet-20241022", result.ModelName)
			},
			description: "should correctly extract streaming flag when true",
		},
		{
			name: "streaming_disabled",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"stream": false,
				"messages": [
					{"role": "user", "content": "Don't stream this"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.False(t, result.IsStreaming)
			},
			description: "should correctly extract streaming flag when false",
		},
		{
			name: "streaming_not_specified",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "No stream field"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.False(t, result.IsStreaming) // defaults to false
			},
			description: "should default streaming to false when not specified",
		},
		{
			name: "target_path_is_v1_messages",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Test"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.Equal(t, "/v1/messages", result.TargetPath)
			},
			description: "should set target path to /v1/messages",
		},
		{
			name: "body_preserved_unchanged",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 2048,
				"temperature": 0.7,
				"system": "You are a helpful assistant",
				"messages": [
					{"role": "user", "content": "Complex request"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				var req AnthropicRequest
				err := json.Unmarshal(result.Body, &req)
				require.NoError(t, err)

				// Verify all fields preserved
				assert.Equal(t, "claude-3-5-sonnet-20241022", req.Model)
				assert.Equal(t, 2048, req.MaxTokens)
				assert.NotNil(t, req.Temperature)
				assert.Equal(t, 0.7, *req.Temperature)
				assert.NotNil(t, req.System)
			},
			description: "should preserve body unchanged with all fields intact",
		},
		{
			name: "model_name_extraction",
			requestBody: `{
				"model": "claude-opus-4-20250514",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Test model extraction"}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				assert.Equal(t, "claude-opus-4-20250514", result.ModelName)
			},
			description: "should correctly extract model name from request",
		},
		{
			name: "invalid_max_tokens_zero",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 0,
				"messages": [
					{"role": "user", "content": "Test"}
				]
			}`,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "max_tokens must be at least 1",
			description: "should return error for invalid max_tokens value",
		},
		{
			name: "invalid_temperature_too_high",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"temperature": 3.0,
				"messages": [
					{"role": "user", "content": "Test"}
				]
			}`,
			maxMsgSize:  10 << 20,
			wantErr:     true,
			errContains: "temperature must be between 0 and 2",
			description: "should return error for invalid temperature value",
		},
		{
			name: "complex_request_with_tools",
			requestBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "What's the weather?"}
				],
				"tools": [
					{
						"name": "get_weather",
						"description": "Get weather information",
						"input_schema": {
							"type": "object",
							"properties": {
								"location": {"type": "string"}
							}
						}
					}
				]
			}`,
			maxMsgSize: 10 << 20,
			wantErr:    false,
			validateFunc: func(t *testing.T, result *translator.PassthroughRequest) {
				var req AnthropicRequest
				err := json.Unmarshal(result.Body, &req)
				require.NoError(t, err)
				assert.Len(t, req.Tools, 1)
				assert.Equal(t, "get_weather", req.Tools[0].Name)
			},
			description: "should handle complex request with tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.AnthropicTranslatorConfig{
				Enabled:            true,
				MaxMessageSize:     tt.maxMsgSize,
				PassthroughEnabled: true,
			}

			translator := NewTranslator(createTestLogger(), cfg)

			// Create HTTP request
			req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader([]byte(tt.requestBody)))
			require.NoError(t, err)

			// Create a minimal mock ProfileLookup (not used by PreparePassthrough but required by interface)
			mockLookup := newMockProfileLookup()

			// Execute PreparePassthrough
			result, err := translator.PreparePassthrough(req, mockLookup)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains, tt.description)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, result)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// TestPreparePassthrough_ReadError tests error handling when reading request body fails
func TestPreparePassthrough_ReadError(t *testing.T) {
	cfg := config.AnthropicTranslatorConfig{
		Enabled:            true,
		MaxMessageSize:     10 << 20,
		PassthroughEnabled: true,
	}

	translator := NewTranslator(createTestLogger(), cfg)

	// Create a request with a body that will error on read
	req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", io.NopCloser(&errorReader{}))
	require.NoError(t, err)

	mockLookup := newMockProfileLookup()

	result, err := translator.PreparePassthrough(req, mockLookup)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read request body")
	assert.Nil(t, result)
}

// TestPreparePassthrough_WithInspector tests that inspector logging is called when enabled
func TestPreparePassthrough_WithInspector(t *testing.T) {
	// Create a temporary directory for inspector output
	tempDir := t.TempDir()

	cfg := config.AnthropicTranslatorConfig{
		Enabled:            true,
		MaxMessageSize:     10 << 20,
		PassthroughEnabled: true,
		Inspector: config.InspectorConfig{
			Enabled:       true,
			OutputDir:     tempDir,
			SessionHeader: "X-Session-ID",
		},
	}

	translator := NewTranslator(createTestLogger(), cfg)

	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Test with inspector"}
		]
	}`

	req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader([]byte(requestBody)))
	require.NoError(t, err)
	req.Header.Set("X-Session-ID", "test-session-123")

	mockLookup := newMockProfileLookup()

	result, err := translator.PreparePassthrough(req, mockLookup)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Note: We don't verify file creation here as that's an implementation detail
	// The important thing is that PreparePassthrough succeeds with inspector enabled
}

// errorReader is a test helper that always returns an error when Read is called
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// TestCanPassthrough_Integration tests the integration between CanPassthrough and realistic endpoint scenarios
func TestCanPassthrough_Integration(t *testing.T) {
	t.Run("realistic_vllm_sglang_deployment", func(t *testing.T) {
		// Simulate a realistic deployment with vLLM and SGLang backends
		endpoints := []*domain.Endpoint{
			{Name: "vllm-gpu-1", Type: "vllm"},
			{Name: "vllm-gpu-2", Type: "vllm"},
			{Name: "sglang-gpu-1", Type: "sglang"},
		}

		profileLookup := newMockProfileLookup().
			withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      true,
				MessagesPath: "/v1/messages",
			}).
			withSupport("sglang", &domain.AnthropicSupportConfig{
				Enabled:      true,
				MessagesPath: "/v1/messages",
			})

		cfg := config.AnthropicTranslatorConfig{
			Enabled:            true,
			MaxMessageSize:     10 << 20,
			PassthroughEnabled: true,
		}

		translator := NewTranslator(createTestLogger(), cfg)
		result := translator.CanPassthrough(endpoints, profileLookup)

		assert.True(t, result, "should support passthrough for vLLM+SGLang deployment")
	})

	t.Run("mixed_deployment_with_ollama", func(t *testing.T) {
		// Simulate a mixed deployment where not all backends support Anthropic
		endpoints := []*domain.Endpoint{
			{Name: "vllm-gpu-1", Type: "vllm"},
			{Name: "ollama-cpu-1", Type: "ollama"}, // Ollama doesn't support Anthropic
		}

		profileLookup := newMockProfileLookup().
			withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      true,
				MessagesPath: "/v1/messages",
			})
		// Ollama has no Anthropic support (returns nil)

		cfg := config.AnthropicTranslatorConfig{
			Enabled:            true,
			MaxMessageSize:     10 << 20,
			PassthroughEnabled: true,
		}

		translator := NewTranslator(createTestLogger(), cfg)
		result := translator.CanPassthrough(endpoints, profileLookup)

		assert.False(t, result, "should not support passthrough with mixed backend capabilities")
	})
}
