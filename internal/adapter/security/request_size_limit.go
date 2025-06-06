package security

import (
	"context"
	"fmt"
	"net/http"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultProtocol = "HTTP/1.1"
)

type SizeValidator struct {
	logger        *logger.StyledLogger
	maxBodySize   int64
	maxHeaderSize int64
}

/*
				Olla Security Adapter - Size Limit Validator
	SizeValidator enforces request size limits for headers and body content.
 	It checks these limits early in the middleware chain to avoid wasting resources
 	on oversized requests.

	Thread-safe by design as it maintains no internal mutable state.
*/

func NewSizeValidator(limits config.ServerRequestLimits, logger *logger.StyledLogger) *SizeValidator {
	return &SizeValidator{
		maxBodySize:   limits.MaxBodySize,
		maxHeaderSize: limits.MaxHeaderSize,
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
				sv.logger.Warn("Request rejected",
					"reason", result.Reason,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr)

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

/***
 * faster by 10-15% with reduced allocations on Go 1.21+
func estimateHeaderSizeFast(headers http.Header, method, uri, proto string) int64 {
	totalSize := int64(len(method) + len(uri) + len(proto) + 4)

	for name, values := range headers {
		totalSize += int64(len(name))
		for i := 0; i < len(values); i++ {
			totalSize += int64(len(values[i]) + 4)
		}
	}

	return totalSize
}
****/
