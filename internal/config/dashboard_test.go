package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestDashboardConfig_Validate_DisabledShortCircuits(t *testing.T) {
	t.Parallel()

	c := DashboardConfig{Enabled: false}
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled dashboard must validate clean, got %v", err)
	}
	if c.ParsedCIDRs() != nil {
		t.Fatalf("disabled dashboard must not populate parsed CIDRs, got %v", c.ParsedCIDRs())
	}
}

func TestDashboardConfig_Validate_EnabledRequiresCIDRs(t *testing.T) {
	t.Parallel()

	c := DashboardConfig{
		Enabled:      true,
		AccessPolicy: AccessPolicyConfig{AllowedHosts: []string{"localhost"}},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "access_policy.allowed_cidrs must not be empty") {
		t.Fatalf("expected error naming allowed_cidrs, got %v", err)
	}
	if !strings.Contains(err.Error(), "dashboard config invalid") {
		t.Fatalf("expected root-level wrap marker, got %v", err)
	}
}

// TestDashboardConfig_Validate_EmptyHostsAllowed confirms there is no
// "hosts must be non-empty" rule: IP-literal Hosts are always accepted,
// so an empty allowed_hosts is a legitimate config.
func TestDashboardConfig_Validate_EmptyHostsAllowed(t *testing.T) {
	t.Parallel()

	c := DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8"},
			AllowedHosts: nil,
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("IP-literal hosts are auto-accepted, so empty allowed_hosts must validate clean, got %v", err)
	}
}

func TestDashboardConfig_Validate_BadCIDRFailsStartup(t *testing.T) {
	t.Parallel()

	c := DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "not-a-cidr"},
			AllowedHosts: []string{"localhost"},
		},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), `invalid access_policy.allowed_cidrs entry "not-a-cidr"`) {
		t.Fatalf("expected wrapped parse error, got %v", err)
	}
	if !strings.Contains(err.Error(), "dashboard config invalid") {
		t.Fatalf("expected root-level wrap marker, got %v", err)
	}
}

func TestDashboardConfig_Validate_PopulatesParsedCIDRs(t *testing.T) {
	t.Parallel()

	c := DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "::1/128"},
			AllowedHosts: []string{"localhost"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected clean validate, got %v", err)
	}
	parsed := c.ParsedCIDRs()
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed CIDRs, got %d", len(parsed))
	}
}

func TestRootValidate_DashboardInvalidFailsStartup(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Dashboard = DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "still-not-a-cidr"},
			AllowedHosts: []string{"localhost"},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "dashboard config invalid") {
		t.Fatalf("expected root Validate to wrap dashboard error, got %v", err)
	}
}

func TestDefaultConfig_DashboardLoopbackDefault(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if !cfg.Dashboard.Enabled {
		t.Fatal("dashboard must default to enabled per spec")
	}
	if cfg.Dashboard.GateInternalAPI {
		t.Fatal("gate_internal_api must default to false to avoid breaking existing monitoring scrapes")
	}
	ap := cfg.Dashboard.AccessPolicy
	if len(ap.AllowedCIDRs) != 2 ||
		ap.AllowedCIDRs[0] != "127.0.0.0/8" ||
		ap.AllowedCIDRs[1] != "::1/128" {
		t.Fatalf("default CIDRs must be loopback-only, got %v", ap.AllowedCIDRs)
	}
	// "localhost" must be present: it's a hostname, not an IP literal, so
	// the auto-accept for IP-literal Hosts doesn't cover it, and DefaultHost is itself
	// "localhost" - a no-config-file startup (go install, curl installer)
	// must not 403 its own default. This was finding 9: the shipped
	// config/config.yaml always carried "localhost" but DefaultConfig() did
	// not, so only a config-file-less startup hit the bug.
	if len(ap.AllowedHosts) != 1 || ap.AllowedHosts[0] != "localhost" {
		t.Fatalf(`default allowed_hosts must be exactly ["localhost"], got %v`, ap.AllowedHosts)
	}
	if err := cfg.Dashboard.Validate(); err != nil {
		t.Fatalf("shipped default must validate clean: %v", err)
	}
}

// captureSlog swaps slog.Default() for a text handler writing to a buffer, so
// a test can assert on what startup validation logs. Returns a restore func.
// Not parallel-safe: callers must not mark these tests t.Parallel() because
// they mutate process-global logger state.
func captureSlog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	return &buf, func() { slog.SetDefault(prev) }
}

// Setting GateInternalAPI=true is inert for now (the wrapping isn't
// implemented yet), so the operator deserves a startup warning naming the
// field. Without it, an operator who sets the flag sees silent acceptance
// and assumes the gate is active, then is surprised when /internal/* stays open.
func TestDashboardConfig_Validate_GateInternalAPIWarnsWhenSetTrue(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	c := DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8"},
		},
		GateInternalAPI: true,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(buf.String(), "gate_internal_api") {
		t.Fatalf("expected warning mentioning gate_internal_api when set true, got: %q", buf.String())
	}
}

// C2 negative: when GateInternalAPI is false (the default), no warning fires.
// The warning is specifically about the inert-but-set case, not the default.
func TestDashboardConfig_Validate_NoGateWarnWhenFalse(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	c := DashboardConfig{
		Enabled: true,
		AccessPolicy: AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8"},
		},
		GateInternalAPI: false,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if strings.Contains(buf.String(), "gate_internal_api") {
		t.Fatalf("expected no gate_internal_api warning when false, got: %q", buf.String())
	}
}
