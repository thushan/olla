package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

func TestBuildValidationReport_AllClean(t *testing.T) {
	t.Parallel()

	items := []validationItem{
		{Name: "config.yaml", Detail: "loaded from config/config.yaml"},
		{Name: "models.yaml", Detail: "loaded from config/models.yaml"},
		{Name: "profiles", Detail: "9 profile(s) loaded"},
	}

	report, exitCode := buildValidationReport(items)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for a fully clean report, got %d", exitCode)
	}
	if !strings.Contains(report, "[OK]   config.yaml") {
		t.Errorf("expected an OK line for config.yaml, got:\n%s", report)
	}
	if !strings.Contains(report, "Result: PASS") {
		t.Errorf("expected a PASS result line, got:\n%s", report)
	}
	if strings.Contains(report, "FAIL") || strings.Contains(report, "WARN") {
		t.Errorf("clean items should not produce any FAIL/WARN lines, got:\n%s", report)
	}
}

func TestBuildValidationReport_FatalErrorFailsAndExitsNonZero(t *testing.T) {
	t.Parallel()

	items := []validationItem{
		{Name: "config.yaml", Err: errors.New("failed to parse config.yaml: yaml: line 3: bad indentation")},
	}

	report, exitCode := buildValidationReport(items)

	if exitCode == 0 {
		t.Error("expected a non-zero exit code when a config source has a fatal error")
	}
	if !strings.Contains(report, "[FAIL] config.yaml") {
		t.Errorf("expected a FAIL line naming config.yaml, got:\n%s", report)
	}
	if !strings.Contains(report, "bad indentation") {
		t.Errorf("expected the underlying error text in the report, got:\n%s", report)
	}
	if !strings.Contains(report, "Result: FAIL") {
		t.Errorf("expected a FAIL result line, got:\n%s", report)
	}
}

func TestBuildValidationReport_WarningsFailAndExitNonZero(t *testing.T) {
	t.Parallel()

	items := []validationItem{
		{Name: "config.yaml", Detail: "loaded from config/config.yaml"},
		{
			Name:     "models.yaml",
			Detail:   "using embedded defaults",
			Warnings: []string{"config/models.yaml: yaml: line 12: found unknown escape character"},
		},
	}

	report, exitCode := buildValidationReport(items)

	// A file that exists but fails to parse must fail the gate, even though
	// the item still has a usable fallback (embedded defaults) - the running
	// server would just warn and carry on, but --validate-config is the
	// stricter check that's supposed to catch this before deployment.
	if exitCode == 0 {
		t.Error("expected a non-zero exit code when a config source has warnings")
	}
	if !strings.Contains(report, "[WARN] models.yaml") {
		t.Errorf("expected a WARN line naming models.yaml, got:\n%s", report)
	}
	if !strings.Contains(report, "found unknown escape character") {
		t.Errorf("expected the warning detail in the report, got:\n%s", report)
	}
	if !strings.Contains(report, "[OK]   config.yaml") {
		t.Errorf("a clean item alongside a warning should still report OK, got:\n%s", report)
	}
}

func TestBuildValidationReport_MultipleWarningsAllListed(t *testing.T) {
	t.Parallel()

	items := []validationItem{
		{
			Name:     "profiles",
			Detail:   "3 profile(s) loaded",
			Warnings: []string{"config/profiles/broken1.yaml: profile name is required", "config/profiles/broken2.yaml: failed to parse profile YAML"},
		},
	}

	report, exitCode := buildValidationReport(items)

	if exitCode == 0 {
		t.Error("expected a non-zero exit code when warnings are present")
	}
	if !strings.Contains(report, "broken1.yaml") || !strings.Contains(report, "broken2.yaml") {
		t.Errorf("expected both warning details listed, got:\n%s", report)
	}
	if !strings.Contains(report, "(2 issue(s))") {
		t.Errorf("expected the warning count in the summary line, got:\n%s", report)
	}
}

// TestRunValidateConfig_ShippedConfigsPass is an end-to-end smoke test
// against the repo's real shipped config, models.yaml and profiles - the
// same files the running server would load. Skipped if run from somewhere
// config/config.yaml can't be found (e.g. `go test ./...` from a different
// working directory), since runValidateConfig deliberately uses the same
// relative-path resolution as production rather than an injectable path.
func TestRunValidateConfig_ShippedConfigsPass(t *testing.T) {
	report, exitCode := runValidateConfig("")

	if exitCode != 0 {
		t.Errorf("expected the shipped repo configs to validate cleanly, got exit code %d:\n%s", exitCode, report)
	}
	if !strings.Contains(report, "Result: PASS") {
		t.Errorf("expected a PASS result for the shipped configs, got:\n%s", report)
	}
}

