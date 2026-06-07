package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

// nextCalled is a simple flag handler used across CORS tests to detect whether
// the next handler in the chain was reached.
func nextCalledHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func baseCORSConfig() config.CorsConfig {
	return config.CorsConfig{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}
}

// TestCORSPreflight verifies that a preflight OPTIONS request is answered by
// rs/cors directly (204, CORS headers set) without reaching the next handler.
func TestCORSPreflight(t *testing.T) {
	t.Parallel()

	var called bool
	c := NewCORS(baseCORSConfig())
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/olla/proxy/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("next handler must not be called for preflight requests")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin to be set on preflight response")
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods to be set on preflight response")
	}
}

// TestCORSActualRequest verifies that a real GET from an allowed origin passes
// through to the next handler and that the response carries both the origin and
// the default exposed X-Olla-* headers.
func TestCORSActualRequest(t *testing.T) {
	t.Parallel()

	var called bool
	c := NewCORS(baseCORSConfig())
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/olla/proxy/v1/models", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler must be called for actual (non-preflight) requests")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin on actual request response")
	}

	exposed := rr.Header().Get("Access-Control-Expose-Headers")
	for _, h := range []string{constants.HeaderXOllaModel, constants.HeaderXOllaEndpoint} {
		if !strings.Contains(exposed, h) {
			t.Errorf("expected Access-Control-Expose-Headers to contain %q, got %q", h, exposed)
		}
	}
}

// TestCORSNoOriginPassthrough verifies that requests without an Origin header
// are passed through unchanged with no CORS headers added.
func TestCORSNoOriginPassthrough(t *testing.T) {
	t.Parallel()

	var called bool
	c := NewCORS(baseCORSConfig())
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	// Deliberately no Origin header — simulates curl, SDK, or server-to-server calls.

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler must be called when no Origin header is present")
	}
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for non-CORS request, got %q", v)
	}
	if v := rr.Header().Get("Access-Control-Expose-Headers"); v != "" {
		t.Errorf("expected no Access-Control-Expose-Headers for non-CORS request, got %q", v)
	}
}

// TestCORSExplicitExposedHeadersOverride confirms that setting ExposedHeaders in
// the config replaces the default X-Olla-* set entirely — no merging happens.
func TestCORSExplicitExposedHeadersOverride(t *testing.T) {
	t.Parallel()

	cfg := baseCORSConfig()
	cfg.ExposedHeaders = []string{"X-Custom-Header"}

	var called bool
	c := NewCORS(cfg)
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/olla/proxy/v1/models", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	exposed := rr.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "X-Custom-Header") {
		t.Errorf("expected custom header in Expose-Headers, got %q", exposed)
	}
	if strings.Contains(exposed, constants.HeaderXOllaModel) {
		t.Errorf("default %q must not appear when ExposedHeaders is explicitly set, got %q",
			constants.HeaderXOllaModel, exposed)
	}
}

// TestCORSAllowCredentials verifies that AllowCredentials=true propagates correctly
// and that the reflected origin is returned (not a wildcard, which the CORS spec forbids).
func TestCORSAllowCredentials(t *testing.T) {
	t.Parallel()

	cfg := baseCORSConfig()
	cfg.AllowCredentials = true

	var called bool
	c := NewCORS(cfg)
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/olla/proxy/v1/models", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if v := rr.Header().Get("Access-Control-Allow-Credentials"); v != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %q", v)
	}
	// When credentials are in play the CORS spec forbids wildcard; the origin must be echoed.
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "https://example.com" {
		t.Errorf("expected Allow-Origin to reflect request origin with credentials, got %q", v)
	}
}

// TestCORSDisallowedOrigin verifies that a request from an unlisted origin does
// not receive an Access-Control-Allow-Origin header.
func TestCORSDisallowedOrigin(t *testing.T) {
	t.Parallel()

	var called bool
	c := NewCORS(baseCORSConfig())
	handler := c.Handler(nextCalledHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/olla/proxy/v1/models", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if v := rr.Header().Get("Access-Control-Allow-Origin"); v == "https://evil.example.com" {
		t.Errorf("disallowed origin must not appear in Access-Control-Allow-Origin, got %q", v)
	}
}

// TestDefaultCORSExposedHeaders is a regression guard that asserts every expected
// X-Olla-* header constant is present in DefaultCORSExposedHeaders. Without this,
// a new header added to content.go would silently go unexposed to browser clients.
func TestDefaultCORSExposedHeaders(t *testing.T) {
	t.Parallel()

	expected := []string{
		constants.HeaderXOllaRequestID,
		constants.HeaderXOllaEndpoint,
		constants.HeaderXOllaBackendType,
		constants.HeaderXOllaModel,
		constants.HeaderXOllaResponseTime,
		constants.HeaderXOllaRoutingStrategy,
		constants.HeaderXOllaRoutingDecision,
		constants.HeaderXOllaRoutingReason,
		constants.HeaderXOllaMode,
		constants.HeaderXOllaStickySession,
		constants.HeaderXOllaStickyKeySource,
		constants.HeaderXOllaSessionID,
	}

	defaultSet := make(map[string]struct{}, len(DefaultCORSExposedHeaders))
	for _, h := range DefaultCORSExposedHeaders {
		defaultSet[h] = struct{}{}
	}

	for _, h := range expected {
		if _, ok := defaultSet[h]; !ok {
			t.Errorf("DefaultCORSExposedHeaders is missing %q", h)
		}
	}

	if len(DefaultCORSExposedHeaders) != len(expected) {
		t.Errorf("DefaultCORSExposedHeaders has %d entries, expected %d — update this test when adding new X-Olla-* headers",
			len(DefaultCORSExposedHeaders), len(expected))
	}
}
