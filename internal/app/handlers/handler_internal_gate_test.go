package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

// gatedInternalCfg is the configuration from #214 that needs the gate most:
// the dashboard off, the internal API gate on. The UI subtree never mounts, so
// only the policy on the monitoring routes can be doing the work.
func gatedInternalCfg(t *testing.T) config.DashboardConfig {
	t.Helper()
	c := config.DashboardConfig{
		Enabled: false,
		AccessPolicy: config.AccessPolicyConfig{
			AllowedCIDRs: []string{"127.0.0.0/8", "::1/128"},
			AllowedHosts: []string{"localhost"},
		},
		GateInternalAPI: true,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("validate gate cfg: %v", err)
	}
	return c
}

// TestInternalAPI_GateCoversEveryMonitoringRoute sweeps the registered route
// table rather than a fixed list, so a /internal/* route added later without
// the gate fails here instead of shipping open. A rejected client must get the
// dashboard's self-diagnosing 403 on each one before any handler runs: the
// Application behind this table has no services, so a request that reached a
// handler would fail on nil state instead of returning 403.
func TestInternalAPI_GateCoversEveryMonitoringRoute(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, gatedInternalCfg(t))

	mux := http.NewServeMux()
	reg.WireUp(mux)

	// The sweep is only as good as the routes it visits, so pin the ones this
	// change is about as a floor. Losing one to a refactor should fail loudly
	// rather than silently shrinking the swept set.
	known := []string{
		constants.DefaultHealthCheckEndpoint,
		"/internal/status",
		constants.DefaultMetricsEndpoint,
		"/internal/status/endpoints",
		"/internal/status/models",
		"/internal/stats/models",
		"/internal/stats/translators",
		"/internal/stats/sticky",
		"/internal/process",
		"/version",
	}
	registered := make(map[string]struct{}, len(known))
	for route := range reg.GetRoutes() {
		registered[route] = struct{}{}
	}
	for _, route := range known {
		if _, ok := registered[route]; !ok {
			t.Fatalf("%s is not registered; the sweep would pass by visiting nothing", route)
		}
	}

	swept := 0
	for route := range registered {
		if !strings.HasPrefix(route, "/internal/") && route != "/version" {
			continue
		}
		swept++

		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.RemoteAddr = "203.0.113.9:54321"
		req.Host = "evil.example"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status=%d, want 403 from the internal gate", route, rec.Code)
			continue
		}
		body := rec.Body.String()
		if !strings.Contains(body, "403 forbidden:") || !strings.Contains(body, "203.0.113.9") {
			t.Errorf("%s: body=%q, want the dashboard's self-diagnosing 403 naming the client", route, body)
		}
	}

	if swept < len(known) {
		t.Fatalf("swept %d gated routes, want at least the %d known ones", swept, len(known))
	}
}
