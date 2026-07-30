package dashboard

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
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

// Handler serves the embedded dashboard. It is the value WP-3 passes to
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

// dashboardHandler returns the SPA handler when the embedded dist carries a
// built index.html. If only the .gitkeep sentinel is present (the binary was
// compiled without `make build-web`), it returns a not-built handler that logs
// a startup warning once and serves 503, so a stale or unbuilt dashboard is
// obvious to the operator instead of silently serving a placeholder shell.
func dashboardHandler(root fs.FS) http.Handler {
	if _, err := fs.Stat(root, indexFile); err != nil {
		return notBuiltHandler()
	}
	return &spaHandler{root: root}
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
		http.Error(w, "dashboard not built; run `make build-web` before building the binary", http.StatusServiceUnavailable)
	})
}

type spaHandler struct {
	root fs.FS
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
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
		http.NotFound(w, r)
		return
	}

	f, err := h.root.Open(rel)
	if err != nil {
		// Unknown path: only fall back to the SPA shell for what looks like a
		// client-side route (extensionless, not under assets/). A path that
		// looks like a static asset - assets/missing-hash.js, missing.woff2 -
		// is a real 404: a browser holding a stale index.html across a binary
		// upgrade would otherwise get "200 text/html" for a JS chunk that no
		// longer exists and fail with a cryptic "Unexpected token '<'" instead
		// of a clean, diagnosable 404. Genuine SPA routes (/dashboard/endpoints
		// etc) have no extension and must still resolve to index.html.
		if looksLikeStaticAsset(rel) {
			http.NotFound(w, r)
			return
		}
		h.serveIndex(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.serveIndex(w, r)
		return
	}
	if stat.IsDir() {
		// A directory request (e.g. /dashboard/assets/) is served as
		// index.html: no listing, no escape from the SPA shell.
		h.serveIndex(w, r)
		return
	}

	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	etag := `"` + etagFor(data) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", contentTypeFor(rel))
	w.Header().Set("Cache-Control", cacheControlFor(rel))

	// modtime is serveEpoch rather than the zero time: ServeContent uses it to
	// set Last-Modified and to drive If-Modified-Since precondition checks, so
	// a non-zero value is what makes conditional GET work for clients that
	// prefer that header. The strong ETag above carries If-None-Match.
	http.ServeContent(w, r, path.Base(rel), serveEpoch, bytes.NewReader(data))
}

// serveIndex emits the SPA entry document. It is the fallback for any unknown
// path and for directory requests, so client-side routing keeps working
// without hitting a 404. Cache-Control is no-cache so a new deploy is picked
// up on the next navigation, never a stale service-worker-cached shell.
func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(h.root, indexFile)
	if err != nil {
		http.Error(w, "dashboard index missing", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mimeOverrides[".html"])
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+etagFor(data)+`"`)
	http.ServeContent(w, r, indexFile, serveEpoch, bytes.NewReader(data))
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
