package discovery

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// minLeakLen is the shortest fragment of a credential treated as a leak. The
// parser quotes pieces of the input (":secret" in an invalid port message), so
// checking only the whole secret misses partial disclosure.
const minLeakLen = 3

// assertNoCredentialFragments fails if any substring of at least minLeakLen
// characters from any secret appears in got.
func assertNoCredentialFragments(t *testing.T, got string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		for l := minLeakLen; l <= len(secret); l++ {
			for i := 0; i+l <= len(secret); i++ {
				if frag := secret[i : i+l]; strings.Contains(got, frag) {
					t.Errorf("error leaks fragment %q of %q: %s", frag, secret, got)
					return
				}
			}
		}
		// Secrets shorter than the fragment length are checked whole.
		if len(secret) < minLeakLen && strings.Contains(got, secret) {
			t.Errorf("error leaks %q: %s", secret, got)
		}
	}
}

// A URL that fails to parse never reaches the userinfo check in
// validateEndpointConfig, so the parse-failure error is the only thing standing
// between the operator's credentials and the startup log. A username can itself
// be a token, so both halves must stay out of the text, and so must the parser's
// own detail, which quotes the input.
func TestLoadFromConfig_MalformedURLErrorRedactsUserinfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		url     string
		secrets []string
	}{
		{"bad port with user and password", "http://wombat:q7x9kz@host:notaport", []string{"wombat", "q7x9kz"}},
		{"bad port with token as username", "http://tok-4f9a8c1d@host:notaport/v1", []string{"tok-4f9a8c1d"}},
		{"unbracketed ipv6", "http://wombat:q7x9kz@::1:11434", []string{"wombat", "q7x9kz"}},
		{"password with unescaped slash and at", "http://kiwi:gl/ob@z1ng@host:notaport", []string{"kiwi", "gl/ob", "z1ng"}},
		{"userinfo with query", "http://quokka:m4ngo22@host:notaport?x=1", []string{"quokka", "m4ngo22"}},
		{"slash in password, port detail quotes it", "http://kiwi:plover/numbat@host:11434", []string{"kiwi", "plover", "numbat"}},
		{"slash in password, bad port after at", "http://wombat:q7x9kz/kelpie@host:notaport", []string{"wombat", "q7x9kz", "kelpie"}},
		{"question mark in password", "http://kiwi:pl?over@host:11434", []string{"kiwi", "pl?over"}},
		{"hash in password", "http://kiwi:pl#over@host:11434", []string{"kiwi", "pl#over"}},
		{"bad escape in password", "http://kiwi:wo%zzrd@host:11434", []string{"kiwi", "wo%zzrd"}},
		{"at in password", "http://dingo:gu@lah7@host:notaport", []string{"dingo", "gu@lah7"}},
		{"token as username with slash", "http://tok-9b2e7f/x@host:notaport", []string{"tok-9b2e7f"}},
		{"token before the scheme", "tok-9b2e7f@http://host:notaport", []string{"tok-9b2e7f"}},
		{"user and password before the scheme", "wombat:q7x9kz@http://host:notaport", []string{"wombat", "q7x9kz"}},
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
			assertNoCredentialFragments(t, err.Error(), tc.secrets)
		})
	}
}

// Without an '@' there is no userinfo to protect, so the parser's detail is
// still worth showing the operator.
func TestMalformedEndpointURLError_KeepsDetailWithoutUserinfo(t *testing.T) {
	t.Parallel()

	raw := "http://host:notaport"
	_, err := url.Parse(raw)
	if err == nil {
		t.Fatalf("%q should not parse", raw)
	}
	got := malformedEndpointURLError(raw, err).Error()
	if !strings.Contains(got, raw) || !strings.Contains(got, "invalid port") {
		t.Errorf("expected URL and parser detail, got: %s", got)
	}
}

func TestRedactURLUserinfo(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"http://host:8080", "http://host:8080"},
		{"http://u:p@host:8080/v1", "http://<redacted>@host:8080/v1"},
		{"http://tok@host", "http://<redacted>@host"},
		{"u:p@host", "<redacted>@host"},
		// The last '@' wins, wherever the credential's own '?' or '#' fall.
		{"http://kiwi:pl?over@host:11434", "http://<redacted>@host:11434"},
		{"http://kiwi:pl#over@host:11434", "http://<redacted>@host:11434"},
		{"http://kiwi:gl/ob@z1ng@host", "http://<redacted>@host"},
		{"http://u:p@host?a=b@c", "http://<redacted>@c"},
		// A credential ahead of the scheme is not part of the scheme.
		{"tok-9b2e7f@http://host:notaport", "<redacted>@http://host:notaport"},
		{"u:p@http://host:notaport", "<redacted>@http://host:notaport"},
		{"1ttp://u:p@host", "<redacted>@host"},
		{"://u:p@host", "<redacted>@host"},
	}
	for _, tc := range cases {
		if got := redactURLUserinfo(tc.in); got != tc.want {
			t.Errorf("redactURLUserinfo(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
