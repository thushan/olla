package domain_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/core/domain"
)

// TestAuthHintRoundTrip verifies that the auth hint section in a profile YAML
// deserialises correctly and that the zero value is safe (omitempty means the
// hint is simply absent when not configured).
func TestAuthHintRoundTrip(t *testing.T) {
	t.Parallel()

	yamlInput := `
name: test-profile
version: "1.0"
characteristics:
  timeout: 5m
  streaming_support: true
  auth:
    required: false
    types:
      - bearer
      - api_key
    default_header: "X-Api-Key"
`

	var cfg domain.ProfileConfig
	if err := yaml.Unmarshal([]byte(yamlInput), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	hint := cfg.Characteristics.Auth

	if hint.Required {
		t.Error("expected Required=false")
	}
	if len(hint.Types) != 2 {
		t.Errorf("expected 2 auth types, got %d: %v", len(hint.Types), hint.Types)
	}
	if hint.Types[0] != "bearer" {
		t.Errorf("expected Types[0]=bearer, got %q", hint.Types[0])
	}
	if hint.Types[1] != "api_key" {
		t.Errorf("expected Types[1]=api_key, got %q", hint.Types[1])
	}
	if hint.DefaultHeader != "X-Api-Key" {
		t.Errorf("expected DefaultHeader=X-Api-Key, got %q", hint.DefaultHeader)
	}
}

// TestAuthHintAbsent verifies that a profile without an auth section produces
// a zero-value AuthHint, not an error.
func TestAuthHintAbsent(t *testing.T) {
	t.Parallel()

	yamlInput := `
name: minimal-profile
version: "1.0"
characteristics:
  timeout: 2m
  streaming_support: false
`

	var cfg domain.ProfileConfig
	if err := yaml.Unmarshal([]byte(yamlInput), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	hint := cfg.Characteristics.Auth
	if hint.Required {
		t.Error("expected Required=false for absent auth hint")
	}
	if len(hint.Types) != 0 {
		t.Errorf("expected no auth types for absent hint, got %v", hint.Types)
	}
}

// TestAuthHintRequiredFlag verifies the required flag is parsed correctly for
// profiles that mandate authentication (e.g. a cloud API gateway).
func TestAuthHintRequiredFlag(t *testing.T) {
	t.Parallel()

	yamlInput := `
name: cloud-profile
version: "1.0"
characteristics:
  timeout: 1m
  auth:
    required: true
    types:
      - bearer
`

	var cfg domain.ProfileConfig
	if err := yaml.Unmarshal([]byte(yamlInput), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	if !cfg.Characteristics.Auth.Required {
		t.Error("expected Required=true")
	}
	if len(cfg.Characteristics.Auth.Types) != 1 || cfg.Characteristics.Auth.Types[0] != "bearer" {
		t.Errorf("unexpected types: %v", cfg.Characteristics.Auth.Types)
	}
}
