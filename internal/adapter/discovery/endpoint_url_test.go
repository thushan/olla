package discovery

import (
	"strings"
	"testing"

	"github.com/thushan/olla/internal/config"
)

func TestValidateEndpointURLs(t *testing.T) {
	t.Parallel()

	const good = "http://localhost:11434"

	// Secret markers are distinctive so any 3+ character fragment of them
	// showing up in an error is a leak, not a coincidence with the wording.
	secrets := []string{"q7x9kz", "tok9b2e"}

	tests := []struct {
		name     string
		cfg      config.EndpointConfig
		wantErr  bool
		wantText string // optional substring the error must carry
	}{
		{name: "plain", cfg: config.EndpointConfig{URL: good}},
		{name: "https upper-case scheme", cfg: config.EndpointConfig{URL: "HTTPS://host:8080/v1"}},
		{name: "bracketed ipv6", cfg: config.EndpointConfig{URL: "http://[::1]:11434"}},
		{name: "path overrides", cfg: config.EndpointConfig{URL: good, HealthCheckURL: "/health/100%", ModelURL: "/api/tags"}},
		{name: "absolute override", cfg: config.EndpointConfig{URL: good, ModelURL: "http://other:9000/v1/models"}},

		{name: "credential before scheme", cfg: config.EndpointConfig{URL: "wombat:q7x9kz@http://host:11434"}, wantErr: true},
		{name: "credential before scheme with slashes", cfg: config.EndpointConfig{URL: "wombat://tok9b2e@http://host"}, wantErr: true},
		{name: "userinfo", cfg: config.EndpointConfig{URL: "http://tok9b2e:q7x9kz@host"}, wantErr: true},
		{name: "at sign in path", cfg: config.EndpointConfig{URL: "http://tok9b2e/x@host"}, wantErr: true},
		{name: "escaped at sign", cfg: config.EndpointConfig{URL: "http://host/a%40q7x9kz"}, wantErr: true},
		{name: "escaped at sign lower-case", cfg: config.EndpointConfig{URL: "http://host/a%4Dq7x9kz%40"}, wantErr: true},
		{name: "port-only host", cfg: config.EndpointConfig{URL: "http://:11434"}, wantErr: true},
		{name: "no scheme", cfg: config.EndpointConfig{URL: "localhost:11434"}, wantErr: true, wantText: "http://localhost:11434"},
		{name: "ftp", cfg: config.EndpointConfig{URL: "ftp://host"}, wantErr: true},
		{name: "file", cfg: config.EndpointConfig{URL: "file:///x"}, wantErr: true},
		{name: "no host", cfg: config.EndpointConfig{URL: "http://"}, wantErr: true},
		{name: "unbracketed ipv6", cfg: config.EndpointConfig{URL: "http://::1:11434"}, wantErr: true},
		{name: "empty url", cfg: config.EndpointConfig{}, wantErr: true},

		{name: "health at sign", cfg: config.EndpointConfig{URL: good, HealthCheckURL: "tok9b2e@y"}, wantErr: true},
		{name: "model scheme-relative", cfg: config.EndpointConfig{URL: good, ModelURL: "//host/x"}, wantErr: true},
		{name: "model no leading slash", cfg: config.EndpointConfig{URL: good, ModelURL: "api/tags"}, wantErr: true},
		{name: "model host and port", cfg: config.EndpointConfig{URL: good, ModelURL: "host:11434/x"}, wantErr: true},
		{name: "model at sign", cfg: config.EndpointConfig{URL: good, ModelURL: "/x/q7x9kz@y"}, wantErr: true},
		{name: "model escaped at sign", cfg: config.EndpointConfig{URL: good, ModelURL: "/x%40tok9b2e"}, wantErr: true},
		{name: "model absolute ftp", cfg: config.EndpointConfig{URL: good, ModelURL: "ftp://host/x"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.cfg.Name = "ep1"
			err := ValidateEndpointURLs(tc.cfg)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			assertErrorLeaksNothing(t, err.Error(), tc.wantText, secrets)
		})
	}
}

// assertErrorLeaksNothing checks the endpoint is named, the wanted hint is
// present, and no 3+ character fragment of any secret appears.
func assertErrorLeaksNothing(t *testing.T, got, wantText string, secrets []string) {
	t.Helper()
	if !strings.Contains(got, `"ep1"`) {
		t.Errorf("error should name the endpoint: %s", got)
	}
	if wantText != "" && !strings.Contains(got, wantText) {
		t.Errorf("error should contain %q: %s", wantText, got)
	}
	for _, s := range secrets {
		for n := 3; n <= len(s); n++ {
			for i := 0; i+n <= len(s); i++ {
				if frag := s[i : i+n]; strings.Contains(got, frag) {
					t.Errorf("error leaks %q (from %q): %s", frag, s, got)
					return
				}
			}
		}
	}
}
