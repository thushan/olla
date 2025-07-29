package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
)

const (
	// Header constants
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
		// This is CRITICAL for edge deployments on Cloudflare/Akamai
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

	// Add real IP tracking headers
	if realIP := originalReq.Header.Get("X-Real-IP"); realIP == "" {
		if ip := extractClientIP(originalReq); ip != "" {
			proxyReq.Header.Set("X-Real-IP", ip)
		}
	}

	// Update or set X-Forwarded headers
	updateForwardedHeaders(proxyReq, originalReq)
}

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
			proxyReq.Header.Set("X-Forwarded-Proto", "https")
		} else {
			proxyReq.Header.Set("X-Forwarded-Proto", "http")
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

// WriteErrorResponse writes a standard error response
func WriteErrorResponse(w http.ResponseWriter, statusCode int, err error, stats *ports.RequestStats) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if stats != nil && stats.RequestID != "" {
		w.Header().Set(HeaderRequestID, stats.RequestID)
	}

	w.WriteHeader(statusCode)
	fmt.Fprintln(w, err.Error())
}

// ProxyStats contains common proxy statistics
type ProxyStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	TotalLatency       int64
	MinLatency         int64
	MaxLatency         int64
}

// RecordSuccess records a successful proxy request
func (s *ProxyStats) RecordSuccess(latency int64) {
	atomic.AddInt64(&s.SuccessfulRequests, 1)
	atomic.AddInt64(&s.TotalLatency, latency)

	// Update min latency
	for {
		oldMin := atomic.LoadInt64(&s.MinLatency)
		if oldMin != 0 && oldMin <= latency {
			break
		}
		if atomic.CompareAndSwapInt64(&s.MinLatency, oldMin, latency) {
			break
		}
	}

	// Update max latency
	for {
		oldMax := atomic.LoadInt64(&s.MaxLatency)
		if oldMax >= latency {
			break
		}
		if atomic.CompareAndSwapInt64(&s.MaxLatency, oldMax, latency) {
			break
		}
	}
}

// RecordFailure records a failed proxy request
func (s *ProxyStats) RecordFailure() {
	atomic.AddInt64(&s.FailedRequests, 1)
}

// GetStats returns current statistics
func (s *ProxyStats) GetStats() ports.ProxyStats {
	total := atomic.LoadInt64(&s.TotalRequests)
	successful := atomic.LoadInt64(&s.SuccessfulRequests)
	failed := atomic.LoadInt64(&s.FailedRequests)
	totalLatency := atomic.LoadInt64(&s.TotalLatency)

	avgLatency := int64(0)
	if successful > 0 {
		avgLatency = totalLatency / successful
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
		MinLatency:         atomic.LoadInt64(&s.MinLatency),
		MaxLatency:         atomic.LoadInt64(&s.MaxLatency),
	}
}

// StreamResponse performs buffered streaming with proper error handling
func StreamResponse(ctx context.Context, w http.ResponseWriter, body io.Reader, bufferSize int, rlog logger.StyledLogger) (int, error) {
	buffer := make([]byte, bufferSize)
	totalBytes := 0
	flusher, canFlush := w.(http.Flusher)

	for {
		select {
		case <-ctx.Done():
			return totalBytes, ctx.Err()
		default:
			n, err := body.Read(buffer)
			if n > 0 {
				written, writeErr := w.Write(buffer[:n])
				totalBytes += written

				if writeErr != nil {
					rlog.Debug("write error during streaming", "error", writeErr, "bytes_written", totalBytes)
					return totalBytes, writeErr
				}

				if canFlush {
					flusher.Flush()
				}
			}

			if err != nil {
				if err == io.EOF {
					return totalBytes, nil
				}
				rlog.Debug("read error during streaming", "error", err, "bytes_read", totalBytes)
				return totalBytes, err
			}
		}
	}
}

// RecordRequestMetrics records common request metrics
func RecordRequestMetrics(ctx context.Context, statsCollector ports.StatsCollector, endpoint *domain.Endpoint,
	duration time.Duration, bytesTransferred int64, success bool) {

	if statsCollector == nil || endpoint == nil {
		return
	}

	status := "success"
	if !success {
		status = "error"
	}

	statsCollector.RecordRequest(endpoint, status, duration, bytesTransferred)
}
