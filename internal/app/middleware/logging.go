package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/util"

	"github.com/thushan/olla/internal/logger"
)

// Context keys for request ID and logger
type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	LoggerKey    contextKey = "logger"
)

const (
	// maxRequestIDLength caps the inbound X-Request-ID length. Values beyond this
	// are most likely probing attempts or misconfigured clients; generating a fresh
	// ID is cheaper than propagating an unbounded string into every log line.
	maxRequestIDLength = 128
)

// sanitiseRequestID validates a client-supplied request ID and returns it
// unchanged if it passes. An empty string signals the caller to generate a
// fresh ID instead. Rejected when: longer than maxRequestIDLength, or contains
// any character that is not a printable ASCII non-space (CR, LF, NUL, tabs and
// other control characters are log-injection vectors).
func sanitiseRequestID(id string) string {
	if len(id) > maxRequestIDLength {
		return ""
	}
	for _, c := range id {
		// Only allow printable ASCII (0x21–0x7E). Space (0x20) is technically
		// printable but rarely intentional in IDs and trips some log parsers.
		if c < 0x21 || c > 0x7E {
			return ""
		}
	}
	return id
}

// IsProxyRequest determines if a request is for the proxy endpoints
// Used to decide logging levels to avoid redundancy with proxy handler logging
func IsProxyRequest(path string) bool {
	// checks for proxy prefixes
	// /olla/ is the main proxy prefix
	return strings.Contains(path, constants.DefaultOllaProxyPathPrefix) ||
		(strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/v0/")) // /api/v0/ is internal
}

// internalPathPrefix is where the admin dashboard's status/health/stats polls
// land. An open dashboard tab polls several of these every few seconds; at
// info level that floods both the console log and the dedicated access log
// with traffic that carries no operational signal, so successful polling is
// demoted to debug alongside the existing proxy hot-path treatment. A 404,
// wrong-method request, 5xx, or repeated 403 under /internal/ (e.g. the
// dashboard's access-control gate being probed) must never be swallowed by
// this, so quieting is conditioned on outcome, not just path — see
// isQuietPollOutcome.
const internalPathPrefix = "/internal/"

// isInternalPollMethod reports whether method is one a polling GET/HEAD
// client would use. Anything else (POST, PUT, DELETE, ...) against
// /internal/ is never routine polling traffic and must always log normally.
func isInternalPollMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// isQuietPollRoute reports whether path/method belongs to a route whose
// traffic is presumptively routine and high-frequency, for the PRE-request
// log line where the response status does not exist yet: the proxy hot path
// (already unconditionally demoted) plus a GET/HEAD request under /internal/.
// This is deliberately optimistic — chosen approach (b): quiet the
// pre-request line by method/path alone, and let the post-request line
// (isQuietPollOutcome) apply the real, status-aware gate. The alternative,
// deferring the pre-request decision entirely to the post-request call,
// would mean either logging every dashboard poll's "started" line at Info
// (reintroducing the flood this exists to prevent) or logging it at Debug
// unconditionally including for non-GET/HEAD internal requests, which is a
// worse trade for a line that carries no status/duration/diagnostic value
// anyway. The "completed" line below is the one that must never hide a
// real problem, and it does not depend on this pre-request line's decision.
func isQuietPollRoute(method, path string) bool {
	return IsProxyRequest(path) || (isInternalPollMethod(method) && strings.HasPrefix(path, internalPathPrefix))
}

// isQuietPollOutcome is the authoritative, status-aware gate applied once
// the response is known: the POST-request log line and the single access-log
// line both use this, not isQuietPollRoute. A /internal/ request is only
// treated as quiet polling traffic when it is GET/HEAD AND the response was
// 2xx or 304 — anything else (404, 4xx, 5xx, or a non-GET/HEAD method that
// happened to succeed) logs at its normal level regardless of path, so a
// probed dashboard access-control 403, a wrong-route 404, or a 500 is never
// invisible at the default Info level just because it lives under /internal/.
func isQuietPollOutcome(method, path string, status int) bool {
	if IsProxyRequest(path) {
		return true
	}
	if !isInternalPollMethod(method) || !strings.HasPrefix(path, internalPathPrefix) {
		return false
	}
	return (status >= 200 && status < 300) || status == http.StatusNotModified
}

// isQuietAccessOutcome is the access-log analogue of isQuietPollOutcome.
// The access log is the operator's audit record of every request that hit
// Olla, so unlike the console path it must NOT quiet proxy traffic: a proxy
// success is the audit-worthy outcome the security-practices doc promises to
// record at the default Info level, and a proxy 4xx/5xx must never disappear
// into Debug. Only the dashboard's own /internal/ polling surface is
// routine enough to demote, and even then only for GET/HEAD + 2xx/304:
// a 404, 403, 5xx, or non-GET/HEAD method under /internal/ stays loud so a
// probed access-control gate or failing handler is visible at Info. The
// console path (isQuietPollOutcome) intentionally still quiets proxy traffic
// regardless of status because the console has dedicated proxy-handler
// logging; do not unify the two.
func isQuietAccessOutcome(method, path string, status int) bool {
	if !isInternalPollMethod(method) || !strings.HasPrefix(path, internalPathPrefix) {
		return false
	}
	return (status >= 200 && status < 300) || status == http.StatusNotModified
}

// accessLogLevel resolves the slog level the access log should emit at for a
// given outcome, applying the status-aware isQuietAccessOutcome gate. Factored
// out as a pure function so the level table is testable directly without a
// capturing slog handler.
func accessLogLevel(method, path string, status int) slog.Level {
	if isQuietAccessOutcome(method, path, status) {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// responseWriter wraps http.ResponseWriter to capture response size and status
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int64
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += int64(size)
	return size, err
}

func (rw *responseWriter) WriteHeader(s int) {
	rw.status = s
	rw.ResponseWriter.WriteHeader(s)
}

// Flush implements http.Flusher interface
func (rw *responseWriter) Flush() {
	// OLLA-102: Choppy output in streaming responses
	// We need to flush the underlying response writer
	// for streaming responses, otherwise buffers will
	// not be sent immediately causing choppy output.
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// GetLogger retrieves a logger with request ID from context
func GetLogger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(LoggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// EnhancedLoggingMiddleware adds request ID to logger context and logs request/response details
func EnhancedLoggingMiddleware(styledLogger logger.StyledLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Get or create request ID. Validate the inbound value to prevent
			// log injection via CR/LF or non-printable characters, and to cap
			// the length so structured log fields stay bounded.
			requestID := sanitiseRequestID(r.Header.Get(constants.HeaderXRequestID))
			if requestID == "" {
				requestID = util.GenerateRequestID()
			}

			// Calculate request size
			requestSize := r.ContentLength
			if requestSize < 0 {
				requestSize = 0
			}

			// Add to context for propagation
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

			// Create a base logger with request ID. slog.With allocates; it runs
			// regardless of level because the logger is stored in context for handlers
			// that may log at any level. This is unavoidable and is not gated.
			baseLogger := slog.Default().With(constants.ContextRequestIdKey, requestID)
			ctx = context.WithValue(ctx, LoggerKey, baseLogger)

			// Add to response header for client tracking
			w.Header().Set("X-Olla-Request-ID", requestID)

			// Wrap response writer to capture metrics
			wrapped := &responseWriter{ResponseWriter: w, status: 200}

			// Gate field construction on whether the record will actually be emitted.
			// On the proxy hot path and the dashboard's /internal/ polling surface, at
			// the default info level Debug records are discarded by the handler —
			// building the []any slice and calling formatBytes 2x per request is pure
			// waste. Other requests log at Info, so they only pay the cost when
			// info-level logging is actually enabled.
			//
			// This pre-request line uses the optimistic, status-blind gate
			// (isQuietPollRoute) since the response doesn't exist yet — see its
			// doc comment for why. The post-request line below uses the
			// status-aware gate (isQuietPollOutcome) instead.
			preQuiet := isQuietPollRoute(r.Method, r.URL.Path)
			if preQuiet {
				if baseLogger.Enabled(ctx, slog.LevelDebug) {
					baseLogger.Debug("HTTP request started",
						"method", r.Method,
						"path", r.URL.Path,
						"remote_addr", r.RemoteAddr,
						"user_agent", r.UserAgent(),
						"request_bytes", requestSize,
						"request_size_formatted", formatBytes(requestSize),
					)
				}
			} else {
				if baseLogger.Enabled(ctx, slog.LevelInfo) {
					baseLogger.Info("Request started",
						"method", r.Method,
						"path", r.URL.Path,
						"remote_addr", r.RemoteAddr,
						"user_agent", r.UserAgent(),
						"request_bytes", requestSize,
						"request_size_formatted", formatBytes(requestSize),
					)
				}
			}

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)

			// The completed line is the one that must never hide a real problem,
			// so it re-evaluates quietness with the response status known: a 404,
			// 5xx, or non-GET/HEAD request under /internal/ always logs here at
			// its normal level even if the pre-request line above was quieted.
			postQuiet := isQuietPollOutcome(r.Method, r.URL.Path, wrapped.status)
			if postQuiet {
				if baseLogger.Enabled(ctx, slog.LevelDebug) {
					baseLogger.Debug("HTTP request completed",
						"method", r.Method,
						"path", r.URL.Path,
						"status", wrapped.status,
						"duration_ms", duration.Milliseconds(),
						"duration_formatted", duration.String(),
						"request_bytes", requestSize,
						"response_bytes", wrapped.size,
						"size_flow", fmt.Sprintf("%s -> %s", formatBytes(requestSize), formatBytes(wrapped.size)),
					)
				}
			} else {
				if baseLogger.Enabled(ctx, slog.LevelInfo) {
					baseLogger.Info("Request completed",
						"method", r.Method,
						"path", r.URL.Path,
						"status", wrapped.status,
						"duration_ms", duration.Milliseconds(),
						"duration_formatted", duration.String(),
						"request_bytes", requestSize,
						"response_bytes", wrapped.size,
						"size_flow", fmt.Sprintf("%s -> %s", formatBytes(requestSize), formatBytes(wrapped.size)),
					)
				}
			}
		})
	}
}

