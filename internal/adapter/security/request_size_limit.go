package security

/*
				Olla Security Adapter - Size Limit Validator
	SizeValidator enforces request size limits for headers and body content.
 	It checks these limits early in the middleware chain to avoid wasting resources
 	on oversized requests.

	Thread-safe by design as it maintains no internal mutable state.
*/

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

const (
	DefaultProtocol = "HTTP/1.1"
)

type SizeValidator struct {
	logger        logger.StyledLogger
	metrics       ports.SecurityMetricsService
	maxBodySize   int64
	maxHeaderSize int64
}

func NewSizeValidator(limits config.ServerRequestLimits, metrics ports.SecurityMetricsService, logger logger.StyledLogger) *SizeValidator {
	return &SizeValidator{
		maxBodySize:   limits.MaxBodySize,
		maxHeaderSize: limits.MaxHeaderSize,
		metrics:       metrics,
		logger:        logger,
	}
}

func (sv *SizeValidator) Name() string {
	return "size_limit"
}

// Validate checks the request against configured size constraints.
// Returns a SecurityResult indicating whether the request is allowed.
func (sv *SizeValidator) Validate(ctx context.Context, req ports.SecurityRequest) (ports.SecurityResult, error) {
	if err := sv.validateHeaderSize(req); err != nil {
		return ports.SecurityResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Request headers too large: %v", err),
		}, nil
	}

	if err := sv.validateBodySize(req); err != nil {
		return ports.SecurityResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Request body too large: %v", err),
		}, nil
	}

	return ports.SecurityResult{
		Allowed: true,
	}, nil
}

// validateHeaderSize estimates total header size, including field names and values.
// Returns an error if the combined size exceeds the configured max.
func (sv *SizeValidator) validateHeaderSize(req ports.SecurityRequest) error {
	if sv.maxHeaderSize <= 0 {
		return nil
	}

	totalSize := estimateHeaderSize(req.Headers, req.Method, req.Endpoint, DefaultProtocol) // assume HTTP/1.1
	if totalSize > sv.maxHeaderSize {
		return fmt.Errorf("header size %d exceeds limit %d", totalSize, sv.maxHeaderSize)
	}
	return nil
}

// validateBodySize checks the request body size against the configured limit.
func (sv *SizeValidator) validateBodySize(req ports.SecurityRequest) error {
	if sv.maxBodySize <= 0 {
		return nil
	}

	if req.BodySize > sv.maxBodySize {
		return fmt.Errorf("content-length %d exceeds limit %d", req.BodySize, sv.maxBodySize)
	}

	return nil
}

func (sv *SizeValidator) CreateMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := ports.SecurityRequest{
				Endpoint:   r.URL.Path,
				Method:     r.Method,
				BodySize:   r.ContentLength,
				HeaderSize: estimateHeaderSize(r.Header, r.Method, r.URL.RequestURI(), r.Proto),
				Headers:    r.Header,
			}

			result, err := sv.Validate(r.Context(), req)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if !result.Allowed {
				sv.recordViolation(r, req, result)

				if r.ContentLength > sv.maxBodySize {
					http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "Request headers too large", http.StatusRequestHeaderFieldsTooLarge)
				}
				return
			}

			if sv.maxBodySize > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, sv.maxBodySize)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CreateNonProxyMiddleware is like CreateMiddleware but also rejects chunked
// (Content-Length == -1) requests whose bodies exceed the configured limit.
// It is intended for non-proxy routes (status, health, stats) that never read
// the body themselves, so MaxBytesReader alone would never fire for them.
//
// The approach: drain up to maxBodySize+1 bytes through MaxBytesReader before
// calling next. If the read hits the limit the reader returns an error and we
// return 413. Otherwise we restore the buffered bytes as the request body so
// a handler that does read the body still receives the full content.
//
// Proxy routes must NOT use this middleware — they stream large bodies
// legitimately and must not be buffered. Use CreateMiddleware for those paths.
func (sv *SizeValidator) CreateNonProxyMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := ports.SecurityRequest{
				Endpoint:   r.URL.Path,
				Method:     r.Method,
				BodySize:   r.ContentLength,
				HeaderSize: estimateHeaderSize(r.Header, r.Method, r.URL.RequestURI(), r.Proto),
				Headers:    r.Header,
			}

			result, err := sv.Validate(r.Context(), req)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if !result.Allowed {
				sv.recordViolation(r, req, result)

				if r.ContentLength > sv.maxBodySize {
					http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "Request headers too large", http.StatusRequestHeaderFieldsTooLarge)
				}
				return
			}

			// For chunked requests (ContentLength == -1), drain the body up to the
			// cap now. Handlers on non-proxy routes never read the body themselves, so
			// MaxBytesReader installed on r.Body would silently never trigger; we must
			// enforce the limit here before passing control to the handler.
			if sv.maxBodySize > 0 && r.ContentLength < 0 && r.Body != nil {
				limited := http.MaxBytesReader(w, r.Body, sv.maxBodySize)
				buf, readErr := io.ReadAll(limited)
				if readErr != nil {
					// Treat chunked overflow identically to known-Content-Length overflow so
					// security metrics capture both paths. Report size as limit+1 — the actual
					// byte count is unknown for chunked transfers, but limit+1 conveys "exceeded".
					overflowReq := ports.SecurityRequest{
						Endpoint:   r.URL.Path,
						Method:     r.Method,
						BodySize:   sv.maxBodySize + 1,
						HeaderSize: req.HeaderSize,
						Headers:    r.Header,
					}
					overflowResult := ports.SecurityResult{
						Allowed: false,
						Reason:  fmt.Sprintf("Request body too large: chunked body exceeds limit %d", sv.maxBodySize),
					}
					sv.recordViolation(r, overflowReq, overflowResult)
					http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				// Restore the buffered body so downstream handlers that do read it
				// still get the content.
				r.Body = io.NopCloser(bytes.NewReader(buf))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// recordViolation emits a security violation metric. Factored out so both
// CreateMiddleware and CreateNonProxyMiddleware share the same reporting path
// without duplicating the metric-construction logic.
func (sv *SizeValidator) recordViolation(r *http.Request, req ports.SecurityRequest, result ports.SecurityResult) {
	if sv.metrics == nil {
		sv.logger.Warn("Request rejected",
			"reason", result.Reason,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr)
		return
	}

	violationType := constants.ViolationSizeLimit
	size := req.BodySize
	if strings.Contains(result.Reason, "headers too large") {
		size = req.HeaderSize
	}

	violation := ports.SecurityViolation{
		ClientID:      util.GetClientIP(r, false, nil),
		ViolationType: violationType,
		Endpoint:      r.URL.Path,
		Size:          size,
		Timestamp:     time.Now(),
	}
	_ = sv.metrics.RecordViolation(r.Context(), violation)

	sv.logger.Warn("Request rejected",
		"reason", result.Reason,
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr)
}

func estimateHeaderSize(headers http.Header, method, uri, proto string) int64 {
	var totalSize int64

	for name, values := range headers {
		totalSize += int64(len(name))
		for _, value := range values {
			totalSize += int64(len(value))
		}
		totalSize += int64(len(values) * 4) // header overhead
	}

	totalSize += int64(len(method) + len(uri) + len(proto) + 4) // request line

	return totalSize
}
