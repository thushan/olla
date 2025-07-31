package handlers

import (
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/app/handlers/dashboard"
)

// registerDashboardRoutes sets up the dashboard UI routes
func (a *Application) registerDashboardRoutes() {
	// Get the dashboard handler
	dashboardHandler, err := dashboard.Handler()
	if err != nil {
		a.logger.Error("Failed to create dashboard handler", "error", err)
		return
	}

	// Redirect root to dashboard
	a.routeRegistry.RegisterWithMethod("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	}, "Root redirect to dashboard", "GET")

	// Dashboard route - strip prefix for the embedded handler
	a.routeRegistry.RegisterWithMethod("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		// Strip the /dashboard prefix
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/dashboard")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		dashboardHandler.ServeHTTP(w, r)
	}, "Olla Dashboard UI", "GET")

	// Also handle sub-paths under /dashboard/
	a.routeRegistry.Register("/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		// Strip the /dashboard prefix
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/dashboard")
		dashboardHandler.ServeHTTP(w, r)
	}, "Dashboard assets and routes")
}