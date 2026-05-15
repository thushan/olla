// Package envresolver expands ${VAR} and ${VAR:-default} placeholders in
// configuration strings using environment variable lookups. It intentionally
// does not support the bare $VAR form: config files often contain literal
// dollar signs (shell scripts, cost strings, regex), and requiring braces
// eliminates ambiguity without meaningful ergonomic cost.
package envresolver

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// tokenPattern matches ${VAR} and ${VAR:-default} — no nesting, no bare $VAR.
var tokenPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Expand replaces every ${VAR} and ${VAR:-default} placeholder in s with its
// resolved value. An unset variable with no default resolves to the empty
// string. Expand never returns an error; use ExpandStrict when a missing
// variable must be fatal.
func Expand(s string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}

	return tokenPattern.ReplaceAllStringFunc(s, func(token string) string {
		expr := token[2 : len(token)-1] // strip ${ and }
		name, fallback, hasFallback := strings.Cut(expr, ":-")

		if v := os.Getenv(name); v != "" {
			return v
		}
		if hasFallback {
			return fallback
		}
		return ""
	})
}

// ExpandStrict is like Expand but returns an error when a placeholder has no
// environment value and no default. The error message names the variable but
// never echoes the surrounding string or any partial value, so secrets in
// adjacent placeholders do not leak into logs.
func ExpandStrict(s string) (string, error) {
	if s == "" || !strings.Contains(s, "${") {
		return s, nil
	}

	var missing []string

	expanded := tokenPattern.ReplaceAllStringFunc(s, func(token string) string {
		expr := token[2 : len(token)-1]
		name, fallback, hasFallback := strings.Cut(expr, ":-")

		if v := os.Getenv(name); v != "" {
			return v
		}
		if hasFallback {
			return fallback
		}
		missing = append(missing, name)
		return ""
	})

	if len(missing) > 0 {
		errs := make([]error, len(missing))
		for i, name := range missing {
			errs[i] = fmt.Errorf("required environment variable %q is not set", name)
		}
		return "", errors.Join(errs...)
	}

	return expanded, nil
}

// ExpandWithFile resolves a config value that may come from either a literal
// string or a file path (the _file sibling-field convention). Callers pass the
// literal value and the file path; exactly one must be non-empty.
//
// When fileValue is set, the file is read and its contents are returned with
// leading/trailing whitespace trimmed. This mirrors the Docker Secrets / k8s
// mounted-secret pattern where a file holds a single secret value.
//
// Both values being non-empty is a configuration error the operator must fix
// before the process starts — this function fails fast so the mistake surfaces
// immediately rather than silently preferring one source.
func ExpandWithFile(value, fileValue string) (string, error) {
	hasValue := value != ""
	hasFile := fileValue != ""

	if hasValue && hasFile {
		return "", errors.New("both value and value_file are set; use exactly one")
	}

	if hasFile {
		raw, err := os.ReadFile(fileValue)
		if err != nil {
			// Report the path but not any partial content.
			return "", fmt.Errorf("reading secret file %q: %w", fileValue, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}

	// Plain value path — still expand any ${VAR} placeholders inside it.
	return Expand(value), nil
}
