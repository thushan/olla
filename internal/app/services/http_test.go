package services

import (
	"math"
	"net/http"
	"strings"
	"testing"
)

// TestMaxHeaderBytesFromConfig covers the derivation of http.Server's
// MaxHeaderBytes from server.request_limits.max_header_size: the configured
// value is used verbatim above the floor, zero/negative falls back to Go's
// own default rather than disabling the cap, and a small positive value is
// floored rather than crippling legitimate requests.
func TestMaxHeaderBytesFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure int64
		want      int
	}{
		{"configured value used verbatim", 2 * 1024 * 1024, 2 * 1024 * 1024},
		{"default config value (1MiB)", 1024 * 1024, 1024 * 1024},
		{"zero falls back to Go's default, not disabled", 0, http.DefaultMaxHeaderBytes},
		{"negative falls back to Go's default, not disabled", -1, http.DefaultMaxHeaderBytes},
		{"small positive value floored", 100, minMaxHeaderBytes},
		{"exactly the floor stays unchanged", minMaxHeaderBytes, minMaxHeaderBytes},
		{"huge value clamped to MaxInt on overflow", math.MaxInt64, math.MaxInt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maxHeaderBytesFromConfig(tc.configure)
			if got != tc.want {
				t.Errorf("maxHeaderBytesFromConfig(%d) = %d, want %d", tc.configure, got, tc.want)
			}
		})
	}
}

// TestDashboardStartupLine_AssetsBuilt covers the honest-claim branch: when
// the embedded dist carries real built assets, the startup line says "ready".
// A binary built via `make build` (or a release target) lands here.
func TestDashboardStartupLine_AssetsBuilt(t *testing.T) {
	t.Parallel()

	got := dashboardStartupLine("http://localhost:40114/internal/ui/", true)
	if !strings.Contains(got, "ready") {
		t.Errorf("assets-built line should say ready, got: %q", got)
	}
	if !strings.Contains(got, "http://localhost:40114/internal/ui/") {
		t.Errorf("assets-built line should carry the URL, got: %q", got)
	}
	if strings.Contains(got, "not built") {
		t.Errorf("assets-built line must not claim assets are missing, got: %q", got)
	}
}

// TestDashboardStartupLine_AssetsMissing covers the honest-claim inverse: a
// binary built without `make build-web` (e.g. `go install`) embeds only the
// .gitkeep sentinel, so the dashboard route 503s on every request. The line
// must NOT say "ready" and must name the fix so a failed boot is a copy-paste
// resolution rather than a debugging loop.
func TestDashboardStartupLine_AssetsMissing(t *testing.T) {
	t.Parallel()

	got := dashboardStartupLine("http://localhost:40114/internal/ui/", false)
	if strings.Contains(got, "ready") {
		t.Errorf("assets-missing line must not claim ready, got: %q", got)
	}
	if !strings.Contains(got, "not built") {
		t.Errorf("assets-missing line should explain assets are not built, got: %q", got)
	}
	if !strings.Contains(got, "make build-web") {
		t.Errorf("assets-missing line should name the build-web fix, got: %q", got)
	}
	if !strings.Contains(got, "http://localhost:40114/internal/ui/") {
		t.Errorf("assets-missing line should still carry the URL, got: %q", got)
	}
}
