package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeProviderType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// lm-studio variations
		{"lmstudio to lm-studio", "lmstudio", "lm-studio"},
		{"lm_studio to lm-studio", "lm_studio", "lm-studio"},
		{"lm-studio unchanged", "lm-studio", "lm-studio"},
		{"LMStudio to lm-studio", "LMStudio", "lm-studio"},
		{"LM_STUDIO to lm-studio", "LM_STUDIO", "lm-studio"},

		// openai variations
		{"openai-compatible unchanged", "openai-compatible", "openai-compatible"},
		{"OpenAI to openai", "OpenAI", "openai"},

		// others lowercase
		{"Ollama to ollama", "Ollama", "ollama"},
		{"VLLM to vllm", "VLLM", "vllm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormaliseProviderType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractProviderFromPath(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		expectedProvider string
		expectedPath     string
		expectedOk       bool
	}{
		{"lmstudio normalized", "/olla/lmstudio/v1/models", "lm-studio", "/v1/models", true},
		{"lm-studio direct", "/olla/lm-studio/v1/models", "lm-studio", "/v1/models", true},
		{"lm_studio normalized", "/olla/lm_studio/v1/models", "lm-studio", "/v1/models", true},
		{"ollama", "/olla/ollama/api/tags", "ollama", "/api/tags", true},
		{"openai", "/olla/openai/v1/completions", "openai", "/v1/completions", true},
		{"invalid path", "/proxy/something", "", "", false},
		{"no trailing path", "/olla/ollama", "ollama", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, path, ok := extractProviderFromPath(tt.path)
			assert.Equal(t, tt.expectedOk, ok)
			if ok {
				assert.Equal(t, tt.expectedProvider, provider)
				assert.Equal(t, tt.expectedPath, path)
			}
		})
	}
}

func TestIsProviderSupported(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		supported bool
	}{
		{"ollama supported", "ollama", true},
		{"lmstudio supported", "lmstudio", true},
		{"lm-studio supported", "lm-studio", true},
		{"lm_studio supported", "lm_studio", true},
		{"openai supported", "openai", true},
		{"vllm supported", "vllm", true},
		{"unknown not supported", "unknown", false},
		{"empty not supported", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isProviderSupported(tt.provider)
			assert.Equal(t, tt.supported, result)
		})
	}
}
