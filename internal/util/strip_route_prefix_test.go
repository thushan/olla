package util

import (
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

// Characterisation of StripRoutePrefix, the lookup the proxy handler drives
// with its route-prefix context key. The lookup key is the typed route-prefix
// key inside the function; these rows pin the stripping behaviour for the
// value present, absent, and non-matching cases.
func TestStripRoutePrefix_PresentAbsentAndNoMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctxValue string // empty means no value installed
		path     string
		want     string
	}{
		{"value present strips the prefix", "/olla/proxy/", "/olla/proxy/v1/chat/completions", "/v1/chat/completions"},
		{"value present restores the leading slash", "/olla/proxy", "/olla/proxy/v1/chat/completions", "/v1/chat/completions"},
		{"value present path without prefix unchanged", "/olla/proxy/", "/v1/chat/completions", "/v1/chat/completions"},
		{"no value leaves the path unchanged", "", "/olla/proxy/v1/chat/completions", "/olla/proxy/v1/chat/completions"},
		{"empty path with matching prefix becomes root", "/olla/proxy", "/olla/proxy", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			if tt.ctxValue != "" {
				ctx = constants.WithRoutePrefix(ctx, tt.ctxValue)
			}
			if got := StripRoutePrefix(ctx, tt.path); got != tt.want {
				t.Errorf("StripRoutePrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
