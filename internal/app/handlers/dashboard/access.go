// Package dashboard wires the embedded admin dashboard into Olla's HTTP mux.
//
// This file holds the access-control middleware and the registration
// helper that mounts the dashboard subtree. The static-asset handler itself
// lives in embed.go.
//
// Security posture: there is no authentication. The dashboard relies entirely
// on the two network-layer checks here, both of which must pass per request:
//  1. the TCP source (r.RemoteAddr, never a proxy header) is inside an allowed
//     CIDR;
//  2. r.Host (port stripped, case-insensitive) is either an IP literal or
//     appears in allowed_hosts (IP-literal Hosts are always accepted because
//     DNS rebinding requires the browser to have resolved a hostname).
//
// A rejected request receives a self-diagnosing 403: the body states which
// check failed and names the rejected client IP and Host. There is no auth
// secret to protect by staying silent, and the operator is entitled to see
// the rejected IP/Host so the Docker first-run 403 doesn't resolve to "just
// set 0.0.0.0/0".
package dashboard

import (
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// DashboardRoute is the mux subtree the dashboard owns. Go's http.ServeMux
// treats the trailing slash as a catch-all, so this single registration covers
// /internal/ui/ and every sub-path. Mounted under /internal/ so its assets
// share the same access-policy zone as the data they display, and so no
// profile-driven provider route can collide (provider routes are always
// under /olla/).
const DashboardRoute = "/internal/ui/"

// SlashlessDashboardRoute is the exact-match path without the trailing slash.
// Go's ServeMux answers a request for this path with its own redirect to the
// trailing-slash subtree BEFORE any registered handler (and therefore before
// AccessMiddleware) ever runs, so a disallowed client previously got a bare
// redirect on this one path instead of the self-diagnosing 403 every other
// dashboard path returns - the policy still held on the canonical path, but
// this one leaked its existence. Registering it explicitly, gated by the
// same middleware, closes that gap.
//
// Exported so callers registering routes ahead of the dashboard (see
// server_routes.go's pre-registration collision check) can guard against
// this path too, not just DashboardRoute.
const SlashlessDashboardRoute = "/internal/ui"

// AccessMiddleware returns a handler that enforces the dashboard's
// network-layer policy before delegating to next. cfg must already be Validated
// (RegisterRoutes guarantees this); an unvalidated config with nil parsed CIDRs
// rejects every request, which is the safe failure mode. log may be nil: the
// registry has tests that construct handlers without one, and in that case
// rejections still surface in the response body, just without the Warn line.
func AccessMiddleware(cfg config.DashboardConfig, log logger.StyledLogger, next http.Handler) http.Handler {
	allowedHosts := make(map[string]struct{}, len(cfg.AccessPolicy.AllowedHosts))
	for _, h := range cfg.AccessPolicy.AllowedHosts {
		allowedHosts[strings.ToLower(normaliseHost(h))] = struct{}{}
	}
	parsed := cfg.ParsedCIDRs()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := remoteAddrIP(r.RemoteAddr)

		if !ipInAnyCIDR(clientIP, parsed) {
			reject(w, log, "ip not in allowed range", clientIP, r.Host)
			return
		}

		host := strings.ToLower(normaliseHost(r.Host))
		if !hostAccepted(host, allowedHosts) {
			reject(w, log, "host not accepted", clientIP, r.Host)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// hostAccepted: any Host that parses as an IP literal is accepted
// unconditionally, regardless of allowed_hosts. A non-IP Host must appear in
// the allowlist (case-insensitive, port-stripped on both sides).
func hostAccepted(normalisedHost string, allowedHosts map[string]struct{}) bool {
	if net.ParseIP(normalisedHost) != nil {
		return true
	}
	_, ok := allowedHosts[normalisedHost]
	return ok
}

// RegisterRoutes mounts the dashboard subtree on the registry, gated by
// cfg.Enabled. When the dashboard is disabled the route is absent from the mux
// entirely (default-mux 404), never registered-then-403, so external scanners
// cannot discover the mount point. handler is the static-asset handler from
// embed.go; it is wrapped by AccessMiddleware before registration so the policy
// applies uniformly to /internal/ui/ and every sub-path.
//
// Returns true if the dashboard was registered, false otherwise.
func RegisterRoutes(registry *router.RouteRegistry, cfg config.DashboardConfig, log logger.StyledLogger, handler http.Handler) bool {
	if !cfg.Enabled {
		return false
	}
	guarded := AccessMiddleware(cfg, log, handler)
	registry.RegisterWithMethod(DashboardRoute, http.HandlerFunc(guarded.ServeHTTP), "Admin dashboard (read-only)", "GET")

	// Gated separately from DashboardRoute above: same policy, same security
	// headers, but its own registration so ServeMux never gets the chance to
	// answer this exact path before AccessMiddleware runs.
	slashlessGuarded := AccessMiddleware(cfg, log, http.HandlerFunc(redirectToDashboardRoot))
	registry.RegisterWithMethod(SlashlessDashboardRoute, http.HandlerFunc(slashlessGuarded.ServeHTTP), "Admin dashboard (redirect)", "GET")
	return true
}

// redirectToDashboardRoot issues the canonical redirect to DashboardRoute for
// a client AccessMiddleware has already approved. Security headers are set
// explicitly because http.Redirect's response never passes through the
// static-asset handler's own header-setting path.
func redirectToDashboardRoot(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	http.Redirect(w, r, DashboardRoute, http.StatusTemporaryRedirect)
}

// remoteAddrIP extracts the host portion of r.RemoteAddr via net.SplitHostPort,
// falling back to the raw value if it is not an IP:port pair (IPv6 without
// brackets, unix sockets, tests passing a bare IP). Proxy headers are
// deliberately never consulted here, even though internal/util.GetClientIP will
// honour them under trustProxyHeaders: that path is for the rate limiter, where
// the operator explicitly opted into trusting a reverse proxy CIDR. The
// dashboard has no such trust mechanism, so it reads the TCP source only.
func remoteAddrIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// normaliseHost strips a trailing :port from a host literal AND any IPv6
// brackets, returning the bare host. SplitHostPort already drops brackets for
// the IP:port case, but a bracketed literal with no port (e.g. an allowlist
// entry "[::1]") reaches the fallback. Normalising both sides to the bare form
// means a request Host of "[::1]:41141" compares equal to an allowlist entry of
// "[::1]" or "::1" without the operator guessing which form the check expects.
func normaliseHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		return host[1 : len(host)-1]
	}
	return host
}

// ipInAnyCIDR reports whether ip is inside any of the parsed CIDRs. A nil or
// empty CIDR slice is a deny-all (Validate guarantees a non-empty slice when
// enabled, but defence-in-depth: never silently allow).
func ipInAnyCIDR(ip string, cidrs []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, c := range cidrs {
		if c.Contains(parsed) {
			return true
		}
	}
	return false
}

// reject writes the self-diagnosing 403 response. The body names the failed
// check, echoes the rejected client IP and Host, and names the config keys to
// adjust (dashboard.access_policy.allowed_hosts / allowed_cidrs), so the
// operator reading the response (or a support transcript of it) can see
// exactly what to fix without re-reading YAML. The matching Warn log line
// carries the same detail for the observability path. Plain text, no JSON:
// this is an operational diagnostic, not an API response.
func reject(w http.ResponseWriter, log logger.StyledLogger, reason, clientIP, host string) {
	body := strings.NewReader("403 forbidden: " + reason + " (ip=" + clientIP + ", host=" + host + "). Adjust dashboard.access_policy.allowed_hosts (or allowed_cidrs) in your config.\n")
	// Hardening headers apply to every dashboard response path, including
	// this 403: the body is operator-facing text that must not be frameable,
	// sniffable, or referrer-leaked. setSecurityHeaders is shared with
	// embed.go so the policy is identical across the handler tree.
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.Copy(w, body)
	if log != nil {
		log.Warn("dashboard access denied",
			"reason", reason,
			"client_ip", clientIP,
			"host", host)
	}
}
