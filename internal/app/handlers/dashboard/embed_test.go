package dashboard

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// populatedFS is a synthetic embedded dist the handler tests drive, so they do
// not depend on whether the committed dist happens to be built. The shape
// mirrors a real Vite build: an index.html entry plus content-hashed assets.
var populatedFS = fstest.MapFS{
	indexFile:                    &fstest.MapFile{Data: []byte(`<!doctype html><html lang="en"><head><title>spa</title></head><body>spa shell</body></html>`)},
	"assets/index-AbC123.js":     &fstest.MapFile{Data: []byte("console.log(1)")},
	"assets/index-9f3b2c1d.css":  &fstest.MapFile{Data: []byte("body{}")},
	"assets/ibm-plex-a1b2.woff2": &fstest.MapFile{Data: []byte("wOF2\x00\x01\x00\x00fontbytes")},
	"favicon.ico":                &fstest.MapFile{Data: []byte{0x00, 0x00, 0x01, 0x00}},
}

// mountedHandler wires the handler under test behind the same /internal/ui/
// prefix production mounts it under, so tests exercise the real request path.
func mountedHandler(root fs.FS) http.Handler {
	return http.StripPrefix(DashboardRoute, dashboardHandler(root))
}

// TestHandlerServesIndexAtRoot drives the mounted handler over HTTP rather
// than poking the FS directly, so the test exercises the full path the
// request takes through ServeHTTP including headers.
func TestHandlerServesIndexAtRoot(t *testing.T) {
	ts := httptest.NewServer(dashboardHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html prefix", ct)
	}
	if len(body) == 0 {
		t.Error("body is empty - served nothing for /")
	}
}

// TestHandlerMountedAtDashboardRoute proves the handler works when accessed
// through the same /internal/ui/ prefix RouteRegistry mounts it under in
// production. This is the path users actually hit.
func TestHandlerMountedAtDashboardRoute(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/")
	if err != nil {
		t.Fatalf("GET /internal/ui/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}
}

// TestSPAFallback proves unknown sub-paths serve index.html, not 404. Without
// this, client-side routes like /internal/ui/endpoints would break in
// production the moment an operator refreshes.
func TestSPAFallback(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	for _, p := range []string{"/internal/ui/endpoints", "/internal/ui/models/overview", "/internal/ui/deep/nested/route"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status got %d, want 200", p, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: Content-Type got %q, want text/html", p, ct)
		}
		if !strings.Contains(strings.ToLower(string(body)), "<html") {
			t.Errorf("%s: body is not the SPA shell: %q", p, string(body[:min(60, len(body))]))
		}
	}
}

// TestMissingStaticAssetIs404 is the regression guard for finding 10: a
// browser holding a stale index.html across a binary upgrade can request a
// hashed JS chunk, font or icon that no longer exists. Falling back to the
// SPA shell for that request produced "200 text/html" for what the browser
// expected to be JS/CSS/a font, surfacing as a cryptic "Unexpected token '<'"
// instead of a clean 404. Only extensionless navigation paths should still
// fall back to index.html.
func TestMissingStaticAssetIs404(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	cases := []string{
		"/internal/ui/assets/missing-hash.js",
		"/internal/ui/assets/does-not-exist.css",
		"/internal/ui/missing.woff2",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			resp, err := http.Get(ts.URL + p)
			if err != nil {
				t.Fatalf("GET %s: %v", p, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s: status got %d, want 404", p, resp.StatusCode)
			}
		})
	}
}

// TestExtensionlessUnknownPathStillFallsBackToIndex proves the fix didn't
// overreach: a client-side route with no file extension must still resolve
// to the SPA shell with 200, exactly as TestSPAFallback already covers, but
// pinned here specifically alongside the new 404 behaviour so the two are
// tested as a pair.
func TestExtensionlessUnknownPathStillFallsBackToIndex(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/some/deep/client/route")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(string(body)), "<html") {
		t.Errorf("body is not the SPA shell: %q", string(body[:min(80, len(body))]))
	}
}

// TestExistingAssetsStillServeWithCacheHeaders confirms the fix didn't
// disturb the happy path: a real asset under assets/ still serves its bytes
// with 200 and the expected long-lived cache header.
func TestExistingAssetsStillServeWithCacheHeaders(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/assets/index-AbC123.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if string(body) != "console.log(1)" {
		t.Errorf("body mismatch: got %q", string(body))
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control: got %q, want immutable", got)
	}
}

