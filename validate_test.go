package main

import (
	"errors"
	"strings"
	"testing"
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
