package constants_test

import (
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

func TestLlamaCppProviderConstants(t *testing.T) {
	t.Run("provider type constant", func(t *testing.T) {
		expected := "llamacpp"
		if constants.ProviderTypeLlamaCpp != expected {
			t.Errorf("ProviderTypeLlamaCpp: expected %q, got %q", expected, constants.ProviderTypeLlamaCpp)
		}
	})

	t.Run("display name constant", func(t *testing.T) {
		expected := "llama.cpp"
		if constants.ProviderDisplayLlamaCpp != expected {
			t.Errorf("ProviderDisplayLlamaCpp: expected %q, got %q", expected, constants.ProviderDisplayLlamaCpp)
		}
	})

	t.Run("routing prefix variations", func(t *testing.T) {
		tests := []struct {
			name     string
			constant string
			expected string
		}{
			{"primary prefix", constants.ProviderPrefixLlamaCpp1, "llamacpp"},
			{"hyphenated prefix", constants.ProviderPrefixLlamaCpp2, "llama-cpp"},
			{"underscored prefix", constants.ProviderPrefixLlamaCpp3, "llama_cpp"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.constant != tt.expected {
					t.Errorf("%s: expected %q, got %q", tt.name, tt.expected, tt.constant)
				}
			})
		}
	})
}
