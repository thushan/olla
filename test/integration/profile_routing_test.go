package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry/profile"
)

// TestProfileRoutingPrefixes verifies that the profile factory correctly
// validates provider types using the routing prefixes defined in profiles
func TestProfileRoutingPrefixes(t *testing.T) {
	// Create profile factory which loads profiles with routing prefixes
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	tests := []struct {
		name     string
		provider string
		expected bool
	}{
		// Direct profile names
		{"ollama profile", "ollama", true},
		{"lm-studio profile", "lm-studio", true},
		{"openai-compatible profile", "openai-compatible", true},

		// Routing prefixes for lm-studio (from lmstudio.yaml)
		{"lmstudio prefix", "lmstudio", true},
		{"lm_studio prefix", "lm_studio", true},

		// Routing prefix for openai (from openai.yaml)
		{"openai prefix", "openai", true},

		// Auto profile
		{"auto profile", "auto", true},

		// Invalid providers
		{"unknown provider", "unknown", false},
		{"empty provider", "", false},

		// Case sensitivity test
		{"OLLAMA uppercase", "OLLAMA", false},
		{"LmStudio mixed", "LmStudio", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := profileFactory.ValidateProfileType(tt.provider)
			assert.Equal(t, tt.expected, result,
				"ValidateProfileType(%q) = %v, want %v", tt.provider, result, tt.expected)
		})
	}
}

func TestProfileFactoryIntegration(t *testing.T) {
	// Test that the factory correctly validates all expected provider variations
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	validProviders := []string{
		"ollama",
		"lm-studio",
		"lmstudio",
		"lm_studio",
		"openai",
		"openai-compatible",
		"auto",
	}

	for _, provider := range validProviders {
		t.Run(provider, func(t *testing.T) {
			assert.True(t, profileFactory.ValidateProfileType(provider),
				"Provider %s should be valid", provider)
		})
	}

	invalidProviders := []string{
		"unknown",
		"",
		"not-a-provider",
	}

	for _, provider := range invalidProviders {
		t.Run(provider, func(t *testing.T) {
			assert.False(t, profileFactory.ValidateProfileType(provider),
				"Provider %s should be invalid", provider)
		})
	}
}
