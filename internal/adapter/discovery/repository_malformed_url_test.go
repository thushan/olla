package discovery

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// A URL that fails to parse never reaches the userinfo check in
// validateEndpointConfig, so the parse-failure error is the only thing standing
// between the operator's credentials and the startup log. A username can itself
// be a token, so both halves must stay out of the text.
func TestLoadFromConfig_MalformedURLErrorRedactsUserinfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		url     string
		secrets []string
	}{
		{"bad port with user and password", "http://alice:s3cr3t@host:notaport", []string{"alice", "s3cr3t"}},
		{"bad port with token as username", "http://tok-4f9a8c1d@host:notaport/v1", []string{"tok-4f9a8c1d"}},
		{"unbracketed ipv6", "http://alice:s3cr3t@::1:11434", []string{"alice", "s3cr3t"}},
		{"password with unescaped slash and at", "http://bob:p/ss@w0rd@host:notaport", []string{"bob", "p/ss", "w0rd"}},
		{"userinfo with query", "http://carol:hunter2@host:notaport?x=1", []string{"carol", "hunter2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := url.Parse(tc.url); err == nil {
				// Go before 1.26 accepts unbracketed IPv6; nothing to redact then.
				t.Skipf("toolchain parses %q, so the malformed-URL path is not reachable", tc.url)
			}

			repo := NewStaticEndpointRepository()
			err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{{
				Name:           "malformed",
				URL:            tc.url,
				Type:           "ollama",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			}})
			if err == nil {
				t.Fatalf("expected an error for %q", tc.url)
			}
			if !strings.Contains(err.Error(), "invalid endpoint URL") {
				t.Errorf("error should still say what is wrong, got: %v", err)
			}
			for _, secret := range tc.secrets {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error must not echo %q, got: %v", secret, err)
				}
			}
		})
	}
}

func TestRedactURLUserinfo(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"http://host:8080", "http://host:8080"},
		{"http://u:p@host:8080/v1", "http://<redacted>@host:8080/v1"},
		{"http://tok@host", "http://<redacted>@host"},
		{"http://u:p@host?a=b@c", "http://<redacted>@host?a=b@c"},
		{"u:p@host", "<redacted>@host"},
	}
	for _, tc := range cases {
		if got := redactURLUserinfo(tc.in); got != tc.want {
			t.Errorf("redactURLUserinfo(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
