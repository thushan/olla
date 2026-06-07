package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAuthConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("bearer", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:  "bearer",
			Token: "${TOKEN}",
		}
		roundTrip(t, original, &AuthConfig{})
	})

	t.Run("bearer with file", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:      "bearer",
			TokenFile: "/run/secrets/token",
		}
		roundTrip(t, original, &AuthConfig{})
	})

	t.Run("api_key with custom header", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:   "api_key",
			Key:    "${API_KEY}",
			Header: "X-Custom-Key",
		}
		roundTrip(t, original, &AuthConfig{})
	})

	t.Run("api_key with file", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:    "api_key",
			KeyFile: "/run/secrets/key",
		}
		roundTrip(t, original, &AuthConfig{})
	})

	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:     "basic",
			Username: "user",
			Password: "pass",
		}
		roundTrip(t, original, &AuthConfig{})
	})

	t.Run("basic with files", func(t *testing.T) {
		t.Parallel()
		original := &AuthConfig{
			Type:         "basic",
			UsernameFile: "/run/secrets/user",
			PasswordFile: "/run/secrets/pass",
		}
		roundTrip(t, original, &AuthConfig{})
	})
}

func TestEndpointConfig_AuthAndHeaders_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("full endpoint with auth and headers", func(t *testing.T) {
		t.Parallel()
		original := &EndpointConfig{
			Name: "secure-llama",
			URL:  "http://llamabox.local:8080",
			Type: "llamacpp",
			Auth: &AuthConfig{
				Type:   "bearer",
				Token:  "${TOKEN}",
				Header: "X-Api-Key",
			},
			Headers: map[string]string{
				"X-Custom":  "value",
				"X-Another": "other",
			},
		}
		roundTrip(t, original, &EndpointConfig{})
	})

	t.Run("endpoint without auth or headers is backwards compatible", func(t *testing.T) {
		t.Parallel()
		yamlIn := `
name: plain
url: http://localhost:11434
type: ollama
`
		var got EndpointConfig
		if err := yaml.Unmarshal([]byte(yamlIn), &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got.Auth != nil {
			t.Errorf("Auth should be nil when not present in YAML, got %+v", got.Auth)
		}
		if got.Headers != nil {
			t.Errorf("Headers should be nil when not present in YAML, got %+v", got.Headers)
		}
		if got.Name != "plain" || got.URL != "http://localhost:11434" || got.Type != "ollama" {
			t.Errorf("base fields not parsed correctly: %+v", got)
		}
	})

	t.Run("headers map with multiple entries round-trips", func(t *testing.T) {
		t.Parallel()
		original := &EndpointConfig{
			Name: "ep",
			Headers: map[string]string{
				"X-Tenant":  "acme",
				"X-Region":  "us-east",
				"X-Version": "2",
			},
		}
		roundTrip(t, original, &EndpointConfig{})
	})
}

// roundTrip marshals src to YAML and unmarshals into dst, then asserts deep equality.
func roundTrip[T any](t *testing.T, src *T, dst *T) {
	t.Helper()
	data, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(src, dst) {
		t.Errorf("round-trip mismatch\n  got:  %+v\n  want: %+v", dst, src)
	}
}