// TestPathTraversalRejected is the explicit traversal proof the spec demands.
// We try the obvious forms an attacker would attempt and assert each is
// refused - either as 404 or as the SPA fallback (which only ever serves
// index.html, never a file outside webassets). The invariant under test is
// not the status code per se, but that no byte of /etc/passwd or any file
// outside the embed root can be returned.
func TestPathTraversalRejected(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	cases := []string{
		"/internal/ui/../etc/passwd",
		"/internal/ui/../../etc/passwd",
		"/internal/ui/..%2F..%2Fetc%2Fpasswd",
		"/internal/ui/%2e%2e/%2e%2e/etc/passwd",
		"/internal/ui/.%2e/.%2e/etc/passwd",
		"/internal/ui/./../etc/passwd",
		"/internal/ui/ /../etc/passwd",
		"/internal/ui/etc/passwd",
		"/internal/ui/%00",
	}

	for _, p := range cases {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("%s: request error: %v", p, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Forbidden outputs: 200 with SPA shell (fallback), 404, or 400. The
		// only invariant that matters: no /etc/passwd content leaks.
		if strings.Contains(string(body), "root:") {
			t.Errorf("%s: LEAK - response contains /etc/passwd content", p)
		}
		if resp.StatusCode == http.StatusOK {
			if !strings.Contains(strings.ToLower(string(body)), "<html") {
				t.Errorf("%s: 200 response is not the SPA shell: %q", p, string(body[:min(80, len(body))]))
			}
		}
	}
}

// TestMIMETypes is the table-driven MIME check for the eight extensions the
// spec calls out, driven through the resolver function production uses.
func TestMIMETypes(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{".js", "text/javascript"},
		{".mjs", "text/javascript"},
		{".css", "text/css"},
		{".html", "text/html"},
		{".svg", "image/svg+xml"},
		{".woff", "font/woff"},
		{".woff2", "font/woff2"},
		{".png", "image/png"},
		{".ico", "image/x-icon"},
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			got := contentTypeFor("asset" + c.ext)
			if !strings.HasPrefix(got, c.want) {
				t.Errorf("contentTypeFor(%q): got %q, want prefix %q", c.ext, got, c.want)
			}
		})
	}
}

// TestCacheControlHeaders covers both branches of the caching strategy:
// hashed Vite assets get long-lived immutable caching, index.html gets
// no-cache so deploys are picked up on next navigation.
func TestCacheControlHeaders(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"index.html", "index.html", "no-cache"},
		{"hashed js", "assets/index-AbC123.js", "public, max-age=31536000, immutable"},
		{"hashed css", "assets/index-9f3b2c1d.css", "public, max-age=31536000, immutable"},
		{"hashed woff2", "assets/inter-a1b2c3d4.woff2", "public, max-age=31536000, immutable"},
		{"unhashed asset", "favicon.ico", "public, max-age=3600"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cacheControlFor(c.path)
			if got != c.want {
				t.Errorf("cacheControlFor(%q): got %q, want %q", c.path, got, c.want)
			}
		})
	}
}

// TestResponseHeadersPresent asserts every response carries both a non-empty
// ETag and a non-zero Last-Modified. The prior prototype's defect was
// zero-value headers, which left every response uncached.
func TestResponseHeadersPresent(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if et := resp.Header.Get("ETag"); et == "" {
		t.Error("ETag header is missing - responses will not be cached conditionally")
	} else if !strings.HasPrefix(et, `"`) {
		t.Errorf("ETag %q is not a strong (quoted) etag", et)
	}
	if lm := resp.Header.Get("Last-Modified"); lm == "" {
		t.Error("Last-Modified header is missing")
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Error("Cache-Control header is missing")
	} else if cc != "no-cache" {
		t.Errorf("index.html Cache-Control: got %q, want no-cache", cc)
	}
}

// TestConditionalGET proves If-None-Match against our strong ETag returns
// 304 with no body. The dashboard polls every few seconds; without working
// conditional GET the steady-state cost scales with the number of open tabs.
func TestConditionalGET(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	first, err := http.Get(ts.URL + "/internal/ui/")
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	etag := first.Header.Get("ETag")
	first.Body.Close()

	if etag == "" {
		t.Fatal("no ETag on first response")
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/internal/ui/", nil)
	req.Header.Set("If-None-Match", etag)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("conditional GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional GET: got %d, want 304", resp.StatusCode)
	}
	if body, _ := io.ReadAll(resp.Body); len(body) != 0 {
		t.Errorf("conditional GET returned %d bytes, want empty body", len(body))
	}
}

// TestMethodNotAllowed confirms GET/HEAD only - the dashboard is read-only.
func TestMethodNotAllowed(t *testing.T) {
	ts := httptest.NewServer(dashboardHandler(populatedFS))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status: got %d, want 405", resp.StatusCode)
	}
}

// TestHandlerRejectsWhenNotBuilt is the build-contract guard: when the
// embedded dist has no index.html (a fresh checkout carrying only the .gitkeep
// sentinel, or a binary built without `make build-web`), the handler must
// serve a clear 503 rather than a silent placeholder. This is what makes a
// stale or unbuilt dashboard loud instead of mysterious.
func TestHandlerRejectsWhenNotBuilt(t *testing.T) {
	// Only the sentinel is present - no index.html, no assets.
	sentinelOnly := fstest.MapFS{".gitkeep": &fstest.MapFile{Data: nil}}
	ts := httptest.NewServer(dashboardHandler(sentinelOnly))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want 503", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(string(body)), "not built") {
		t.Errorf("body should explain the dashboard is not built: %q", string(body))
	}
}

// TestServeIndexMissingGuardsNoPanic documents the failure mode if the embed
// ever ships without an index.html: we return 404, not a panic. The guard is
// here so a future packaging mistake degrades rather than crashes.
func TestServeIndexMissingGuardsNoPanic(t *testing.T) {
	h := &spaHandler{root: fstestFS{}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	defer func() {
		if x := recover(); x != nil {
			t.Fatalf("serveIndex panicked on missing index: %v", x)
		}
	}()
	h.serveIndex(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("missing index: got %d, want 404", w.Code)
	}
}

// fstestFS is a minimal fs.FS that always errors, used only to prove the
// missing-index path returns 404 rather than panicking.
type fstestFS struct{}

func (fstestFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
