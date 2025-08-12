package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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

// IsProxyRequest determines if a request is for the proxy endpoints
// Used to decide logging levels to avoid redundancy with proxy handler logging
func IsProxyRequest(path string) bool {
	// checks for proxy prefixes
	// /olla/ is the main proxy prefix
	return strings.Contains(path, constants.DefaultOllaProxyPathPrefix) ||
		(strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/v0/")) // /api/v0/ is internal
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

			// Get or create request ID
			requestID := r.Header.Get(constants.HeaderXRequestID)
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

			// Create a base logger with request ID
			baseLogger := slog.Default().With(constants.ContextRequestIdKey, requestID)
			ctx = context.WithValue(ctx, LoggerKey, baseLogger)

			// Add to response header for client tracking
			w.Header().Set("X-Olla-Request-ID", requestID)

			// Wrap response writer to capture metrics
			wrapped := &responseWriter{ResponseWriter: w, status: 200}

			// Log request start - use Debug for proxy requests to reduce noise
			// Proxy requests will log their own "Request received" at INFO level
			logFields := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"request_bytes", requestSize,
				"request_size_formatted", formatBytes(requestSize),
			}

			if IsProxyRequest(r.URL.Path) {
				// For proxy requests, just log at debug level since handler will log INFO
				baseLogger.Debug("HTTP request started", logFields...)
			} else {
				// For non-proxy requests (health, status, etc), log at INFO
				baseLogger.Info("Request started", logFields...)
			}

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)

			// Log request completion - use Debug for proxy requests to reduce noise
			completionFields := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration_ms", duration.Milliseconds(),
				"duration_formatted", duration.String(),
				"request_bytes", requestSize,
				"response_bytes", wrapped.size,
				"size_flow", fmt.Sprintf("%s -> %s", formatBytes(requestSize), formatBytes(wrapped.size)),
			}

			if IsProxyRequest(r.URL.Path) {
				// For proxy requests, just log at debug level since handler will log INFO
				baseLogger.Debug("HTTP request completed", completionFields...)
			} else {
				// For non-proxy requests, log at INFO
				baseLogger.Info("Request completed", completionFields...)
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

			// Create detailed context for file logging only
			detailedCtx := context.WithValue(r.Context(), logger.DefaultDetailedCookie, true)

			// Log detailed access information (file only)
			baseLogger := slog.Default()
			baseLogger.InfoContext(detailedCtx, "Access log",
				"timestamp", start.Format(time.RFC3339),
				"request_id", requestID,
				"remote_addr", r.RemoteAddr,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"status", wrapped.status,
				"request_bytes", requestSize,
				"response_bytes", wrapped.size,
				"duration_ms", duration.Milliseconds(),
				"user_agent", r.UserAgent(),
				"referer", r.Referer(),
				"content_type", r.Header.Get(constants.HeaderContentType),
				"accept", r.Header.Get(constants.HeaderAccept))
		})
	}
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
