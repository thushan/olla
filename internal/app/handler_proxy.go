package app

import (
	"fmt"
	"github.com/thushan/olla/internal/util"
	"net/http"
	"time"
)

// proxyHandler handles Ollama API proxy requests
func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	requestID := util.GenerateRequestID()
	requestStartTime := time.Now()

	requestLogger := a.logger.WithRequestID(requestID)
	requestLogger.Info("Request started",
		"client_ip", util.GetClientIP(r),
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength)

	if totalBytes, err := a.proxyService.ProxyRequest(r.Context(), w, r); err != nil {
		duration := time.Since(requestStartTime)

		// Don't use http.Error here as it might have already written to the response
		requestLogger.Error("Request failed", "error", err,
			"duration_ms", duration.Milliseconds(),
			"total_bytes", totalBytes)

		// If headers haven't been written yet, return an error instead
		if w.Header().Get("Content-Type") == "" {
			http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		}
	} else {
		duration := time.Since(requestStartTime)
		requestLogger.Info("Request completed",
			"duration_ms", duration.Milliseconds(), "total_bytes", totalBytes)
	}
}
