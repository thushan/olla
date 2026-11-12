package constants_test

import (
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

func TestOMLXProviderConstants(t *testing.T) {
	t.Parallel()

	t.Run("provider type constant", func(t *testing.T) {
		t.Parallel()
		expected := "omlx"
		if constants.ProviderTypeOMLX != expected {
			t.Errorf("ProviderTypeOMLX: expected %q, got %q", expected, constants.ProviderTypeOMLX)
		}
	})

	t.Run("display name constant", func(t *testing.T) {
		t.Parallel()
		expected := "oMLX"
		if constants.ProviderDisplayOMLX != expected {
			t.Errorf("ProviderDisplayOMLX: expected %q, got %q", expected, constants.ProviderDisplayOMLX)
		}
	})

	t.Run("routing prefix", func(t *testing.T) {
		t.Parallel()
		expected := "omlx"
		if constants.ProviderPrefixOMLX != expected {
			t.Errorf("ProviderPrefixOMLX: expected %q, got %q", expected, constants.ProviderPrefixOMLX)
		}
	})

	t.Run("type and prefix match", func(t *testing.T) {
		t.Parallel()
		// oMLX uses a single prefix that matches the type directly — no alias variations needed
		if constants.ProviderTypeOMLX != constants.ProviderPrefixOMLX {
			t.Errorf("ProviderTypeOMLX %q should equal ProviderPrefixOMLX %q", constants.ProviderTypeOMLX, constants.ProviderPrefixOMLX)
		}
	})
}

func TestRapidMLXProviderConstants(t *testing.T) {
	t.Parallel()

	t.Run("provider type constant", func(t *testing.T) {
		t.Parallel()
		expected := "rapid-mlx"
		if constants.ProviderTypeRapidMLX != expected {
			t.Errorf("ProviderTypeRapidMLX: expected %q, got %q", expected, constants.ProviderTypeRapidMLX)
		}
	})

	t.Run("display name constant", func(t *testing.T) {
		t.Parallel()
		expected := "Rapid-MLX"
		if constants.ProviderDisplayRapidMLX != expected {
			t.Errorf("ProviderDisplayRapidMLX: expected %q, got %q", expected, constants.ProviderDisplayRapidMLX)
		}
	})

	t.Run("routing prefix", func(t *testing.T) {
		t.Parallel()
		expected := "rapid-mlx"
		if constants.ProviderPrefixRapidMLX != expected {
			t.Errorf("ProviderPrefixRapidMLX: expected %q, got %q", expected, constants.ProviderPrefixRapidMLX)
		}
	})

	t.Run("type and prefix match", func(t *testing.T) {
		t.Parallel()
		// Rapid-MLX uses a single prefix that matches the type directly: no alias variations needed
		if constants.ProviderTypeRapidMLX != constants.ProviderPrefixRapidMLX {
			t.Errorf("ProviderTypeRapidMLX %q should equal ProviderPrefixRapidMLX %q", constants.ProviderTypeRapidMLX, constants.ProviderPrefixRapidMLX)
		}
	})
}

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
