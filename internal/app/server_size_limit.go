package app

import (
	"fmt"
	"net/http"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
)

type RequestSizeLimiter struct {
	maxBodySize   int64
	maxHeaderSize int64
	logger        *logger.StyledLogger
}

func NewRequestSizeLimiter(limits config.ServerRequestLimits, logger *logger.StyledLogger) *RequestSizeLimiter {
	return &RequestSizeLimiter{
		maxBodySize:   limits.MaxBodySize,
		maxHeaderSize: limits.MaxHeaderSize,
		logger:        logger,
	}
}

func (rsl *RequestSizeLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check header size first (cheaper than full body checks like the TSA :D)
		if err := rsl.validateHeaderSize(r); err != nil {
			rsl.logger.Warn("Request rejected: header size exceeded",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr)

			http.Error(w, "Request headers too large", http.StatusRequestHeaderFieldsTooLarge)
			return
		}

		// Wrap body with size-limited reader
		if err := rsl.validateAndLimitBody(r); err != nil {
			rsl.logger.Warn("Request rejected: body size exceeded",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr)

			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rsl *RequestSizeLimiter) validateHeaderSize(r *http.Request) error {
	if rsl.maxHeaderSize <= 0 {
		return nil
	}

	var totalSize int64

	// Calculate all header sizes
	for name, values := range r.Header {
		totalSize += int64(len(name))
		for _, value := range values {
			totalSize += int64(len(value))
		}
		// BUGFIX:
		// Account for ": " and "\r\n" per header line
		totalSize += int64(len(values) * 4)
	}

	// Add request line size (method + path + protocol)
	totalSize += int64(len(r.Method) + len(r.URL.RequestURI()) + len(r.Proto) + 4)

	if totalSize > rsl.maxHeaderSize {
		return fmt.Errorf("header size %d exceeds limit %d", totalSize, rsl.maxHeaderSize)
	}

	return nil
}

func (rsl *RequestSizeLimiter) validateAndLimitBody(r *http.Request) error {
	if rsl.maxBodySize <= 0 {
		return nil
	}

	// Check Content-Length header first (if present)
	if r.ContentLength > rsl.maxBodySize {
		return fmt.Errorf("content-length %d exceeds limit %d", r.ContentLength, rsl.maxBodySize)
	}

	// Wrap body with limited reader to catch cases where Content-Length is wrong/missing
	r.Body = http.MaxBytesReader(nil, r.Body, rsl.maxBodySize)

	return nil
}
