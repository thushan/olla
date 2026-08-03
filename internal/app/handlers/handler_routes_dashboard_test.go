package handlers

// Route-mounting tests for the dashboard, scoped to the no-ripple design in
// simple-dashboard.md §5: registerRoutes() keeps its existing return type,
// the dashboard subtree mounts last, a collision logs Error and skips mounting
// rather than halting startup, and the access middleware stays scoped to
// /internal/ui/. The three GateInternalAPI subtests from
// feature/dashboard-impl are deliberately dropped: that wrapping is not
// implemented on this branch (§5.2).

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/app/handlers/dashboard"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/router"
)

// enabledDashboardCfg returns a validated loopback-only DashboardConfig. This
// is the shipped default and the configuration under which any leak of the
// access middleware onto another route would be most visible (loopback-only
// rejects everything we probe with from outside 127.0.0.0/8).
func enabledDashboardCfg(t *testing.T) config.DashboardConfig {
	t.Helper()
	c := config.DashboardConfig{
		Enabled: true,
		AccessPolicy: config.AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "::1/128"},
			AllowedHosts: []string{"localhost", "127.0.0.1", "[::1]"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("validate dashboard cfg: %v", err)
	}
	return c
}

// applicationWithStaticRouteTable builds an Application whose registerRoutes
// call produces the static provider route table (the superset production
// emits when YAML profile loading is unavailable). profileFactory and
// translatorRegistry are deliberately nil so registerProviderRoutes falls
// through to registerStaticProviderRoutes and registerTranslatorRoutes warns
// and returns; translator routes have their own dedicated coverage elsewhere.
func applicationWithStaticRouteTable(t *testing.T, dash config.DashboardConfig) (*Application, *router.RouteRegistry) {
	t.Helper()
	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config: &config.Config{
			Dashboard: dash,
		},
		logger:        &mockStyledLogger{},
		routeRegistry: reg,
	}
	app.registerRoutes()
	return app, reg
}

// TestFullRouteTable_DashboardMiddlewareStaysScoped is the full route table
// loop proving the dashboard access middleware stays scoped to /internal/ui/
// and does not silently attach to any other route. It enumerates every route
// the production registration path emits, replaces each non-dashboard handler
// with a per-route marker, then probes every route with a non-loopback
// RemoteAddr + non-allowed Host. Any non-dashboard route that returns the
// dashboard's signature 403 has had the access middleware leak onto it.
func TestFullRouteTable_DashboardMiddlewareStaysScoped(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, enabledDashboardCfg(t))

	// Replace every non-dashboard handler with a marker so leaks surface as a
	// body mismatch rather than a nil-handler panic. The dashboard subtree is
	// left untouched so the access middleware stays in place for it.
	mark := map[string]string{}
	for route := range reg.GetRoutes() {
		if strings.HasPrefix(route, dashboard.DashboardRoute) {
			continue
		}
		mark[route] = "MARK:" + route
		r := route
		reg.GetRoutes()[r] = router.RouteInfo{
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("MARK:" + r))
			},
			Description: "marker",
			Method:      http.MethodGet,
			Order:       reg.GetRoutes()[r].Order,
		}
	}

	mux := http.NewServeMux()
	reg.WireUp(mux)

	// Every non-dashboard route must reach its own marker unchanged when
	// probed with a RemoteAddr/Host the dashboard middleware would reject.
	// A 403 here means the access gate leaked onto that route.
	for route, want := range mark {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.RemoteAddr = "203.0.113.9:54321"
		req.Host = "evil.example"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status=%d, want 200 (dashboard middleware leaked?)", route, rec.Code)
			continue
		}
		if rec.Body.String() != want {
			t.Errorf("%s: body=%q, want %q", route, rec.Body.String(), want)
		}
	}

	// The dashboard subtree itself must still be gated: a non-loopback probe
	// gets 403 with the self-diagnosing body, proving the policy is live on
	// /internal/ui/ even though it leaked nowhere else.
	dashReq := httptest.NewRequest(http.MethodGet, dashboard.DashboardRoute, nil)
	dashReq.RemoteAddr = "203.0.113.9:54321"
	dashReq.Host = "evil.example"
	dashRec := httptest.NewRecorder()
	mux.ServeHTTP(dashRec, dashReq)
	if dashRec.Code != http.StatusForbidden {
		t.Errorf("dashboard must be gated for non-loopback: got %d, want 403", dashRec.Code)
	}
}

