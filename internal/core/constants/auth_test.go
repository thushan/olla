package constants_test

import (
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

// IsValidAuthType reports whether s is a recognised auth type.
func IsValidAuthType(s string) bool {
	switch s {
	case constants.AuthTypeBearer, constants.AuthTypeAPIKey, constants.AuthTypeBasic:
		return true
	default:
		return false
	}
}

func TestIsValidAuthType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"bearer", constants.AuthTypeBearer, true},
		{"api_key", constants.AuthTypeAPIKey, true},
		{"basic", constants.AuthTypeBasic, true},
		{"empty", "", false},
		{"unknown", "oauth2", false},
		{"case sensitive", "Bearer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsValidAuthType(tt.input)
			if got != tt.want {
				t.Errorf("IsValidAuthType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAuthConstants(t *testing.T) {
	t.Parallel()

	t.Run("auth type values", func(t *testing.T) {
		t.Parallel()
		if constants.AuthTypeBearer != "bearer" {
			t.Errorf("AuthTypeBearer: expected %q, got %q", "bearer", constants.AuthTypeBearer)
		}
		if constants.AuthTypeAPIKey != "api_key" {
			t.Errorf("AuthTypeAPIKey: expected %q, got %q", "api_key", constants.AuthTypeAPIKey)
		}
		if constants.AuthTypeBasic != "basic" {
			t.Errorf("AuthTypeBasic: expected %q, got %q", "basic", constants.AuthTypeBasic)
		}
	})

	t.Run("header names", func(t *testing.T) {
		t.Parallel()
		if constants.AuthHeaderAuthorization != "Authorization" {
			t.Errorf("AuthHeaderAuthorization: expected %q, got %q", "Authorization", constants.AuthHeaderAuthorization)
		}
		if constants.AuthDefaultAPIKeyHeader != "X-Api-Key" {
			t.Errorf("AuthDefaultAPIKeyHeader: expected %q, got %q", "X-Api-Key", constants.AuthDefaultAPIKeyHeader)
		}
	})

	t.Run("scheme prefixes include trailing space", func(t *testing.T) {
		t.Parallel()
		if constants.AuthSchemeBearer != "Bearer " {
			t.Errorf("AuthSchemeBearer: expected %q, got %q", "Bearer ", constants.AuthSchemeBearer)
		}
		if constants.AuthSchemeBasic != "Basic " {
			t.Errorf("AuthSchemeBasic: expected %q, got %q", "Basic ", constants.AuthSchemeBasic)
		}
	})
}
