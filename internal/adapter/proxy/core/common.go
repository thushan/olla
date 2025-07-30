package core

import (
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/version"
	"net"
	"net/http"
	"slices"
	"strings"
)

const (
	HeaderRequestID   = "X-Olla-Request-ID"
	HeaderEndpoint    = "X-Olla-Endpoint"
	HeaderBackendType = "X-Olla-Backend-Type"
	HeaderModel       = "X-Olla-Model"
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
		if normalisedHeader == "Authorization" ||
			normalisedHeader == "Cookie" ||
			normalisedHeader == "X-Api-Key" ||
			normalisedHeader == "X-Auth-Token" ||
			normalisedHeader == "Proxy-Authorization" {
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
	proxyReq.Header.Set("X-Proxied-By", GetProxiedByHeader())

	// Via header tracks the request path through proxies (RFC 7230 section 5.7.1)
	// we append to existing via headers to maintain the proxy chain
	if via := originalReq.Header.Get("Via"); via != "" {
		proxyReq.Header.Set("Via", via+", "+GetViaHeader())
	} else {
		proxyReq.Header.Set("Via", GetViaHeader())
	}

	// SHERPA-44: Ensure X-Real-IP header is set
	// Add real IP tracking headers
	if realIP := originalReq.Header.Get("X-Real-IP"); realIP == "" {
		if ip := extractClientIP(originalReq); ip != "" {
			proxyReq.Header.Set("X-Real-IP", ip)
		}
	}

	// Update or set X-Forwarded headers
	updateForwardedHeaders(proxyReq, originalReq)
}

// SHERPA-81: Update X-Forwarded-* headers in request
// updateForwardedHeaders updates X-Forwarded-* headers
func updateForwardedHeaders(proxyReq, originalReq *http.Request) {
	// X-Forwarded-For
	if forwarded := originalReq.Header.Get("X-Forwarded-For"); forwarded != "" {
		if clientIP := extractClientIP(originalReq); clientIP != "" {
			proxyReq.Header.Set("X-Forwarded-For", forwarded+", "+clientIP)
		} else {
			proxyReq.Header.Set("X-Forwarded-For", forwarded)
		}
	} else if clientIP := extractClientIP(originalReq); clientIP != "" {
		proxyReq.Header.Set("X-Forwarded-For", clientIP)
	}

	// X-Forwarded-Proto
	if proto := originalReq.Header.Get("X-Forwarded-Proto"); proto == "" {
		if originalReq.TLS != nil {
			proxyReq.Header.Set("X-Forwarded-Proto", constants.ProtocolHTTP)
		} else {
			proxyReq.Header.Set("X-Forwarded-Proto", constants.ProtocolHTTPS)
		}
	}

	// X-Forwarded-Host
	if host := originalReq.Header.Get("X-Forwarded-Host"); host == "" && originalReq.Host != "" {
		proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	}
}

// isHopByHopHeader checks if a header is hop-by-hop
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
	return slices.ContainsFunc(hopByHopHeaders, func(h string) bool {
		return strings.EqualFold(h, header)
	})
}

// extractClientIP extracts the client IP address from the request
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the comma-separated list
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// SHERPA-89: Check X-Forwarded-Host header is set
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
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
	h.Set("X-Served-By", GetProxiedByHeader())
	h.Set("Via", GetViaHeader())

	// Set request tracking headers
	if stats.RequestID != "" {
		h.Set(HeaderRequestID, stats.RequestID)
	}

	// Set endpoint information
	if endpoint != nil {
		h.Set(HeaderEndpoint, endpoint.Name)
		h.Set(HeaderBackendType, endpoint.Type)

		// Set model header if available
		if stats.Model != "" {
			h.Set(HeaderModel, stats.Model)
		}
	}
}
