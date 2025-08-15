package core

import (
	"net"
	"net/http"
	"slices"
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

// CopyHeaders copies headers from originalReq to proxyReq with proper handling
func CopyHeaders(proxyReq, originalReq *http.Request) {
	proxyReq.Header = make(http.Header)
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

	// SCOUT-581: Host header missing for some requests
	// preserve the original host header which is critical for virtual hosting
	// many backend services rely on this to route requests correctly
	if originalReq.Host != "" {
		proxyReq.Host = originalReq.Host
	}

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
			responseTime := time.Since(stats.StartTime)
			h.Set(constants.HeaderXOllaResponseTime, responseTime.String())
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
