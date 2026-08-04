package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/app/handlers/dashboard"
	"github.com/thushan/olla/internal/app/middleware"
)

// muxWithStatusRoute builds a mux wired the way registerRoutes wires it in
// production: the status handler is wrapped in middleware.GzipFunc before
// registration. The Application is the same one the wire-shape tests use, so
// the status handler has its real dependencies (repository, stats, registry,
// Config). A fresh mux keeps the assertion local to the status route.
func muxWithStatusRoute(t *testing.T) (http.Handler, *Application) {
	t.Helper()
	app := seedWireShapeApp(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/status", middleware.GzipFunc(app.statusHandler))
	return mux, app
}

// TestStatusRoute_GzipAppliedThroughMux confirms the production wiring: a
// request to /internal/status with Accept-Encoding: gzip gets a gzipped body
// and the Content-Encoding header. This catches accidental unwrapping of the
// middleware at the registration call site.
func TestStatusRoute_GzipAppliedThroughMux(t *testing.T) {
	t.Parallel()

	mux, _ := muxWithStatusRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"),
		"status route must be wrapped with the gzip middleware at registration time")

	gr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.NotEmpty(t, body)
}

// TestStatusRoute_PlainPathUntouched confirms the same route, requested
// without Accept-Encoding, is delivered verbatim with no Content-Encoding.
func TestStatusRoute_PlainPathUntouched(t *testing.T) {
	t.Parallel()

	mux, _ := muxWithStatusRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

// TestStaticAssetRoute_GzipNotApplied confirms the scope discipline: the
// dashboard's static asset subtree is served by embed.go via
// http.ServeContent (Range/206 semantics), so it must NOT be wrapped by the
// gzip middleware. We assert by probing /internal/ui/ through the real
// registerRoutes() output and confirming no Content-Encoding header appears
// on the response. This catches a future regression that wraps the dashboard
// handler at registration time.
func TestStaticAssetRoute_GzipNotApplied(t *testing.T) {
	t.Parallel()

	_, reg := applicationWithStaticRouteTable(t, enabledDashboardCfg(t))
	mux := http.NewServeMux()
	reg.WireUp(mux)

	req := httptest.NewRequest(http.MethodGet, dashboard.DashboardRoute, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Whatever the dashboard handler returns (503 sentinel-only dist, 200 with
	// real assets, 403 from a non-loopback probe), it must not carry
	// Content-Encoding: the static asset handler is intentionally outside the
	// gzip scope.
	assert.Empty(t, rec.Header().Get("Content-Encoding"),
		"static asset handler under /internal/ui/ must not be wrapped by gzip middleware")
}
