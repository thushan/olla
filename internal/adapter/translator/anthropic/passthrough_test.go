package anthropic

import (
	"encoding/json"
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
			// The handler pre-filters to only capable endpoints before calling CanPassthrough.
			// By the time this method is called, non-capable backends are already excluded.
			// A mixed vllm+ollama deployment arrives here as [vllm-1] — passthrough proceeds.
			name:               "mixed_endpoints_some_support_some_dont",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"}, // handler already stripped ollama
			},
			profileLookup: newMockProfileLookup().
				withSupport("vllm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}),
			want:        true,
			description: "should return true when handler pre-filtered to only capable endpoints",
		},
		{
			// When no backends support Anthropic, the handler's filter produces an empty list.
			// CanPassthrough returns false for an empty list regardless of config.
			name:               "no_endpoints_support_anthropic",
			passthroughEnabled: true,
			endpoints:          []*domain.Endpoint{}, // handler filtered all out
			profileLookup:      newMockProfileLookup().withSupport("ollama", nil),
			want:               false,
			description:        "should return false when handler pre-filter yields no capable endpoints",
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
			// Backend with no AnthropicSupportConfig is excluded by the handler filter.
			// CanPassthrough receives an empty list and returns false.
			name:               "nil_anthropic_support_config",
			passthroughEnabled: true,
			endpoints:          []*domain.Endpoint{}, // handler excluded the unsupported custom endpoint
			profileLookup:      newMockProfileLookup(),
			want:               false,
			description:        "should return false when handler filter excludes unsupported backends",
		},
		{
			// Backend with Enabled:false is excluded by the handler filter.
			// CanPassthrough receives an empty list and returns false.
			name:               "anthropic_support_disabled",
			passthroughEnabled: true,
			endpoints:          []*domain.Endpoint{}, // handler excluded the disabled endpoint
			profileLookup: newMockProfileLookup().withSupport("vllm", &domain.AnthropicSupportConfig{
				Enabled:      false,
				MessagesPath: "/v1/messages",
			}),
			want:        false,
			description: "should return false when handler filter excludes explicitly-disabled backends",
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
			// The handler filters the unsupported backend before calling CanPassthrough.
			// The three vllm endpoints survive the filter — passthrough proceeds.
			name:               "single_endpoint_without_support",
			passthroughEnabled: true,
			endpoints: []*domain.Endpoint{
				{Name: "vllm-1", Type: "vllm"},
				{Name: "vllm-2", Type: "vllm"},
				{Name: "vllm-3", Type: "vllm"},
				// unsupported-1 already removed by handler filter
			},
			profileLookup: newMockProfileLookup().
				withSupport("vllm", &domain.AnthropicSupportConfig{
					Enabled:      true,
					MessagesPath: "/v1/messages",
				}),
			want:        true,
			description: "should return true when handler pre-filter removes the unsupported endpoint",
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
			maxMsgSize:  50, // Very small limit
			wantErr:     true,
			errContains: "request body exceeds maximum size",
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

			// Pre-buffer the body bytes (as the handler does in production)
			bodyBytes := []byte(tt.requestBody)

			// Create HTTP request -- body is no longer read by PreparePassthrough
			// but the request is still passed for header access (inspector, session ID)
			req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", http.NoBody)
			require.NoError(t, err)

			// Create a minimal mock ProfileLookup (not used by PreparePassthrough but required by interface)
			mockLookup := newMockProfileLookup()

			// Execute PreparePassthrough with pre-buffered body
			result, err := translator.PreparePassthrough(bodyBytes, req, mockLookup)

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

// TestPreparePassthrough_OversizedBody tests that PreparePassthrough rejects bodies exceeding the configured limit.
// With bodyBytes passed directly, the translator enforces its own size cap rather than relying on LimitReader.
func TestPreparePassthrough_OversizedBody(t *testing.T) {
	const maxSize = 100

	cfg := config.AnthropicTranslatorConfig{
		Enabled:            true,
		MaxMessageSize:     maxSize,
		PassthroughEnabled: true,
	}

	translator := NewTranslator(createTestLogger(), cfg)

	// Build a body that exceeds maxSize
	oversizedBody := make([]byte, maxSize+1)
	for i := range oversizedBody {
		oversizedBody[i] = 'x'
	}

	req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", http.NoBody)
	require.NoError(t, err)

	mockLookup := newMockProfileLookup()

	result, err := translator.PreparePassthrough(oversizedBody, req, mockLookup)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request body exceeds maximum size")
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

	bodyBytes := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Test with inspector"}
		]
	}`)

	req, err := http.NewRequest("POST", "/olla/anthropic/v1/messages", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-Session-ID", "test-session-123")

	mockLookup := newMockProfileLookup()

	result, err := translator.PreparePassthrough(bodyBytes, req, mockLookup)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Note: We don't verify file creation here as that's an implementation detail
	// The important thing is that PreparePassthrough succeeds with inspector enabled
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
		// The handler filters to only Anthropic-capable backends before calling CanPassthrough.
		// In a mixed vllm+ollama deployment, only vllm-gpu-1 survives the filter.
		// CanPassthrough receives the already-filtered list and returns true.
		endpoints := []*domain.Endpoint{
			{Name: "vllm-gpu-1", Type: "vllm"}, // ollama-cpu-1 already removed by handler filter
		}

		profileLookup := newMockProfileLookup().
			withSupport("vllm", &domain.AnthropicSupportConfig{
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

		assert.True(t, result, "should support passthrough when handler pre-filtered to only capable backends")
	})
}
