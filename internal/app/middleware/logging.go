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

// consoleLogParams resolves the level and message the console log lines
// (request started/completed) should use for a given quiet gate result.
// Factored out so the pre- and post-request blocks share one place that
// decides "quiet => Debug with the terse message, otherwise Info with the
// operator-facing one" instead of each spelling out its own if/else.
func consoleLogParams(quiet bool, quietMsg, loudMsg string) (slog.Level, string) {
	if quiet {
		return slog.LevelDebug, quietMsg
	}
	return slog.LevelInfo, loudMsg
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

// CombinedLoggingMiddleware fuses the console log and the access log into a
// single pass over the request. This used to be two separate middlewares
// chained together, which meant every request built two responseWriter
// wrappers, read time.Now() twice and derived the request ID twice for no
// behavioural benefit, since both wrappers observed the same status/size. This
// wraps once, times once, and feeds both log outputs from that single pass.
//
// The two outputs keep their independently-decided quiet gates -
// isQuietPollOutcome for the console line, isQuietAccessOutcome for the access
// line - see their doc comments for why they must not be unified: the access
// log is the operator's audit record and must not go quiet on proxy traffic
// the way the console line deliberately does.
func CombinedLoggingMiddleware(styledLogger logger.StyledLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := sanitiseRequestID(r.Header.Get(constants.HeaderXRequestID))
			if requestID == "" {
				requestID = util.GenerateRequestID()
			}

			requestSize := r.ContentLength
			if requestSize < 0 {
				requestSize = 0
			}

			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			baseLogger := slog.Default().With(constants.ContextRequestIdKey, requestID)
			ctx = context.WithValue(ctx, LoggerKey, baseLogger)

			w.Header().Set("X-Olla-Request-ID", requestID)

			wrapped := &responseWriter{ResponseWriter: w, status: 200}

			preQuiet := isQuietPollRoute(r.Method, r.URL.Path)
			level, msg := consoleLogParams(preQuiet, "HTTP request started", "Request started")
			if baseLogger.Enabled(ctx, level) {
				baseLogger.Log(ctx, level, msg,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"user_agent", r.UserAgent(),
					"request_bytes", requestSize,
					"request_size_formatted", formatBytes(requestSize),
				)
			}

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)

			postQuiet := isQuietPollOutcome(r.Method, r.URL.Path, wrapped.status)
			level, msg = consoleLogParams(postQuiet, "HTTP request completed", "Request completed")
			if baseLogger.Enabled(ctx, level) {
				baseLogger.Log(ctx, level, msg,
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

			// Access log uses a plain (non-.With'd) logger since request_id is
			// already an explicit field below - attaching it via .With() as well
			// would double it up in the emitted record.
			accessLogger := slog.Default()
			detailedCtx := context.WithValue(r.Context(), logger.DefaultDetailedCookie, true)
			accessLevel := accessLogLevel(r.Method, r.URL.Path, wrapped.status)
			if accessLogger.Enabled(detailedCtx, accessLevel) {
				accessLogger.Log(detailedCtx, accessLevel, "Access log",
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