// AccessLoggingMiddleware provides structured access logging for detailed analysis
func AccessLoggingMiddleware(styledLogger logger.StyledLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Use existing request ID from context or create one
			requestID := GetRequestID(r.Context())
			if requestID == "" {
				requestID = util.GenerateRequestID()
				ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
				r = r.WithContext(ctx)
			}

			// Calculate request size
			requestSize := r.ContentLength
			if requestSize < 0 {
				requestSize = 0
			}

			wrapped := &responseWriter{ResponseWriter: w, status: 200}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// Access logs route to a dedicated file handler (keyed by DefaultDetailedCookie).
			// Build fields only when the handler is enabled to avoid allocating the
			// format.RFC3339 string, redactQuery output, and the variadic slice on every
			// request when the file sink is not configured.
			//
			// Unlike the console line, the access log is status-aware for proxy
			// traffic: a proxy 4xx/5xx must surface at Info here (the access log is
			// the operator's audit record), while routine proxy success and
			// successful /internal/ polls stay at Debug. See isQuietAccessOutcome.
			baseLogger := slog.Default()
			detailedCtx := context.WithValue(r.Context(), logger.DefaultDetailedCookie, true)
			level := accessLogLevel(r.Method, r.URL.Path, wrapped.status)
			if baseLogger.Enabled(detailedCtx, level) {
				baseLogger.Log(detailedCtx, level, "Access log",
					"timestamp", start.Format(time.RFC3339),
					"request_id", requestID,
					"remote_addr", r.RemoteAddr,
					"method", r.Method,
					"path", r.URL.Path,
					"query", redactQuery(r.URL.RawQuery),
					"status", wrapped.status,
					"request_bytes", requestSize,
					"response_bytes", wrapped.size,
					"duration_ms", duration.Milliseconds(),
					"user_agent", r.UserAgent(),
					"referer", r.Referer(),
					"content_type", r.Header.Get(constants.HeaderContentType),
					"accept", r.Header.Get(constants.HeaderAccept))
			}
		})
	}
}

