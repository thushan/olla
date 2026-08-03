package dashboard

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

// serveEpoch is a stable, non-zero timestamp used as the Last-Modified value
// for embedded assets. The embed FS itself carries no mtime (files report the
// zero time), so http.ServeContent would otherwise emit no Last-Modified and
// skip If-Modified-Since handling entirely, defeating conditional GET for
// clients that prefer that header. Using process start time is a deliberate
// proxy: imperfect as a content timestamp, but it shifts only across restarts
// and the strong ETag remains the authoritative freshness signal, so a changed
// serveEpoch never causes a wrong 200 (only a missed 304 on the first request
// after restart).
var serveEpoch = time.Now()

// distRoot holds the embedded dashboard SPA bundle. It is regenerated from
// web/dashboard/ by `make build-web` and is gitignored: only a .gitkeep
// sentinel stays committed so the embed is non-empty and a fresh checkout
// compiles with `go build ./...` without Node installed. Release targets run
// build-web before compiling, so shipped binaries carry the real SPA; a binary
// built without that step serves a loud not-built response (see Handler) rather
// than a silent placeholder.
//
//go:embed all:dist
var embeddedFS embed.FS

// notBuiltWarn ensures the "dashboard not built" warning fires once per
// process. Handler() is invoked during route registration at startup, so the
// sync.Once collapses what is already a single call rather than request-time
// repeats.
var notBuiltWarn sync.Once

const (
	distRoot  = "dist"
	indexFile = "index.html"
)

// mimeOverrides patches extensions Go's registry gets wrong or omits. Without
// these, .js falls back to text/plain on some hosts and .woff2 is unknown,
// forcing content-sniffing on every response.
var mimeOverrides = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".json":  "application/json; charset=utf-8",
	".webp":  "image/webp",
}

