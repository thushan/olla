package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

// mockProfileFactory is a test implementation of ProfileFactory
type mockProfileFactory struct {
	validProfiles map[string]bool
	profiles      map[string]domain.InferenceProfile
}

func (m *mockProfileFactory) GetProfile(profileType string) (domain.InferenceProfile, error) {
	if profile, ok := m.profiles[profileType]; ok {
		return profile, nil
	}
	return nil, fmt.Errorf("profile not found")
}

func (m *mockProfileFactory) GetAvailableProfiles() []string {
	profiles := make([]string, 0, len(m.validProfiles))
	for p := range m.validProfiles {
		profiles = append(profiles, p)
	}
	return profiles
}

func (m *mockProfileFactory) ReloadProfiles() error {
	return nil
}

func (m *mockProfileFactory) ValidateProfileType(platformType string) bool {
	// For the mock, just check if it's in valid profiles directly
	return m.validProfiles[platformType]
}

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
	t.Run("fallback without profile factory", func(t *testing.T) {
		// Create a minimal Application with no profile factory to test fallback
		app := &Application{}

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
				result := app.isProviderSupported(tt.provider)
				assert.Equal(t, tt.supported, result)
			})
		}
	})

	t.Run("with profile factory", func(t *testing.T) {
		// Create a mock profile factory
		mockFactory := &mockProfileFactory{
			validProfiles: map[string]bool{
				"ollama":    true,
				"lm-studio": true,
				"openai":    true,
				"vllm":      true,
				"custom":    true, // this wouldn't be in the fallback
			},
		}

		app := &Application{
			profileFactory: mockFactory,
		}

		// Test that custom profile is supported via factory
		assert.True(t, app.isProviderSupported("custom"))

		// Test normalization still works
		assert.True(t, app.isProviderSupported("lmstudio"))
		assert.True(t, app.isProviderSupported("lm_studio"))

		// Test unknown is not supported
		assert.False(t, app.isProviderSupported("unknown"))
	})
}
