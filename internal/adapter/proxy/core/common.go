package core

import (
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/version"
)

var (
	proxiedByHeader string
	viaHeader       string
)

func init() {
	proxiedByHeader = version.Name + "/" + version.Version
	viaHeader = "1.1 " + version.ShortName + "/" + version.Version
}

// GetProxiedByHeader returns the X-Proxied-By header value
func GetProxiedByHeader() string {
	return proxiedByHeader
}

// GetViaHeader returns the Via header value
func GetViaHeader() string {
	return viaHeader
}

// CopyHeaders copies headers from originalReq to proxyReq with proper handling.
// endpoint carries the per-endpoint auth and custom header config applied after
// the client headers are copied and the sensitive strip list runs.
func CopyHeaders(proxyReq, originalReq *http.Request, endpoint *domain.Endpoint) {
	// Pre-size based on source to avoid rehashing
	if proxyReq.Header == nil {
		proxyReq.Header = make(http.Header, len(originalReq.Header))
	}
	for header, values := range originalReq.Header {
		// Skip hop-by-hop headers as per RFC 2616 section 13.5.1
		// these headers are connection-specific and shouldn't be forwarded
		if isHopByHopHeader(header) {
			continue
		}

		// SECURITY: Filter sensitive headers to prevent credential leakage
		// TODO: we should consider a more copmrehensive security policy / technique here
		normalisedHeader := http.CanonicalHeaderKey(header)
		if normalisedHeader == constants.HeaderAuthorization ||
			normalisedHeader == constants.HeaderCookie ||
			normalisedHeader == constants.HeaderXAPIKey ||
			normalisedHeader == constants.HeaderXAuthToken ||
			normalisedHeader == constants.HeaderProxyAuthorization {
			continue
		}

		proxyReq.Header[header] = values
	}

	// Deprecates SCOUT-581, superseded by OLLA-135:
	// https://github.com/thushan/olla/issues/135
	// Do not propagate the inbound Host onto the outbound request.
	// req.URL.Host is authoritative for the backend; X-Forwarded-Host (set below) carries
	// the original value for backends that need it.

	// Add proxy identification headers
	proxyReq.Header.Set(constants.HeaderXProxiedBy, GetProxiedByHeader())

	// Via header tracks the request path through proxies (RFC 7230 section 5.7.1)
	// we append to existing via headers to maintain the proxy chain
	if via := originalReq.Header.Get(constants.HeaderVia); via != "" {
		proxyReq.Header.Set(constants.HeaderVia, via+", "+GetViaHeader())
	} else {
		proxyReq.Header.Set(constants.HeaderVia, GetViaHeader())
	}

	// SHERPA-44: Ensure X-Real-IP header is set
	// Add real IP tracking headers
	if realIP := originalReq.Header.Get(constants.HeaderXRealIP); realIP == "" {
		if ip := extractClientIP(originalReq); ip != "" {
			proxyReq.Header.Set(constants.HeaderXRealIP, ip)
		}
	}

	// Update or set X-Forwarded headers
	updateForwardedHeaders(proxyReq, originalReq)

	// Apply endpoint-level custom headers after the strip so operators can explicitly
	// re-introduce a header that the strip removed (e.g. a backend that needs X-Api-Key).
	// Auth is applied after these so the auth: section always wins on conflict — if the
	// user accidentally puts Authorization in headers: and auth:, auth: takes precedence.
	if endpoint != nil {
		for name, value := range endpoint.Headers {
			proxyReq.Header.Set(name, value)
		}

		// Auth wins over anything in the headers: map and over anything the client sent.
		// The strip loop above already removed client credentials; Set() here is
		// defensive so future strip-list gaps can't leak client creds to the upstream.
		if endpoint.AuthHeaderName != "" {
			proxyReq.Header.Set(endpoint.AuthHeaderName, endpoint.AuthHeaderValue)
		}
	}
}

// SHERPA-81: Update X-Forwarded-* headers in request
// updateForwardedHeaders updates X-Forwarded-* headers
func updateForwardedHeaders(proxyReq, originalReq *http.Request) {
	// X-Forwarded-For
	if forwarded := originalReq.Header.Get(constants.HeaderXForwardedFor); forwarded != "" {
		if clientIP := extractClientIP(originalReq); clientIP != "" {
			proxyReq.Header.Set(constants.HeaderXForwardedFor, forwarded+", "+clientIP)
		} else {
			proxyReq.Header.Set(constants.HeaderXForwardedFor, forwarded)
		}
	} else if clientIP := extractClientIP(originalReq); clientIP != "" {
		proxyReq.Header.Set(constants.HeaderXForwardedFor, clientIP)
	}

	// X-Forwarded-Proto
	if proto := originalReq.Header.Get(constants.HeaderXForwardedProto); proto == "" {
		if originalReq.TLS != nil {
			proxyReq.Header.Set(constants.HeaderXForwardedProto, constants.ProtocolHTTPS)
		} else {
			proxyReq.Header.Set(constants.HeaderXForwardedProto, constants.ProtocolHTTP)
		}
	}

	// X-Forwarded-Host
	if host := originalReq.Header.Get(constants.HeaderXForwardedHost); host == "" && originalReq.Host != "" {
		proxyReq.Header.Set(constants.HeaderXForwardedHost, originalReq.Host)
	}
}

