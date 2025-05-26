package app

import (
	"fmt"
	"net/http"
)

// proxyHandler handles Ollama API proxy requests
func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("Proxy request started",
		// "request_id", requestID,
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		// "client_ip", clientIP,
		// "user_agent", userAgent,
		"content_length", r.ContentLength,
		"content_type", r.Header.Get("Content-Type"))

	if err := a.proxyService.ProxyRequest(r.Context(), w, r); err != nil {
		// Don't use http.Error here as it might have already written to the response
		a.logger.Error("Proxy error", "error", err)
		// If headers haven't been written yet, return an error instead
		if w.Header().Get("Content-Type") == "" {
			http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		}
	}
}
