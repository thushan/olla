package main

import (
	"fmt"
	"strings"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/config"
)

// validationItem captures the outcome of validating a single configuration
// source. Kept independent of how the report gets formatted (buildValidationReport)
// so the formatting/exit-code logic can be tested without touching the real
// filesystem or config packages.
type validationItem struct {
	Err       error    // the source is entirely unusable
	Name      string   // e.g. "config.yaml"
	Detail    string   // human-readable summary, shown when there's no fatal Err
	Warnings  []string // non-fatal issues - the source is usable, typically via fallback
	IgnoredIn string   // file the ignored keys came from, for the section heading
	Ignored   []string // keys the file sets that nothing reads; reported but never affect the verdict
}

// buildValidationReport formats a human-readable --validate-config summary
// from already-gathered items and decides the process exit code.
//
// A file that exists but doesn't fully parse is treated as a hard-gate
// failure here (exit 1), even though the running server itself would just
// warn and fall back to defaults for the same file - that graceful runtime
// degradation is exactly what --validate-config exists to catch ahead of
// time, not paper over.
func buildValidationReport(items []validationItem) (report string, exitCode int) {
	var buf strings.Builder
	buf.WriteString("Olla Configuration Validation\n")
	buf.WriteString("==============================\n")

	clean := true
	for _, item := range items {
		switch {
		case item.Err != nil:
			fmt.Fprintf(&buf, "[FAIL] %-12s %v\n", item.Name, item.Err)
			clean = false
		case len(item.Warnings) > 0:
			fmt.Fprintf(&buf, "[WARN] %-12s %s (%d issue(s))\n", item.Name, item.Detail, len(item.Warnings))
			for _, w := range item.Warnings {
				fmt.Fprintf(&buf, "         - %s\n", w)
			}
			clean = false
		default:
			fmt.Fprintf(&buf, "[OK]   %-12s %s\n", item.Name, item.Detail)
		}
	}

	// Ignored keys are an advisory only: Olla loads such files fine, so they
	// stay out of the WARN/FAIL path and cannot change the exit code (CI that
	// validates shipped-derived configs must not start failing because of them).
	for _, item := range items {
		if len(item.Ignored) == 0 {
			continue
		}
		fmt.Fprintf(&buf, "\nIgnored keys in %s (%d) - not read by Olla, no effect on the result:\n", item.IgnoredIn, len(item.Ignored))
		for _, k := range item.Ignored {
			fmt.Fprintf(&buf, "         - %s\n", k)
		}
	}

	buf.WriteString("\n")
	if clean {
		buf.WriteString("Result: PASS - all configuration files are valid\n")
		return buf.String(), 0
	}
	buf.WriteString("Result: FAIL - one or more configuration files could not be fully parsed\n")
	return buf.String(), 1
}

// runValidateConfig loads the main config, models.yaml and all profiles the
// same way the running server would, and builds a validation report. Kept
// thin and deliberately not directly unit-tested (it touches the real
// filesystem and package-level singletons in the config/unifier packages) -
// buildValidationReport carries the testable logic, and this function is
// exercised end-to-end by running the binary against the shipped configs.
func runValidateConfig(configFile string) (string, int) {
	var items []validationItem

	cfg, err := config.Load(configFile)
	if err != nil {
		items = append(items, validationItem{Name: "config.yaml", Err: err})
	} else {
		source := cfg.Filename
		if source == "" {
			source = "(none found, using built-in defaults)"
		} else {
			source = "loaded from " + source
		}
		items = append(items, validationItem{Name: "config.yaml", Detail: source, IgnoredIn: cfg.Filename, Ignored: unknownKeyStrings(cfg.UnknownKeys)})
	}

	_, modelErr := unifier.LoadModelConfig()
	modelSource := unifier.ConfigSource()
	if modelSource == "" {
		modelSource = "using embedded defaults"
	} else {
		modelSource = "loaded from " + modelSource
	}
	items = append(items, validationItem{
		Name:     "models.yaml",
		Detail:   modelSource,
		Warnings: unifier.ConfigWarnings(),
		Err:      modelErr,
	})

	profileFactory, profileErr := profile.NewFactoryWithDefaults()
	if profileErr != nil {
		items = append(items, validationItem{Name: "profiles", Err: profileErr})
	} else {
		count := len(profileFactory.GetAvailableProfiles())
		items = append(items, validationItem{
			Name:     "profiles",
			Detail:   fmt.Sprintf("%d profile(s) loaded", count),
			Warnings: profileFactory.LoadWarnings(),
		})
	}

	return buildValidationReport(items)
}

// deprecatedProxyRetryFields names the legacy proxy.max_retries / retry_backoff
// keys when the operator set them, or returns "". Both defaults are zero, so a
// non-zero value can only have come from the file. They were never read, and are
// deliberately not mapped onto proxy.retry: the old default of 3 would silently
// cap failover for anyone who copied it from an example.
func deprecatedProxyRetryFields(cfg *config.Config) string {
	var set []string
	if cfg.Proxy.MaxRetries != 0 {
		set = append(set, "proxy.max_retries")
	}
	if cfg.Proxy.RetryBackoff != 0 {
		set = append(set, "proxy.retry_backoff")
	}
	if len(set) == 0 {
		return ""
	}
	return strings.Join(set, " and ") + " do nothing and will be removed; failover is controlled by proxy.retry (enabled, max_attempts, non_idempotent)"
}

// warnDeprecatedProxyRetry emits one startup WARN when the legacy keys are set.
func warnDeprecatedProxyRetry(warn func(msg string, args ...any), cfg *config.Config) {
	if msg := deprecatedProxyRetryFields(cfg); msg != "" {
		warn("Deprecated config keys have no effect: " + msg)
	}
}

func unknownKeyStrings(keys []config.UnknownKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.String())
	}
	return out
}

// warnUnknownKeys emits a single startup WARN listing every ignored key, so a
// config with many typos is one log line rather than a flood. Silent when the
// config is clean.
func warnUnknownKeys(warn func(msg string, args ...any), keys []config.UnknownKey) {
	if len(keys) == 0 {
		return
	}
	warn("Ignoring unknown keys in config (not read by Olla, check for typos)",
		"count", len(keys), "keys", strings.Join(unknownKeyStrings(keys), ", "))
}
