package envresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Expand ---

// TestExpand covers the core placeholder expansion logic.
// Subtests cannot be t.Parallel() here because t.Setenv is used for env
// isolation — t.Parallel() inside a subtest that calls t.Setenv panics in Go's
// test runner. The parent still runs concurrently with other top-level tests.
func TestExpand(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		input string
		want  string
	}{
		{
			name:  "empty_string_unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "no_placeholders_unchanged",
			input: "plain text",
			want:  "plain text",
		},
		{
			name: "plain_var_expansion",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_TEST_TOKEN", "secret123")
			},
			input: "${OLLA_TEST_TOKEN}",
			want:  "secret123",
		},
		{
			name: "var_embedded_in_text",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_TEST_TOKEN", "abc")
			},
			input: "Bearer ${OLLA_TEST_TOKEN}",
			want:  "Bearer abc",
		},
		{
			name:  "default_used_when_var_unset",
			input: "${OLLA_MISSING_VAR:-my-default}",
			want:  "my-default",
		},
		{
			name: "default_ignored_when_var_set",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_SET_VAR", "real-value")
			},
			input: "${OLLA_SET_VAR:-ignored-default}",
			want:  "real-value",
		},
		{
			name:  "unset_var_with_no_default_resolves_to_empty",
			input: "${OLLA_DEFINITELY_NOT_SET_XYZ}",
			want:  "",
		},
		{
			name: "multiple_placeholders_in_one_string",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_HOST", "localhost")
				t.Setenv("OLLA_PORT", "8080")
			},
			input: "${OLLA_HOST}:${OLLA_PORT}",
			want:  "localhost:8080",
		},
		{
			name: "adjacent_placeholders_no_separator",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_A", "foo")
				t.Setenv("OLLA_B", "bar")
			},
			// Regression guard for token boundary in ReplaceAllStringFunc.
			input: "${OLLA_A}${OLLA_B}",
			want:  "foobar",
		},
		{
			// Nested ${${X}} is not supported. The regex [^}]+ stops at the
			// first closing brace, so it matches ${${OLLA_OUTER} as a token
			// (name = "${OLLA_OUTER", no env var matches → empty) and leaves a
			// trailing "}" literal in the output. Asserted explicitly so this
			// is documented behaviour, not a silent surprise.
			name:  "nested_placeholder_not_supported",
			input: "${${OLLA_OUTER}}",
			want:  "}",
		},
		{
			// $VAR without braces is not expanded. Bare $ is ambiguous in YAML
			// config files (shell scripts, cost strings, regex) so we require
			// the explicit ${VAR} form to avoid false-positive expansions.
			name:  "bare_dollar_var_not_expanded",
			input: "$OLLA_TEST_TOKEN",
			want:  "$OLLA_TEST_TOKEN",
		},
		{
			name:  "string_without_dollar_unchanged",
			input: "no-dollar-sign-here",
			want:  "no-dollar-sign-here",
		},
		{
			// ${VAR:-} with an empty default is a valid way to force empty
			// without triggering ExpandStrict's missing-var error.
			name:  "default_with_empty_value_part",
			input: "${OLLA_MISSING_EMPTY:-}",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			assert.Equal(t, tt.want, Expand(tt.input))
		})
	}
}

// --- ExpandStrict ---

func TestExpandStrict(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T)
		input       string
		want        string
		wantErr     bool
		errContains []string
		errAbsent   []string
	}{
		{
			name: "set_var_expands",
			setup: func(t *testing.T) {
				t.Setenv("OLLA_STRICT_KEY", "strict-val")
			},
			input: "${OLLA_STRICT_KEY}",
			want:  "strict-val",
		},
		{
			name:  "default_used_when_var_unset",
			input: "${OLLA_STRICT_MISSING:-fallback}",
			want:  "fallback",
		},
		{
			name:    "missing_var_returns_error",
			input:   "${OLLA_STRICT_NOT_SET_ZZZ}",
			wantErr: true,
			// Must name the variable so the operator knows what to set.
			errContains: []string{"OLLA_STRICT_NOT_SET_ZZZ"},
			// Must NOT echo the ${...} token — that leaks the unresolved
			// placeholder literally into logs.
			errAbsent: []string{"${"},
		},
		{
			name:        "multiple_missing_vars_all_reported",
			input:       "${OLLA_MISSING_ONE} ${OLLA_MISSING_TWO}",
			wantErr:     true,
			errContains: []string{"OLLA_MISSING_ONE", "OLLA_MISSING_TWO"},
		},
		{
			name:  "empty_string_no_error",
			input: "",
			want:  "",
		},
		{
			name:  "no_placeholders_no_error",
			input: "static-value",
			want:  "static-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			got, err := ExpandStrict(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				for _, s := range tt.errContains {
					assert.Contains(t, err.Error(), s)
				}
				for _, s := range tt.errAbsent {
					assert.NotContains(t, err.Error(), s)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