// sensitiveQueryKeys lists query parameter names whose values must never appear
// in logs. Values are compared case-insensitively.
var sensitiveQueryKeys = []string{
	"api_key", "token", "access_token", "key", "password", "secret", "auth",
}

// redactQuery returns a sanitised version of a raw query string with values for
// sensitive parameter names replaced by [REDACTED]. It does not modify the
// original string; callers should use the return value for logging only.
func redactQuery(raw string) string {
	if raw == "" {
		return raw
	}

	// Parse into individual key=value pairs while preserving order and raw form.
	// We rebuild manually rather than using url.Values.Encode() because the latter
	// percent-encodes bracket characters in "[REDACTED]".
	pairs := strings.Split(raw, "&")
	var changed bool
	out := make([]string, len(pairs))

	for i, pair := range pairs {
		k, _, hasVal := strings.Cut(pair, "=")
		if !hasVal {
			out[i] = pair
			continue
		}
		// Decode the key for comparison so percent-encoded forms like
		// %70assword (password) are caught. Fall back to the raw key if
		// the escape sequence is malformed.
		decoded, decodeErr := url.QueryUnescape(k)
		if decodeErr != nil {
			decoded = k
		}
		sensitive := false
		for _, sk := range sensitiveQueryKeys {
			if strings.EqualFold(decoded, sk) {
				sensitive = true
				break
			}
		}
		if sensitive {
			out[i] = k + "=[REDACTED]"
			changed = true
		} else {
			out[i] = pair
		}
	}

	if !changed {
		return raw
	}
	return strings.Join(out, "&")
}

// formatBytes converts byte count to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	const suffixes = "KMGTPE"

	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	size := float64(bytes) / float64(div)
	return fmt.Sprintf("%.1f%cB", size, suffixes[exp])
}

// FormatBytes is the exported version for external use
func FormatBytes(bytes int64) string {
	return formatBytes(bytes)
}
