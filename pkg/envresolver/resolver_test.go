package envresolver

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTemp creates a temp file with the given content and returns its path.
// The file is automatically removed when t completes.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "envresolver-*.txt")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

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

// TestExpand_LookupEnvSemantics covers the distinction between unset and
// explicitly-empty variables. Subtests do not call t.Parallel() because
// t.Setenv panics when combined with t.Parallel() in Go's test runner.
func TestExpand_LookupEnvSemantics(t *testing.T) {
	t.Run("unset_var_no_default_resolves_to_empty", func(t *testing.T) {
		// Lenient Expand never errors; unset resolves to "".
		got := Expand("${OLLA_LOOKUP_UNSET_XYZ_ABC}")
		assert.Equal(t, "", got)
	})

	t.Run("explicit_empty_no_default_resolves_to_empty", func(t *testing.T) {
		t.Setenv("OLLA_LOOKUP_EXPLICIT_EMPTY", "")
		got := Expand("${OLLA_LOOKUP_EXPLICIT_EMPTY}")
		assert.Equal(t, "", got)
	})

	t.Run("default_used_when_var_unset", func(t *testing.T) {
		got := Expand("${OLLA_LOOKUP_UNSET_FOR_DEFAULT_XYZ:-mydefault}")
		assert.Equal(t, "mydefault", got)
	})

	t.Run("default_used_when_var_explicit_empty", func(t *testing.T) {
		// POSIX :- treats empty the same as unset; the default wins.
		t.Setenv("OLLA_LOOKUP_EMPTY_DEFAULT", "")
		got := Expand("${OLLA_LOOKUP_EMPTY_DEFAULT:-fallback}")
		assert.Equal(t, "fallback", got)
	})

	t.Run("default_not_used_when_var_non_empty", func(t *testing.T) {
		t.Setenv("OLLA_LOOKUP_NON_EMPTY", "real-value")
		got := Expand("${OLLA_LOOKUP_NON_EMPTY:-ignored}")
		assert.Equal(t, "real-value", got)
	})
}

// TestExpandStrict_LookupEnvSemantics covers the ExpandStrict distinction
// between unset (fatal) and explicitly-empty (allowed; downstream concern).
// Subtests do not call t.Parallel() for the same reason as TestExpand.
func TestExpandStrict_LookupEnvSemantics(t *testing.T) {
	t.Run("unset_var_returns_error", func(t *testing.T) {
		_, err := ExpandStrict("${OLLA_STRICT_LOOKUP_UNSET_XYZ_ABC}")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OLLA_STRICT_LOOKUP_UNSET_XYZ_ABC")
	})

	t.Run("explicit_empty_no_error", func(t *testing.T) {
		// An explicitly set-but-empty variable is not a missing variable;
		// the downstream caller validates whether empty is acceptable.
		t.Setenv("OLLA_STRICT_LOOKUP_EMPTY", "")
		got, err := ExpandStrict("${OLLA_STRICT_LOOKUP_EMPTY}")
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})
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

// --- ExpandWithFile ---

func TestExpandWithFile(t *testing.T) {
	t.Run("both_set_is_error", func(t *testing.T) {
		_, err := ExpandWithFile("direct-value", "/some/file")
		require.Error(t, err)
		// Neither the value nor the path must appear — the value may be a
		// secret and the path leaks config structure.
		assert.NotContains(t, err.Error(), "direct-value")
		assert.NotContains(t, err.Error(), "/some/file")
	})

	t.Run("neither_set_returns_empty", func(t *testing.T) {
		got, err := ExpandWithFile("", "")
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("plain_value_returned", func(t *testing.T) {
		got, err := ExpandWithFile("api-key-value", "")
		require.NoError(t, err)
		assert.Equal(t, "api-key-value", got)
	})

	t.Run("plain_value_with_placeholder_expanded", func(t *testing.T) {
		t.Setenv("OLLA_FILE_TEST_KEY", "expanded-key")
		got, err := ExpandWithFile("${OLLA_FILE_TEST_KEY}", "")
		require.NoError(t, err)
		assert.Equal(t, "expanded-key", got)
	})

	t.Run("file_value_read_and_returned", func(t *testing.T) {
		f := writeTemp(t, "my-secret-token\n")
		got, err := ExpandWithFile("", f)
		require.NoError(t, err)
		assert.Equal(t, "my-secret-token", got)
	})

	t.Run("file_trailing_newline_trimmed", func(t *testing.T) {
		f := writeTemp(t, "  token-with-spaces  \n")
		got, err := ExpandWithFile("", f)
		require.NoError(t, err)
		assert.Equal(t, "token-with-spaces", got)
	})

	t.Run("file_no_trailing_newline_still_works", func(t *testing.T) {
		f := writeTemp(t, "bare-token")
		got, err := ExpandWithFile("", f)
		require.NoError(t, err)
		assert.Equal(t, "bare-token", got)
	})

	t.Run("missing_file_returns_error", func(t *testing.T) {
		_, err := ExpandWithFile("", "/tmp/olla-envresolver-does-not-exist-xyz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "olla-envresolver-does-not-exist-xyz")
	})

	t.Run("file_permission_denied", func(t *testing.T) {
		// chmod-based permission denial is not reliably enforceable on Windows —
		// the process owner can still read files they own regardless of mode bits.
		if isWindows() {
			t.Skip("permission simulation not supported on Windows")
		}
		f := writeTemp(t, "secret")
		require.NoError(t, os.Chmod(f, 0o000))
		t.Cleanup(func() { os.Chmod(f, 0o600) })

		_, err := ExpandWithFile("", f)
		require.Error(t, err)
	})
}
