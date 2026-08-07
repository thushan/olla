package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// This file holds the dashboard-prefixed tests extracted from
// feature/dashboard-impl's internal/config/shipped_config_test.go. The CB
// tests and the proxyCircuitBreakerNodeFromBytes helper that bundled alongside
// them on that branch are deliberately NOT carried over (simple-dashboard.md
// §5 test dispositions). loadShippedConfig, dashboardNodeFromBytes and
// toYAMLBytes are kept because the three Dashboard tests depend on them.

// repoConfigPath is the path from this package's test working directory
// (internal/config/) to the shipped config.yaml at the repo root.
const repoConfigPath = "../../config/config.yaml"

// repoRootConfigPath is the sibling copy of config.yaml kept at the repo
// root. Load's search order tries config/config.yaml first, so this copy is
// never actually read when config/config.yaml is present - it exists for
// operators who copy just the root file out of a checkout. The drift guard
// below keeps the two byte-identical so which one Load finds never matters.
const repoRootConfigPath = "../../config.yaml"

// TestRootConfig_MatchesShippedConfig is the drift guard between the two
// copies of the default config: config/config.yaml (canonical) and the
// root-level config.yaml (convenience copy for running straight out of a
// checkout). They must stay byte-identical - a divergence here is exactly
// how the F1 blocker happened, where the two copies fell out of sync and
// only one of them got the fix. Mirrors the profile drift guard pattern in
// internal/adapter/registry/profile/builtin_drift_test.go.
func TestRootConfig_MatchesShippedConfig(t *testing.T) {
	t.Parallel()

	shipped, err := os.ReadFile(repoConfigPath)
	require.NoError(t, err, "shipped config/config.yaml must exist")

	root, err := os.ReadFile(repoRootConfigPath)
	require.NoError(t, err, "root config.yaml must exist")

	require.True(t, bytes.Equal(shipped, root),
		"config.yaml drifted from config/config.yaml - keep them byte-identical, edit both together")
}

// loadShippedConfig reads the shipped config/config.yaml via the same
// yaml.Unmarshal-onto-DefaultConfig path the production loader uses. The path
// is resolved against the test's working directory, which go test sets to the
// package directory (internal/config/), so the relative walk lands at the repo
// root's config.yaml regardless of where `go test` is invoked from. Returns the
// decoded Config and the raw bytes so callers can run additional parses.
func loadShippedConfig(t *testing.T) (*Config, []byte) {
	t.Helper()

	abs, err := filepath.Abs(repoConfigPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", repoConfigPath, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read shipped config %s: %v\n"+
			"if you moved config.yaml, update repoConfigPath in this test", abs, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatalf("shipped config.yaml failed to parse: %v", err)
	}
	return cfg, data
}

// dashboardNodeFromBytes parses raw YAML and returns the yaml.Node under the
// top-level `dashboard` key. Returns an error if `dashboard` is absent. Used to
// scope a KnownFields(true) check to just the dashboard section: running it on
// the full Config trips on unrelated legacy keys elsewhere in config.yaml that
// are themselves silent-drop bugs but out of scope here.
func dashboardNodeFromBytes(t *testing.T, data []byte) *yaml.Node {
	t.Helper()

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse config.yaml as yaml.Node: %v", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		t.Fatal("config.yaml parsed to empty document")
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		t.Fatal("config.yaml top level is not a mapping")
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "dashboard" {
			return top.Content[i+1]
		}
	}
	t.Fatal("config.yaml has no top-level `dashboard` key; the shipped default must ship a dashboard block")
	return nil
}

// toYAMLBytes re-encodes a yaml.Node so a scoped KnownFields check can decode
// just that subtree against a fresh struct value.
func toYAMLBytes(t *testing.T, node *yaml.Node) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		t.Fatalf("re-encode dashboard node: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close dashboard encoder: %v", err)
	}
	return buf.Bytes()
}