// hashedAsset matches Vite's content-hashed output names, e.g.
// assets/index-AbC123.js or index-9f3b2c1d.css. Detecting the hash lets us
// advertise long-lived immutable caching, since the URL itself changes whenever
// the content does. Vite uses an 8-char hash by default; be permissive on
// length to survive future config changes.
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{6,}\.(js|mjs|css|woff2?|png|svg|ico|webp|json)$`)

// Handler serves the embedded dashboard. It is the value passed to
// RegisterRoutes so the access middleware applies uniformly to /dashboard/
// and every sub-path. The handler is safe for concurrent use; the embedded FS
// is read-only and the per-file ETag is derived deterministically from bytes.
func Handler() http.Handler {
	sub, err := fs.Sub(embeddedFS, distRoot)
	if err != nil {
		// Should never happen: distRoot ships populated via go:embed.
		// Guard anyway so a future rename cannot panic at request time.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "dashboard assets unavailable", http.StatusInternalServerError)
		})
	}
	return dashboardHandler(sub)
}

// AssetsBuilt reports whether the embedded dist carries a built index.html.
// It is the same check dashboardHandler uses to pick between the SPA handler
// and the not-built 503 handler, surfaced separately so startup logging can
// distinguish a binary that carries real assets from one carrying only the
// .gitkeep sentinel (e.g. a binary produced by `go install` without
// `make build-web`). A false return is not an error: the route still mounts
// and answers 503 with a clear message, the binary just should not claim the
// dashboard is "ready".
func AssetsBuilt() bool {
	sub, err := fs.Sub(embeddedFS, distRoot)
	if err != nil {
		return false
	}
	return assetsBuiltIn(sub)
}

// assetsBuiltIn is the FS-level check both Handler (for route selection) and
// AssetsBuilt (for startup logging) consult. Split out so the two call sites
// cannot drift on what "built" means, and so tests can drive the sentinel-only
// and populated branches via a synthetic fstest.MapFS without disturbing the
// package-level embed.
func assetsBuiltIn(root fs.FS) bool {
	info, err := fs.Stat(root, indexFile)
	return err == nil && !info.IsDir()
}

// dashboardHandler returns the SPA handler when the embedded dist carries a
// built index.html. If only the .gitkeep sentinel is present (the binary was
// compiled without `make build-web`), it returns a not-built handler that logs
// a startup warning once and serves 503, so a stale or unbuilt dashboard is
// obvious to the operator instead of silently serving a placeholder shell.
func dashboardHandler(root fs.FS) http.Handler {
	if !assetsBuiltIn(root) {
		return notBuiltHandler()
	}
	h, err := newSPAHandler(root)
	if err != nil {
		// Unreachable for a real embed (assetsBuiltIn passed above), but a
		// synthetic fstest could surface a mid-walk read error. Degrade to the
		// not-built handler rather than panicking at request time.
		return notBuiltHandler()
	}
	return h
}

// notBuiltHandler serves a clear 503 and warns once. The dist is generated at
// build time, so hitting this means the binary was compiled before the SPA was
// built (run a release target, or `make build-web` then rebuild).
func notBuiltHandler() http.Handler {
	notBuiltWarn.Do(func() {
		slog.Warn("dashboard assets not built; serving a not-built response",
			"fix", "run `make build-web` before building, or use a release target")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		setSecurityHeaders(w)
		http.Error(w, "dashboard not built; run `make build-web` before building the binary", http.StatusServiceUnavailable)
	})
}

// cachedAsset is the precomputed response shaping for one embedded asset. The
// embed.FS is immutable for the life of the process (its own go:embed
// contract), so reading each file ONCE at handler construction and serving
// from this map thereafter avoids io.ReadAll + sha256 on every request -
// observable as a per-request allocation and hash on a hot polling path.
type cachedAsset struct {
	data         []byte
	etag         string // strong, quoted: embed.FS bytes never change
	contentType  string
	cacheControl string
}

type spaHandler struct {
	root   fs.FS
	assets map[string]cachedAsset // keyed by fs.FS path (rel, slash-separated)
	index  cachedAsset            // index.html entry, pre-resolved for serveIndex
}

// newSPAHandler walks the embedded dist once, reading each file and computing
// its SHA-256 ETag. Walking at construction (rather than lazily on first
// request) means the first dashboard hit is no slower than any subsequent hit
// and the request-time hot path is a pure map lookup.
func newSPAHandler(root fs.FS) (*spaHandler, error) {
	h := &spaHandler{
		root:   root,
		assets: make(map[string]cachedAsset),
	}
	walkErr := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if p == "." {
			return nil
		}
		data, rerr := fs.ReadFile(root, p)
		if rerr != nil {
			return rerr
		}
		h.assets[p] = cachedAsset{
			data:         data,
			etag:         `"` + etagFor(data) + `"`,
			contentType:  contentTypeFor(p),
			cacheControl: cacheControlFor(p),
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	idx, ok := h.assets[indexFile]
	if !ok {
		// assetsBuiltIn already confirmed index.html exists, so reaching here
		// means the walk somehow skipped it. Treat the dashboard as not built
		// rather than serving a handler whose serveIndex would 404.
		return nil, fs.ErrNotExist
	}
	h.index = idx
	return h, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		setSecurityHeaders(w)
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, DashboardRoute)
	if rel == "" {
		rel = indexFile
	}
	// path.Clean collapses ".." segments lexically, but we still reject any
	// cleaned path that escapes the root or refuses to clean. ValidPath is the
	// stdlib authority on what can safely be passed to fs.FS.Open.
	rel = path.Clean(rel)
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		h.serveIndex(w, r)
		return
	}
	if !fs.ValidPath(rel) {
		setSecurityHeaders(w)
		http.NotFound(w, r)
		return
	}

	asset, ok := h.assets[rel]
	if !ok {
		// Unknown path: only fall back to the SPA shell for what looks like a
		// client-side route (extensionless, not under assets/). A path that
		// looks like a static asset - assets/missing-hash.js, missing.woff2 -
		// is a real 404: a browser holding a stale index.html across a binary
		// upgrade would otherwise get "200 text/html" for a JS chunk that no
		// longer exists and fail with a cryptic "Unexpected token '<'" instead
		// of a clean, diagnosable 404. Genuine SPA routes (/dashboard/endpoints
		// etc) have no extension and must still resolve to index.html.
		if looksLikeStaticAsset(rel) {
			setSecurityHeaders(w)
			http.NotFound(w, r)
			return
		}
		h.serveIndex(w, r)
		return
	}

	w.Header().Set("ETag", asset.etag)
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Cache-Control", asset.cacheControl)
	setSecurityHeaders(w)

	// modtime is serveEpoch rather than the zero time: ServeContent uses it to
	// set Last-Modified and to drive If-Modified-Since precondition checks, so
	// a non-zero value is what makes conditional GET work for clients that
	// prefer that header. The strong ETag above carries If-None-Match.
	http.ServeContent(w, r, path.Base(rel), serveEpoch, bytes.NewReader(asset.data))
}

// serveIndex emits the SPA entry document. It is the fallback for any unknown
// path and for directory requests, so client-side routing keeps working
// without hitting a 404. Cache-Control is no-cache so a new deploy is picked
// up on the next navigation, never a stale service-worker-cached shell. The
// index asset is pre-cached at handler construction, so this path is also a
// pure map-resolved serve with no per-request file I/O.
func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", mimeOverrides[".html"])
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", h.index.etag)
	setSecurityHeaders(w)
	http.ServeContent(w, r, indexFile, serveEpoch, bytes.NewReader(h.index.data))
}

// dashboardCSP is the Content-Security-Policy applied to every successful
// dashboard response (both the SPA shell and its static assets, since either
// could be navigated to directly). It is as strict as the actual built
// bundle allows, not the strictest CSP possible in the abstract:
//
//   - script-src 'self' with no 'unsafe-inline'/'unsafe-eval': the built
//     index.html loads only an external module script
//     (assets/index-*.js), never an inline <script> block, so scripts don't
//     need it.
//   - style-src 'self' 'unsafe-inline': several compiled components
//     (PctBar, RangeBar, SortableTable, the loading skeletons) render
//     data-driven bar widths via a Svelte-compiled style attribute
//     (element.style.cssText = "width:NN%..."), which CSP's style-src
//     gates the same way it gates a literal style="" attribute. Without
//     'unsafe-inline' here every latency/success-rate bar silently renders
//     at 0 width. Tightening this would need the frontend build to move
//     those to CSS custom properties (style:--fill-pct + a stylesheet rule)
//     instead of a raw style string - a frontend build change, logged to
//     the findings file rather than made here.
//   - img-src/font-src 'self': all icons and fonts are same-origin embedded
//     assets, nothing external.
//   - connect-src 'self': the poll stores fetch /internal/status* on the
//     same origin only.
//   - frame-ancestors 'none' plus the redundant X-Frame-Options: DENY below
//     covers browsers that only understand the older header.
//   - base-uri/form-action 'none': the SPA has no forms and no reason to
//     ever change its document base.
const dashboardCSP = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'; " +
	"form-action 'none'"

// setSecurityHeaders applies the browser hardening headers common to every
// dashboard response: MIME-sniffing protection, a frame policy
// (belt-and-braces via both the modern CSP directive and the legacy
// header), the CSP above, and Referrer-Policy so an operator navigating to
// the dashboard from another internal tool does not leak its URL as a
// Referer on subsequent requests. It must be called before any response
// body is written, on every response path (200, 404, 405, 403, 503), so
// error pages are not frameable or sniffable either.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", dashboardCSP)
	h.Set("Referrer-Policy", "no-referrer")
}

// looksLikeStaticAsset reports whether rel is shaped like a built asset
// reference rather than a client-side navigation path: anything under
// assets/, or whose final path segment carries a file extension. SPA routes
// the frontend router owns (e.g. "endpoints", "models/llama3") are
// extensionless by convention, so this deliberately does not try to
// enumerate every real extension - presence of a dot in the last segment is
// enough to distinguish "asset that failed to load" from "app route".
func looksLikeStaticAsset(rel string) bool {
	if rel == "assets" || strings.HasPrefix(rel, "assets/") {
		return true
	}
	return path.Ext(path.Base(rel)) != ""
}

// contentTypeFor resolves the Content-Type for an embedded asset. The override
// map wins first, then Go's extension registry, then a final fall-through to
// application/octet-stream so the client never receives a wild guess from
// content-sniffing on a font or script.
func contentTypeFor(name string) string {
	if ct, ok := mimeOverrides[strings.ToLower(path.Ext(name))]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// cacheControlFor distinguishes hashed Vite assets (which can be cached
// indefinitely because their filename changes on content change) from the
// SPA entry document (which must always be revalidated so new deploys land).
// Unhashed files fall back to a conservative hour, matching the spec floor.
func cacheControlFor(name string) string {
	if strings.EqualFold(path.Base(name), indexFile) {
		return "no-cache"
	}
	if hashedAsset.MatchString(path.Base(name)) {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=3600"
}

// etagFor returns a hex SHA-256 prefix of the file bytes. Strong (not weak)
// because embed.FS is immutable for the life of the process, so the digest
// is a true content identity rather than a heuristic.
func etagFor(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}
