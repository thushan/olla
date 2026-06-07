package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

// stubHandler returns an HTTP handler that records whether it was invoked and
// always responds 200 OK. Used across CORS wiring tests as the wrapped target.
func stubHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func enabledCORSConfig() config.CorsConfig {
	return config.CorsConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}
}

// TestApplyCORSPreflight verifies that an OPTIONS preflight from an allowed origin
// is answered with CORS headers and does not reach the wrapped handler.
func TestApplyCORSPreflight(t *testing.T) {
	t.Parallel()

	var called bool
	handler := applyCORS(stubHandler(&called), enabledCORSConfig())

	req := httptest.NewRequest(http.MethodOptions, "/olla/proxy/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("wrapped handler must not be reached for preflight OPTIONS")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", rr.Code)
	}
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v == "" {
		t.Error("expected Access-Control-Allow-Origin on preflight response")
	}
	if v := rr.Header().Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("expected Access-Control-Allow-Methods on preflight response")
	}
}

// TestApplyCORSActualRequest verifies that a real GET from an allowed origin
// reaches the wrapped handler and carries origin + exposed X-Olla-* headers.
func TestApplyCORSActualRequest(t *testing.T) {
	t.Parallel()

	var called bool
	handler := applyCORS(stubHandler(&called), enabledCORSConfig())

	req := httptest.NewRequest(http.MethodGet, "/olla/proxy/v1/models", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("wrapped handler must be called for actual (non-preflight) requests")
	}
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v == "" {
		t.Error("expected Access-Control-Allow-Origin on actual request response")
	}

	// Default exposed headers must include X-Olla-* routing and model metadata so
	// browser dashboards can read them cross-origin.
	exposed := rr.Header().Get("Access-Control-Expose-Headers")
	for _, h := range []string{constants.HeaderXOllaModel, constants.HeaderXOllaEndpoint} {
		if !strings.Contains(exposed, h) {
			t.Errorf("Access-Control-Expose-Headers must contain %q, got %q", h, exposed)
		}
	}
}

// TestApplyCORSDisabled verifies that when CORS is disabled applyCORS returns the
// original handler identity — no CORS headers are added regardless of Origin.
func TestApplyCORSDisabled(t *testing.T) {
	t.Parallel()

	var called bool
	inner := stubHandler(&called)
	cfg := config.CorsConfig{Enabled: false}

	wrapped := applyCORS(inner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if !called {
		t.Error("original handler must still serve requests when CORS is disabled")
	}
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("no CORS headers expected when disabled, got Access-Control-Allow-Origin: %q", v)
	}
}

// TestApplyCORSNoOriginPassthrough verifies that requests without an Origin header
// (curl, server-to-server, coding agents) pass through cleanly with no CORS headers.
// This is the critical non-regression case: CORS must be invisible to non-browser clients.
func TestApplyCORSNoOriginPassthrough(t *testing.T) {
	t.Parallel()

	var called bool
	handler := applyCORS(stubHandler(&called), enabledCORSConfig())

	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	// No Origin header — simulates curl, SDK calls, or server-to-server traffic.

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler must be called for requests without an Origin header")
	}
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("no CORS headers expected without Origin header, got %q", v)
	}
	if v := rr.Header().Get("Access-Control-Expose-Headers"); v != "" {
		t.Errorf("no Expose-Headers expected without Origin header, got %q", v)
	}
}
