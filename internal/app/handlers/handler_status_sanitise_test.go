package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FR-14 sanitisation half: the display URL surfaced in JSON must strip
// userinfo and the query/fragment wholesale. The config-validation half
// (rejecting userinfo at load time) lives in internal/adapter/discovery.
func TestSanitiseDisplayURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "userinfo and query stripped",
			in:   "https://alice:s3cr3t@ollama.example.local:11434/v1?token=abc&flag=1",
			want: "https://ollama.example.local:11434/v1",
		},
		{
			name: "fragment stripped",
			in:   "http://localhost:11434/api/tags#section",
			want: "http://localhost:11434/api/tags",
		},
		{
			name: "plain url unchanged",
			in:   "http://localhost:11434",
			want: "http://localhost:11434",
		},
		{
			name: "empty string stays empty",
			in:   "",
			want: "",
		},
		{
			name: "user with no password still stripped",
			in:   "https://token@gpu-host:443/v1",
			want: "https://gpu-host:443/v1",
		},
		{
			name: "query only, no userinfo",
			in:   "http://localhost:8080?x=1&y=2",
			want: "http://localhost:8080",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitiseDisplayURL(tc.in))
		})
	}
}

// Guard against a credential leaking into a sanitised URL even when the input
// is adversarial: the sanitised form must never contain the password.
func TestSanitiseDisplayURL_NeverLeaksCredentials(t *testing.T) {
	const password = "super-secret-token-9f8e7d"
	got := sanitiseDisplayURL("https://user:" + password + "@host:443/v1?k=v")
	assert.NotContains(t, got, password)
	assert.NotContains(t, got, "user")
	assert.NotContains(t, got, "k=v")
}

// C1 regression: a parse error (here caused by a space in the password) must
// fail closed. The previous implementation returned the raw string on parse
// error, leaking credentials into the response JSON when url.Parse rejected
// the input. Config-load validation only catches URLs that parse, so an
// unparseable credentialed URL reaches this path. A non-empty sentinel must
// be returned so the operator sees a diagnosable placeholder, never the raw
// input.
func TestSanitiseDisplayURL_FailClosedOnParseError(t *testing.T) {
	const raw = "http://user:p a s s@host/v1"
	got := sanitiseDisplayURL(raw)

	assert.NotEqual(t, raw, got, "raw credentialed URL must not be returned verbatim on parse error")
	assert.NotContains(t, got, "user", "userinfo must not leak through parse-error path")
	assert.NotContains(t, got, "p a s s", "password must not leak through parse-error path")
	assert.NotEmpty(t, got, "sentinel must be non-empty so the operator sees a diagnosable placeholder")
}