var hopByHopHeaders = []string{
	constants.HeaderConnection,
	constants.HeaderKeepAlive,
	constants.HeaderProxyAuthenticate,
	constants.HeaderProxyAuthorization,
	constants.HeaderTE,
	constants.HeaderTrailer,
	constants.HeaderTransferEncoding,
	constants.HeaderUpgrade,
}

// isHopByHopHeader checks if a header is hop-by-hop
func isHopByHopHeader(header string) bool {
	return slices.ContainsFunc(hopByHopHeaders, func(h string) bool {
		return strings.EqualFold(h, header)
	})
}

// extractClientIP extracts the client IP address from the request
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get(constants.HeaderXForwardedFor); xff != "" {
		// Take the first IP in the comma-separated list
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// SHERPA-89: Check X-Forwarded-Host header is set
	// Check X-Real-IP header
	if xri := r.Header.Get(constants.HeaderXRealIP); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// responseHeaderStripList holds upstream response headers that must never reach
// the client. A backend that reflects auth credentials or sets cookies is almost
// certainly misconfigured; stripping here prevents credential leakage in the rare
// case where a compromised or buggy upstream reflects these headers back.
var responseHeaderStripList = []string{
	constants.HeaderAuthorization,
	constants.HeaderProxyAuthorization,
	constants.HeaderXAPIKey,
	constants.HeaderXAuthToken,
	"Set-Cookie",
}

// CopyResponseHeaders copies upstream response headers to the client, filtering
// headers that should never leave the proxy boundary. Use this at every site that
// copies resp.Header to w.Header() to keep the strip list consistent.
//
// The deny set is the union of the static strip list, the endpoint's auth header
// name, and every key in the endpoint's custom header map. Operator-configured
// headers must be stripped on the return path for the same reason they're set on
// the outbound path: if a compromised backend reflects them, the client would
// receive credentials it has no business seeing. Pass nil endpoint to use only the
// static list (safe for callers without endpoint context, though all current call
// sites have one).
func CopyResponseHeaders(dst http.Header, src http.Header, endpoint *domain.Endpoint) {
	// Build a transient deny set: static list + endpoint-specific names.
	// For the common case (no endpoint or empty config) this stays small.
	deny := make(map[string]struct{}, len(responseHeaderStripList)+2)
	for _, h := range responseHeaderStripList {
		deny[h] = struct{}{}
	}
	if endpoint != nil {
		if endpoint.AuthHeaderName != "" {
			deny[http.CanonicalHeaderKey(endpoint.AuthHeaderName)] = struct{}{}
		}
		for name := range endpoint.Headers {
			deny[http.CanonicalHeaderKey(name)] = struct{}{}
		}
	}

	for key, values := range src {
		if _, blocked := deny[http.CanonicalHeaderKey(key)]; blocked {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// SetStickySessionHeaders writes sticky session outcome headers before WriteHeader
// is called. It reads the StickyOutcome pointer that was injected into the context
// by the handler layer after the balancer's Select fills it. Must be called before
// w.WriteHeader() — calling it afterwards is a no-op on a committed response.
func SetStickySessionHeaders(w http.ResponseWriter, r *http.Request) {
	outcome, _ := r.Context().Value(constants.ContextStickyOutcomeKey).(*domain.StickyOutcome)
	if outcome == nil {
		return
	}
	h := w.Header()
	if outcome.Result != "" {
		h.Set(constants.HeaderXOllaStickySession, outcome.Result)
	}
	if outcome.Source != "" && outcome.Source != "none" {
		h.Set(constants.HeaderXOllaStickyKeySource, outcome.Source)
	}
	if outcome.Source == "session_header" {
		if sid := r.Header.Get(constants.HeaderXOllaSessionID); sid != "" {
			h.Set(constants.HeaderXOllaSessionID, sid)
		}
	}
}

// SetResponseHeaders sets common response headers
func SetResponseHeaders(w http.ResponseWriter, stats *ports.RequestStats, endpoint *domain.Endpoint) {
	h := w.Header()

	// Set proxy identification headers
	h.Set(constants.HeaderXServedBy, GetProxiedByHeader())
	h.Set(constants.HeaderVia, GetViaHeader())

	// Set request tracking headers
	if stats != nil {
		if stats.RequestID != "" {
			h.Set(constants.HeaderXOllaRequestID, stats.RequestID)
		}

		// Calculate and set response time if we have timing information
		if !stats.StartTime.IsZero() {
			responseTimeMs := time.Since(stats.StartTime).Milliseconds()
			h.Set(constants.HeaderXOllaResponseTime, strconv.FormatInt(responseTimeMs, 10)+"ms")
		}

		// set routing decision headers if available
		if stats.RoutingDecision != nil {
			h.Set(constants.HeaderXOllaRoutingStrategy, stats.RoutingDecision.Strategy)
			h.Set(constants.HeaderXOllaRoutingDecision, stats.RoutingDecision.Action)
			if stats.RoutingDecision.Reason != "" {
				h.Set(constants.HeaderXOllaRoutingReason, stats.RoutingDecision.Reason)
			}
		}
	}

	// Set endpoint information
	if endpoint != nil {
		h.Set(constants.HeaderXOllaEndpoint, endpoint.Name)
		h.Set(constants.HeaderXOllaBackendType, endpoint.Type)

		// Set model header if available
		if stats != nil && stats.Model != "" {
			h.Set(constants.HeaderXOllaModel, stats.Model)
		}
	}
}
