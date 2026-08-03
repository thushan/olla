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
// ever ships without an index.html: handler construction degrades to the
// not-built handler rather than panicking at request time. Previously this was
// a per-request 404 from serveIndex; now the index is pre-cached at handler
// construction and missing-index is caught at dashboardHandler time, so the
// invariant under test moves from "serveIndex returns 404" to "construction
// falls back to notBuilt without panicking".
func TestServeIndexMissingGuardsNoPanic(t *testing.T) {
	defer func() {
		if x := recover(); x != nil {
			t.Fatalf("dashboardHandler panicked on missing index: %v", x)
		}
	}()

	// root has assets but no index.html - newSPAHandler would construct a
	// populated map but fail to find indexFile, which must surface as a
	// not-built fallback (503) rather than a panic.
	root := fstest.MapFS{
		"assets/payload.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := dashboardHandler(root)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("missing index: got %d, want 503 (not-built fallback)", w.Code)
	}
}

// TestSecurityHeadersOnAsset confirms an ordinary static asset response
// (e.g. the built JS bundle) carries the browser-hardening headers: MIME
// sniffing protection, a frame policy, and a CSP.
func TestSecurityHeadersOnAsset(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/assets/index-AbC123.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want %q", got, "DENY")
	}
	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP should set frame-ancestors 'none', got %q", csp)
	}
	if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP should restrict script-src to 'self', got %q", csp)
	}
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP should never allow unsafe-eval, got %q", csp)
	}
}

// TestSecurityHeadersOnIndex confirms the SPA index response (the entry
// document at /internal/ui/) carries the same hardening headers as static
// assets - it is the response most likely to be navigated to directly.
func TestSecurityHeadersOnIndex(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want %q", got, "DENY")
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Error("Content-Security-Policy header is missing on the SPA index response")
	}
}

// assertDashboardSecurityHeaders checks the four browser-hardening headers
// every dashboard response must carry, regardless of status code. Shared
// with access_test.go (same package) so the 403 path is held to the same
// standard as the 200/404/405/503 paths.
func assertDashboardSecurityHeaders(t *testing.T, h http.Header) {
	t.Helper()
	cases := []struct {
		header string
		want   string
		// substring true means want is checked as a contains, not equality
		// (used for CSP whose full value is long and version-sensitive).
		substring bool
	}{
		{"X-Content-Type-Options", "nosniff", false},
		{"X-Frame-Options", "DENY", false},
		{"Content-Security-Policy", "frame-ancestors 'none'", true},
		{"Referrer-Policy", "no-referrer", false},
	}
	for _, c := range cases {
		got := h.Get(c.header)
		switch {
		case got == "":
			t.Errorf("%s header missing (must be set before any response body)", c.header)
		case c.substring && !strings.Contains(got, c.want):
			t.Errorf("%s: got %q, want to contain %q", c.header, got, c.want)
		case !c.substring && got != c.want:
			t.Errorf("%s: got %q, want %q", c.header, got, c.want)
		}
	}
}

// C3: security headers must be present on EVERY response path in the
// dashboard handler tree, not just the 200 path. A 404 (missing asset), a
// 405 (wrong method), and the not-built 503 must all carry the same
// hardening headers as a successful response, plus Referrer-Policy. Without
// this, an error page can be framed, sniffed, or referrer-leaked exactly as
// a success page can. These paths previously set none of the headers
// (or only X-Content-Type-Options), so they fail against the shared
// assertion until setSecurityHeaders is called before every write.
func TestSecurityHeadersOn404(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/internal/ui/assets/missing-hash.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
	assertDashboardSecurityHeaders(t, resp.Header)
}

func TestSecurityHeadersOn405(t *testing.T) {
	ts := httptest.NewServer(mountedHandler(populatedFS))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/internal/ui/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", resp.StatusCode)
	}
	assertDashboardSecurityHeaders(t, resp.Header)
}

func TestSecurityHeadersOnNotBuilt503(t *testing.T) {
	sentinelOnly := fstest.MapFS{".gitkeep": &fstest.MapFile{Data: nil}}
	ts := httptest.NewServer(dashboardHandler(sentinelOnly))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want 503", resp.StatusCode)
	}
	assertDashboardSecurityHeaders(t, resp.Header)
}

// TestAssetsBuilt_SyntheticFS drives both branches of assetsBuiltIn via a
// synthetic FS, so the decision startup logging depends on is covered without
// coupling the test to whether the committed dist happens to ship a built
// index.html.
func TestAssetsBuilt_SyntheticFS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		root fs.FS
		want bool
	}{
		{
			name: "sentinel only - not built",
			root: fstest.MapFS{".gitkeep": &fstest.MapFile{Data: nil}},
			want: false,
		},
		{
			name: "empty FS - not built",
			root: fstest.MapFS{},
			want: false,
		},
		{
			name: "index present - built",
			root: fstest.MapFS{indexFile: &fstest.MapFile{Data: []byte("<html></html>")}},
			want: true,
		},
		{
			name: "index is a directory - not built",
			root: fstest.MapFS{indexFile + "/": &fstest.MapFile{}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := assetsBuiltIn(tc.root); got != tc.want {
				t.Errorf("assetsBuiltIn = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAssetsBuilt_MatchesRealEmbed confirms the package-level AssetsBuilt
// (read from the production embed) agrees with assetsBuiltIn over the same
// embed. They must never drift: startup logging and route selection use the
// two respectively, and a mismatch would mean the startup line claims ready
// while the route 503s (or vice versa).
func TestAssetsBuilt_MatchesRealEmbed(t *testing.T) {
	t.Parallel()

	sub, err := fs.Sub(embeddedFS, distRoot)
	if err != nil {
		t.Fatalf("fs.Sub failed on the real embed: %v", err)
	}
	want := assetsBuiltIn(sub)
	if got := AssetsBuilt(); got != want {
		t.Errorf("AssetsBuilt() = %v but assetsBuiltIn(realEmbed) = %v; the two call sites have drifted", got, want)
	}
}

// BenchmarkSPAHandler_AssetServing measures the per-request cost of serving a
// cached asset. The pre-construction cache means this should be a pure map
// lookup plus http.ServeContent over a bytes.Reader, with no per-request file
// I/O or SHA-256 computation. Documented so a regression that re-introduces
// io.ReadAll + sha256 on the hot path would surface as an allocation count
// spike here.
func BenchmarkSPAHandler_AssetServing(b *testing.B) {
	h := dashboardHandler(populatedFS)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-AbC123.js", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status: %d", w.Code)
		}
	}
}
