package util

import (
	"context"
	"testing"
)

// Characterisation of StripRoutePrefix, the lookup the proxy handler drives
// with its route-prefix context key. The key is passed in as a plain string
// today; callers own the pairing of writer and lookup key. These rows pin the
// stripping behaviour for the value present, absent, and non-matching cases.
func TestStripRoutePrefix_PresentAbsentAndNoMatch(t *testing.T) {
	t.Parallel()

	// A variable key on purpose: the production caller passes its own constant,
	// and the pairing is theirs, not this function's.
	lookupKey := "route_prefix"

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
				ctx = context.WithValue(ctx, lookupKey, tt.ctxValue)
			}
			if got := StripRoutePrefix(ctx, tt.path, lookupKey); got != tt.want {
				t.Errorf("StripRoutePrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