func TestBuildValidationReport_IgnoredKeysListedWithoutChangingVerdict(t *testing.T) {
	t.Parallel()

	clean := []validationItem{
		{Name: "config.yaml", Detail: "loaded from config.yaml"},
		{Name: "profiles", Detail: "9 profile(s) loaded"},
	}
	withIgnored := []validationItem{
		{Name: "config.yaml", Detail: "loaded from config.yaml", IgnoredIn: "etc/olla.yaml", Ignored: []string{"proxy.retry (line 91)", "server.rate_limits.per_endpoint (line 42)"}},
		{Name: "profiles", Detail: "9 profile(s) loaded"},
	}

	cleanReport, cleanCode := buildValidationReport(clean)
	report, code := buildValidationReport(withIgnored)

	if code != cleanCode || code != 0 {
		t.Errorf("ignored keys must not change the exit code: got %d, clean %d", code, cleanCode)
	}
	if !strings.Contains(report, "Result: PASS") {
		t.Errorf("verdict must stay PASS, got:\n%s", report)
	}
	if strings.Contains(report, "WARN") || strings.Contains(report, "FAIL") {
		t.Errorf("ignored keys must not use the WARN/FAIL path, got:\n%s", report)
	}
	if !strings.Contains(report, "[OK]   config.yaml") {
		t.Errorf("config.yaml should still be OK, got:\n%s", report)
	}
	for _, want := range []string{"Ignored keys in etc/olla.yaml (2)", "proxy.retry (line 91)", "server.rate_limits.per_endpoint (line 42)"} {
		if !strings.Contains(report, want) {
			t.Errorf("expected %q in report:\n%s", want, report)
		}
	}
	if strings.Contains(cleanReport, "Ignored keys") {
		t.Errorf("no section expected when nothing is ignored, got:\n%s", cleanReport)
	}
}

func TestBuildValidationReport_IgnoredKeysDoNotMaskRealFailures(t *testing.T) {
	t.Parallel()

	_, code := buildValidationReport([]validationItem{
		{Name: "config.yaml", Detail: "loaded", IgnoredIn: "c.yaml", Ignored: []string{"a (line 1)"}},
		{Name: "models.yaml", Warnings: []string{"broken"}},
	})
	if code != 1 {
		t.Errorf("a genuine warning must still fail, got exit %d", code)
	}
}

func TestRunValidateConfig_UnknownKeyReportedAndExitUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.yaml")
	if err := os.WriteFile(path, []byte("proxy:\n  bogus_key: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, code := runValidateConfig(path)

	if !strings.Contains(report, "Ignored keys in "+path) {
		t.Errorf("expected the heading to name the real file %s, got:\n%s", path, report)
	}
	if !strings.Contains(report, "proxy.bogus_key (line 2)") {
		t.Errorf("expected the unknown key in the report, got:\n%s", report)
	}
	// The exit code must match the same config without the typo.
	clean := filepath.Join(t.TempDir(), "clean.yaml")
	if err := os.WriteFile(clean, []byte("proxy:\n  engine: olla\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanCode := runValidateConfig(clean)
	if code != cleanCode {
		t.Errorf("exit code changed because of an unknown key: %d vs %d", code, cleanCode)
	}
}

func TestWarnUnknownKeys_SingleLineListingAll(t *testing.T) {
	t.Parallel()

	var calls []string
	warn := func(msg string, args ...any) {
		calls = append(calls, fmt.Sprint(append([]any{msg}, args...)...))
	}

	warnUnknownKeys(warn, nil)
	if len(calls) != 0 {
		t.Fatalf("no keys must log nothing, got %v", calls)
	}

	warnUnknownKeys(warn, []config.UnknownKey{{Path: "proxy.retry", Line: 91}, {Path: "discovery.health_check", Line: 107}, {Path: "a.b", Line: 3}})
	if len(calls) != 1 {
		t.Fatalf("expected exactly one WARN, got %d: %v", len(calls), calls)
	}
	for _, want := range []string{"proxy.retry (line 91)", "discovery.health_check (line 107)", "a.b (line 3)"} {
		if !strings.Contains(calls[0], want) {
			t.Errorf("WARN missing %q: %s", want, calls[0])
		}
	}
}

func TestWarnDeprecatedProxyRetry(t *testing.T) {
	t.Parallel()

	var calls []string
	warn := func(msg string, args ...any) {
		calls = append(calls, fmt.Sprint(append([]any{msg}, args...)...))
	}

	cfg := config.DefaultConfig()
	warnDeprecatedProxyRetry(warn, cfg)
	if len(calls) != 0 {
		t.Fatalf("defaults must not warn, got %v", calls)
	}

	cfg.Proxy.MaxRetries = 3
	cfg.Proxy.RetryBackoff = time.Second
	warnDeprecatedProxyRetry(warn, cfg)
	if len(calls) != 1 {
		t.Fatalf("expected exactly one WARN, got %d: %v", len(calls), calls)
	}
	for _, want := range []string{"proxy.max_retries", "proxy.retry_backoff", "proxy.retry"} {
		if !strings.Contains(calls[0], want) {
			t.Errorf("WARN missing %q: %s", want, calls[0])
		}
	}

	only := config.DefaultConfig()
	only.Proxy.RetryBackoff = time.Second
	if msg := deprecatedProxyRetryFields(only); strings.Contains(msg, "max_retries") || !strings.Contains(msg, "retry_backoff") {
		t.Errorf("only the set key should be named, got %q", msg)
	}
}

func TestRunValidateConfig_NegativeRetryAttemptsFail(t *testing.T) {
	t.Parallel()

	write := func(body string) string {
		path := filepath.Join(t.TempDir(), "olla.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	report, code := runValidateConfig(write("proxy:\n  retry:\n    max_attempts: -1\n"))
	if code == 0 || !strings.Contains(report, "proxy.retry.max_attempts") {
		t.Errorf("negative max_attempts must fail validation (code %d):\n%s", code, report)
	}
}

func TestRunValidateConfig_DeprecatedRetryKeysListedWithoutFailing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "olla.yaml")
	if err := os.WriteFile(path, []byte("proxy:\n  max_retries: 3\n  retry_backoff: 1s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, code := runValidateConfig(path)
	if code != 0 {
		t.Errorf("deprecated keys must not fail validation (code %d):\n%s", code, report)
	}
	for _, want := range []string{"Ignored keys in", "proxy.max_retries", "proxy.retry_backoff"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}
