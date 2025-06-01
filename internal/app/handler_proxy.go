package app

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/util"
	"net/http"
	"time"
)

// proxyHandler handles Ollama API proxy requests
func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	requestID := util.GenerateRequestID()
	requestStartTime := time.Now()

	ctx := context.WithValue(r.Context(), constants.RequestIDKey, requestID)
	ctx = context.WithValue(ctx, constants.RequestTimeKey, requestStartTime)
	r = r.WithContext(ctx)

	rl := a.Config.Server.RateLimits
	clientIP := util.GetClientIP(r, rl.TrustProxyHeaders, rl.TrustedProxyCIDRsParsed)

	requestLogger := a.logger.WithRequestID(requestID)
	requestLogger.Info("Request started",
		"client_ip", clientIP,
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength)

	if stats, err := a.proxyService.ProxyRequest(r.Context(), w, r); err != nil {
		duration := time.Since(requestStartTime)

		// Don't use http.Error here as it might have already written to the response
		requestLogger.Error("Request failed", "error", err,
			"duration_ms", duration.Milliseconds(),
			"latency_ms", stats.Latency,
			"request_id", requestID,
			"endpoint", stats.EndpointName,
			"total_bytes", stats.TotalBytes,
			"request_processing_ms", stats.RequestProcessingMs,
			"backend_response_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"header_processing_ms", stats.HeaderProcessingMs,
			"selection_ms", stats.SelectionMs)

		// If headers haven't been written yet, return an error instead
		if w.Header().Get("Content-Type") == "" {
			http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		}
	} else {
		duration := time.Since(requestStartTime)
		requestLogger.Info("Request completed", "request_id", requestID,
			"endpoint", stats.EndpointName,
			"total_bytes", stats.TotalBytes,
			"duration_ms", duration.Milliseconds(),
			"latency_ms", stats.Latency,
			"request_processing_ms", stats.RequestProcessingMs,
			"backend_response_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"header_processing_ms", stats.HeaderProcessingMs,
			"selection_ms", stats.SelectionMs)

	}
}
