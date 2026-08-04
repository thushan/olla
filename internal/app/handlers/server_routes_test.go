package handlers

// Regression coverage for mountDashboard's enabled gate: dashboard.Handler()
// (walking and SHA-256-hashing the whole embedded SPA bundle to build its
// per-asset cache) must only run when the dashboard is actually enabled. It
// used to be evaluated unconditionally as a call argument before
// dashboard.RegisterRoutes checked cfg.Enabled internally, so a disabled
// dashboard paid that cost for nothing.

import (
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/app/handlers/dashboard"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/router"
)

// TestMountDashboard_Disabled_DoesNotConstructHandler substitutes
// dashboardHandlerFactory with a spy to prove it is never invoked when the
// dashboard is disabled, closing the gap a purely route-table-based
// assertion could not: the old code produced the same empty route table
// whether or not it had wastefully built the handler first.
func TestMountDashboard_Disabled_DoesNotConstructHandler(t *testing.T) {
	orig := dashboardHandlerFactory
	t.Cleanup(func() { dashboardHandlerFactory = orig })

	called := false
	dashboardHandlerFactory = func() http.Handler {
		called = true
		return http.NotFoundHandler()
	}

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:        &config.Config{Dashboard: config.DashboardConfig{Enabled: false}},
		logger:        &mockStyledLogger{},
		routeRegistry: reg,
	}
	app.mountDashboard()

	if called {
		t.Fatal("dashboard handler factory must not be invoked when the dashboard is disabled")
	}
	if _, exists := reg.GetRoutes()[dashboard.DashboardRoute]; exists {
		t.Fatal("disabled dashboard must not register /internal/ui/")
	}
}

// TestMountDashboard_Enabled_ConstructsHandler is the positive counterpart:
// an enabled dashboard does invoke the factory exactly once and mounts the
// route, so the guard above isn't trivially satisfied by mountDashboard's
// enabled branch never running at all.
func TestMountDashboard_Enabled_ConstructsHandler(t *testing.T) {
	orig := dashboardHandlerFactory
	t.Cleanup(func() { dashboardHandlerFactory = orig })

	calls := 0
	dashboardHandlerFactory = func() http.Handler {
		calls++
		return http.NotFoundHandler()
	}

	cfg := config.DashboardConfig{
		Enabled: true,
		AccessPolicy: config.AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "::1/128"},
			AllowedHosts: []string{"localhost", "127.0.0.1", "[::1]"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate dashboard cfg: %v", err)
	}

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:        &config.Config{Dashboard: cfg},
		logger:        &mockStyledLogger{},
		routeRegistry: reg,
	}
	app.mountDashboard()

	if calls != 1 {
		t.Fatalf("dashboard handler factory calls = %d, want 1", calls)
	}
	if _, exists := reg.GetRoutes()[dashboard.DashboardRoute]; !exists {
		t.Fatal("enabled dashboard must register /internal/ui/")
	}
}