// TestShippedConfig_DashboardPopulated is the regression guard for the silent
// key-drop class of bug: if dashboard.allowed_cidrs or dashboard.allowed_hosts
// ever appear directly under `dashboard:` instead of nested under
// `access_policy:`, yaml.Unmarshal silently drops them and the running instance
// falls back to DefaultConfig's loopback pair without complaint. The first
// operator to add a real hostname then gets a silent no-op followed by a 403.
// This test fails loudly if that nesting drifts.
func TestShippedConfig_DashboardPopulated(t *testing.T) {
	t.Parallel()

	cfg, _ := loadShippedConfig(t)

	if err := cfg.Dashboard.Validate(); err != nil {
		t.Fatalf("shipped dashboard must validate clean via the production "+
			"DefaultConfig + Unmarshal + Validate path: %v", err)
	}

	if !cfg.Dashboard.Enabled {
		t.Fatal("shipped dashboard.enabled should be true so the dashboard is " +
			"served out of the box (set false explicitly to disable)")
	}

	ap := cfg.Dashboard.AccessPolicy
	if len(ap.AllowedCIDRs) == 0 {
		t.Fatal("shipped dashboard.access_policy.allowed_cidrs is empty after " +
			"loading; the YAML keys are likely misplaced at the dashboard root " +
			"and silently dropped by yaml.Unmarshal")
	}

	// The shipped default must carry loopback: if an operator widens it in
	// config.yaml later, that's a deliberate edit, but the shipped file should
	// still gate to loopback so a fresh checkout is safe by default.
	wantCIDRs := map[string]bool{"127.0.0.0/8": false, "::1/128": false}
	for _, c := range ap.AllowedCIDRs {
		wantCIDRs[c] = true
	}
	for cidr, seen := range wantCIDRs {
		if !seen {
			t.Errorf("shipped allowed_cidrs missing loopback entry %q; got %v",
				cidr, ap.AllowedCIDRs)
		}
	}

	// ParsedCIDRs comes from Validate; assert the parsed entries actually
	// contain the loopback ranges, not just that the strings round-tripped.
	parsed := cfg.Dashboard.ParsedCIDRs()
	if len(parsed) != len(ap.AllowedCIDRs) {
		t.Fatalf("ParsedCIDRs length mismatch: cfg has %d CIDR strings but %d parsed",
			len(ap.AllowedCIDRs), len(parsed))
	}
	parsedHas := func(cidr string) bool {
		for _, n := range parsed {
			if n.String() == cidr {
				return true
			}
		}
		return false
	}
	parsedStrings := make([]string, len(parsed))
	for i, n := range parsed {
		parsedStrings[i] = n.String()
	}
	for cidr := range wantCIDRs {
		if !parsedHas(cidr) {
			t.Errorf("parsed CIDRs missing %q; the Validate step did not populate "+
				"the parsed net for the request path, got %v", cidr, parsedStrings)
		}
	}

	// localhost is a hostname (not an IP literal) and is rejected unless
	// allowlisted, so the shipped default must list it for
	// http://localhost:40114/... to work out of the box. IP-literal loopback
	// Hosts (127.0.0.1, [::1]) need no entry; their presence is harmless but
	// redundant, so we assert only the non-IP name.
	foundLocalhost := false
	for _, h := range ap.AllowedHosts {
		if h == "localhost" {
			foundLocalhost = true
			break
		}
	}
	if !foundLocalhost {
		t.Errorf("shipped allowed_hosts must list %q for browser access via "+
			"localhost URL; got %v", "localhost", ap.AllowedHosts)
	}

	// gate_internal_api defaults to false: turning it on by default would
	// break existing Prometheus scrapes of /internal/metrics on upgrade. The
	// field is inert on this branch regardless (simple-dashboard.md §5.2);
	// this assertion pins the shipped default so a future PR wiring it does
	// not also have to flip the shipped file.
	if cfg.Dashboard.GateInternalAPI {
		t.Error("shipped dashboard.gate_internal_api must be false to avoid " +
			"breaking existing /internal/* monitoring on upgrade")
	}
}

// TestDefaultConfig_MatchesShippedDashboardAccessPolicy is the regression
// guard for the no-config-file 403 class: DefaultConfig() is what a
// no-config-file startup (go install, curl installer, `Load()`'s fallback when
// no file is found) runs with, so it must not silently diverge from the
// dashboard access policy the shipped config/config.yaml documents.
func TestDefaultConfig_MatchesShippedDashboardAccessPolicy(t *testing.T) {
	t.Parallel()

	defaults := DefaultConfig().Dashboard.AccessPolicy
	shipped, _ := loadShippedConfig(t)
	fromFile := shipped.Dashboard.AccessPolicy

	require.ElementsMatch(t, defaults.AllowedCIDRs, fromFile.AllowedCIDRs,
		"DefaultConfig() and config/config.yaml disagree on allowed_cidrs")
	require.ElementsMatch(t, defaults.AllowedHosts, fromFile.AllowedHosts,
		"DefaultConfig() and config/config.yaml disagree on allowed_hosts")
}

// TestShippedConfig_DashboardKnownFields makes silent dashboard key drops
// loud: any field under `dashboard:` that the DashboardConfig schema does not
// accept fails the test. Scoped to the dashboard node only because running
// KnownFields across the whole Config trips on unrelated legacy keys elsewhere
// in config.yaml that are pre-existing silent-drop bugs and out of scope.
func TestShippedConfig_DashboardKnownFields(t *testing.T) {
	t.Parallel()

	_, data := loadShippedConfig(t)
	node := dashboardNodeFromBytes(t, data)

	// Fresh DashboardConfig with no yaml tags expecting extra keys. Decode the
	// dashboard node with KnownFields(true) so any unknown key (a typo'd or
	// misplaced field) is reported.
	var dc DashboardConfig
	dec := yaml.NewDecoder(bytes.NewReader(toYAMLBytes(t, node)))
	dec.KnownFields(true)
	if err := dec.Decode(&dc); err != nil {
		t.Fatalf("shipped dashboard block contains a key the DashboardConfig "+
			"schema does not accept; this is the silent-drop bug class: %v", err)
	}
}
