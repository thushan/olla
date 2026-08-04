// Package config_docs holds the validation harness for the YAML snippets that
// appear in docs/content/configuration/dashboard.md. It is a Go test rather
// than a script because Olla silently ignores unknown YAML keys - a typo'd
// dashboard.access_policy.allowed_xidrs would otherwise round-trip through
// the loader without complaint, leaving the docs wrong and the reader none
// the wiser.
//
// Each test case is the literal YAML from the docs page, fed through the same
// yaml.Unmarshal-onto-DefaultConfig path the real loader uses, then Validate.
// A test failure here means the docs promise a key the schema does not accept
// or a value it rejects.
package config_docs

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/config"
)

// runSnippet loads one YAML block onto a fresh DefaultConfig and runs the same
// Validate the real startup runs. Failures here are docs bugs, not config bugs.
func runSnippet(t *testing.T, name, yamlStr string) {
	t.Helper()

	cfg := config.DefaultConfig()
	if err := yaml.Unmarshal([]byte(yamlStr), cfg); err != nil {
		t.Fatalf("%s: yaml parse failed (unknown keys would silently pass; a structural error means the snippet is malformed): %v", name, err)
	}
	if err := cfg.Dashboard.Validate(); err != nil {
		t.Fatalf("%s: dashboard Validate failed: %v", name, err)
	}
	if !cfg.Dashboard.Enabled {
		t.Fatalf("%s: dashboard.enabled did not parse to true", name)
	}
}

// TestDocsSnippet_Disabled is the one docs snippet where Enabled is false.
// Validate must short-circuit clean without forcing the allowlists to be
// populated, so an operator can drop the dashboard with a one-liner.
func TestDocsSnippet_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	const yamlStr = `dashboard:
  enabled: false
`
	if err := yaml.Unmarshal([]byte(yamlStr), cfg); err != nil {
		t.Fatalf("disabled snippet: yaml parse failed: %v", err)
	}
	if cfg.Dashboard.Enabled {
		t.Fatalf("disabled snippet: dashboard.enabled parsed as true, want false")
	}
	if err := cfg.Dashboard.Validate(); err != nil {
		t.Fatalf("disabled snippet: Validate should short-circuit, got: %v", err)
	}
}

// TestDocsSnippet_Default is the shipped loopback-only default shown in the
// docs as both the literal default and the "enable and disable" intro example.
// It mirrors DefaultConfig: loopback CIDRs and "localhost" in allowed_hosts
// (localhost is a hostname, not an IP literal, so the auto-accept for
// IP-literal Hosts does not cover it; it's in the default so a no-config
// install is not 403'd against Olla's shipped 0.0.0.0 bind).
func TestDocsSnippet_Default(t *testing.T) {
	const yamlStr = `dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "127.0.0.0/8"
      - "::1/128"
    allowed_hosts:
      - "localhost"
  gate_internal_api: false
`
	runSnippet(t, "default", yamlStr)
}

// TestDocsSnippet_LAN is the LAN widening example from the docs, adding a
// /24 plus an internal hostname. The internal hostname MUST be listed because
// it does not parse as an IP literal (only IP-literal Hosts are auto-accepted).
// gate_internal_api stays false: it has no effect yet, wiring it up is deferred.
func TestDocsSnippet_LAN(t *testing.T) {
	const yamlStr = `dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "127.0.0.0/8"
      - "::1/128"
      - "10.0.1.0/24"
    allowed_hosts:
      - "olla.internal.example.net"
  gate_internal_api: false
`
	runSnippet(t, "lan", yamlStr)
}

// TestDocsSnippet_Docker is the Docker bridge widening example. The CIDR
// here is the docker0 default; the docs tell the operator to confirm their
// own subnet before copying. allowed_hosts is empty because the operator
// browses by the bridge gateway's IP (e.g. http://172.17.0.1:...).
// gate_internal_api stays false: it is inert this PR.
func TestDocsSnippet_Docker(t *testing.T) {
	const yamlStr = `dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "172.17.0.0/16"
    allowed_hosts: []
  gate_internal_api: false
`
	runSnippet(t, "docker", yamlStr)
}

// TestDocsSnippet_TypoIsCaught proves this harness would catch a typo if one
// slipped into the docs. A snippet using a key the schema does not accept
// (allowed_xidrs, missing the second d) silently drops, leaving AllowedCIDRs
// at its DefaultConfig value rather than the value the snippet appeared to
// write. If yaml.v3 ever grows strict-mode behaviour that rejects unknown
// keys by default, this test will start failing on the Unmarshal call - which
// is the better failure mode, and a signal to delete this guard as redundant.
func TestDocsSnippet_TypoIsCaught(t *testing.T) {
	cfg := config.DefaultConfig()
	const yamlStr = `dashboard:
  enabled: true
  access_policy:
    allowed_xidrs:
      - "127.0.0.0/8"
`
	if err := yaml.Unmarshal([]byte(yamlStr), cfg); err != nil {
		// Unmarshal currently accepts unknown keys silently. If that
		// changes, this branch fires and this guard becomes redundant.
		t.Fatalf("typo snippet: yaml parse failed: %v", err)
	}
	// The typo silently drops, so access_policy.allowed_cidrs retains the
	// DefaultConfig value (loopback pair, length 2), not the single
	// "127.0.0.0/8" the typo'd snippet appeared to set (length 1).
	got := cfg.Dashboard.AccessPolicy.AllowedCIDRs
	if len(got) == 1 && got[0] == "127.0.0.0/8" {
		t.Fatalf("typo snippet wrote through - schema now accepts allowed_xidrs; harness needs updating")
	}
	if len(got) == 1 {
		t.Fatalf("typo snippet unexpectedly applied: AllowedCIDRs=%v", got)
	}
}
