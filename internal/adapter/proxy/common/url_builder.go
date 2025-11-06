package common

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

// BuildTargetURL returns the full target URL for proxying a request.
// - When preserve_path: true, joins endpoint base path with the (relative) request path.
// - Otherwise, replaces the base path with the request path.
// - Normalises/guards against path traversal (including percent-encoded ..) for non-preserve mode.
// - Optimises the common case where the endpoint path is empty or "/".
// Note: path.Clean collapses repeated slashes and removes trailing slashes; this function accepts that.
// See [OLLA-GH-80] https://github.com/thushan/olla/issues/80
func BuildTargetURL(r *http.Request, endpoint *domain.Endpoint, proxyPrefix string) *url.URL {
	targetPath := util.StripPrefix(r.URL.Path, proxyPrefix)
	if targetPath == "" {
		targetPath = "/"
	}

	// preserve_path mode: ensure we don't let an absolute request path discard the endpoint prefix.
	if endpoint.PreservePath && endpoint.URL.Path != "" && endpoint.URL.Path != "/" {
		rel := strings.TrimPrefix(targetPath, "/") // avoid path.Join swallowing the base
		joined := path.Join(endpoint.URL.Path, rel)

		u := *endpoint.URL
		u.Path = joined
		u.RawQuery = r.URL.RawQuery
		u.Fragment = ""
		return &u
	}

	// Non-preserve mode: guard against traversal, including perbent-encoded dot segments.
	// We accept that this also normalises repeated slashes.
	if containsDotDot(targetPath) || containsEncodedDotDot(targetPath) {
		// Clean relative to root to avoid escaping above it.
		clean := path.Clean("/" + targetPath)
		if clean == "" {
			clean = "/"
		}
		targetPath = clean
	}

	// Fast path: endpoint has no base path.
	if endpoint.URL.Path == "" || endpoint.URL.Path == "/" {
		u := *endpoint.URL
		u.Path = targetPath
		u.RawQuery = r.URL.RawQuery
		u.Fragment = ""
		return &u
	}

	// Default: resolve against endpoint base.
	u := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	u.RawQuery = r.URL.RawQuery // always copy; empty is fine
	u.Fragment = ""
	return u
}

// containsDotDot returns true if any decoded path segment is ".." or "."
func containsDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return true
		}
	}
	return false
}

// containsEncodedDotDot detects %2e and %2E sequences forming "." or ".." segments.
func containsEncodedDotDot(p string) bool {
	// Fast check to avoid allocations when nothing is encoded.
	if !strings.Contains(p, "%") {
		return false
	}
	// Best-effort: decode once; if decoding fails, conservatively return true to trigger cleaning.
	dec, err := url.PathUnescape(p)
	if err != nil {
		return true
	}
	return containsDotDot(dec)
}