// TestDashboardRoute_ReachableFromLoopback confirms a loopback request gets
// through the access gate. The embedded dist carries only the .gitkeep
// sentinel in this checkout (no `make build-web` ran), so the underlying
// handler serves the not-built 503 - we assert the request reached the
// dashboard handler (not 403) rather than the asset body.
func TestDashboardRoute_ReachableFromLoopback(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, enabledDashboardCfg(t))

	mux := http.NewServeMux()
	reg.WireUp(mux)

	req := httptest.NewRequest(http.MethodGet, dashboard.DashboardRoute, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("loopback request was gated: %d (body=%q)", rec.Code, rec.Body.String())
	}
	// 503 (not built) is the expected response when the dist carries only the
	// sentinel; what matters is that the access middleware let it through.
	if rec.Code != http.StatusServiceUnavailable {
		t.Logf("loopback dashboard response: status=%d (503 expected when dist is sentinel-only)", rec.Code)
	}
}

// TestDashboardRoute_DisabledYields404 confirms the FR-9 off switch: when
// dashboard.enabled is false the route is not registered, so a request to
// /internal/ui/ hits the default mux 404 - never a 403 from dashboard code,
// which would imply the route exists and is gated (and would leak the mount
// point to a scanner).
func TestDashboardRoute_DisabledYields404(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, config.DashboardConfig{Enabled: false})

	if _, exists := reg.GetRoutes()[dashboard.DashboardRoute]; exists {
		t.Fatal("disabled dashboard must not register /internal/ui/")
	}

	mux := http.NewServeMux()
	reg.WireUp(mux)

	req := httptest.NewRequest(http.MethodGet, dashboard.DashboardRoute, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("disabled dashboard must produce default-mux 404, got %d", rec.Code)
	}
}

// TestDashboardRoute_CollisionSkipsMount confirms the belt-and-braces
// collision guard: if /internal/ui/ is somehow already registered when the
// dashboard mount runs, registerRoutes logs Error and skips the mount rather
// than panicking or shadowing the existing registration. The pre-existing
// handler stays in place byte-for-byte; the dashboard never registers.
func TestDashboardRoute_CollisionSkipsMount(t *testing.T) {
	t.Parallel()

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	// Pre-register /internal/ui/ to simulate a future regression where another
	// registration collides with the dashboard subtree.
	const collisionBody = "PRE-EXISTING"
	reg.RegisterWithMethod(dashboard.DashboardRoute, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(collisionBody))
	}, "collision sentinel", http.MethodGet)

	app := &Application{
		Config:        &config.Config{Dashboard: enabledDashboardCfg(t)},
		logger:        &mockStyledLogger{},
		routeRegistry: reg,
	}
	// registerRoutes mounts the dashboard last; with /internal/ui/ already
	// present, the collision branch must skip the mount. No panic, no error
	// return (signature is unchanged), and the sentinel handler survives.
	app.registerRoutes()

	mux := http.NewServeMux()
	reg.WireUp(mux)

	// A loopback probe still gets the sentinel body, proving the dashboard
	// mount did not overwrite the pre-existing handler.
	req := httptest.NewRequest(http.MethodGet, dashboard.DashboardRoute, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("collision: status=%d, want 200 from sentinel handler", rec.Code)
	}
	if rec.Body.String() != collisionBody {
		t.Errorf("collision: body=%q, want sentinel %q (dashboard overwrote it)", rec.Body.String(), collisionBody)
	}
}

// TestDashboardRoute_ProxyAndInternalRoutesPresent is the regression guard
// confirming mounting the dashboard last does not disturb the existing route
// set: every static internal and proxy route is still present by name.
func TestDashboardRoute_ProxyAndInternalRoutesPresent(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, enabledDashboardCfg(t))

	want := []string{
		"/internal/status",
		"/internal/status/endpoints",
		"/internal/status/models",
		"/version",
		"/olla/proxy/",
		dashboard.DashboardRoute,
	}
	routes := reg.GetRoutes()
	got := make([]string, 0, len(routes))
	for r := range routes {
		got = append(got, r)
	}
	sort.Strings(got)

	for _, w := range want {
		if _, ok := routes[w]; !ok {
			t.Errorf("expected route %q in registry, got %v", w, got)
		}
	}
}
