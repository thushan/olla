package config

import (
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestServerConfig_GetAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		want     string
		wantHost string
	}{
		{"ipv4 any", "0.0.0.0", "0.0.0.0:40114", "0.0.0.0"},
		{"hostname", "localhost", "localhost:40114", "localhost"},
		{"empty host binds all interfaces", "", ":40114", ""},
		{"bare ipv6 any", "::", "[::]:40114", "::"},
		{"bracketed ipv6 any", "[::]", "[::]:40114", "::"},
		{"bare ipv6 loopback", "::1", "[::1]:40114", "::1"},
		{"bracketed ipv6 loopback", "[::1]", "[::1]:40114", "::1"},
		{"zoned link-local", "fe80::1%eth0", "[fe80::1%eth0]:40114", "fe80::1%eth0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := ServerConfig{Host: tt.host, Port: 40114}
			got := cfg.GetAddress()
			assert.Equal(t, tt.want, got)

			host, port, err := net.SplitHostPort(got)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, "40114", port)
		})
	}
}

// Binds for real: the unbracketed "::1" used to yield ":::0" and fail with
// "too many colons in address".
func TestServerConfig_GetAddress_BindsIPv6Loopback(t *testing.T) {
	t.Parallel()

	cfg := ServerConfig{Host: "::1", Port: 0}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", cfg.GetAddress())
	if err != nil {
		probe, perr := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "[::1]:0")
		if perr != nil {
			t.Skip("no IPv6 loopback on this host")
		}
		_ = probe.Close()
		require.NoError(t, err)
	}
	require.NoError(t, ln.Close())
}
