package services

import (
	"strings"
	"testing"
)

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
