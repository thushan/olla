// mockbackend is a minimal HTTP server for auth integration tests.
// It enforces a single required header and value, returning 401 when the
// credential is absent or wrong. All other paths return a minimal
// OpenAI-compatible JSON response so Olla's health checks and proxy pass
// through without requiring real model infrastructure.
//
// Usage:
//
//	go run ./test/cmd/mockbackend \
//	    --addr 127.0.0.1:19910 \
//	    --require-header Authorization \
//	    --require-value  "Bearer test-token-abc123"
//
// When --require-header is omitted the server accepts all requests (useful
// for the happy-path AIMock-equivalent flow).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19910", "listen address")
	requireHeader := flag.String("require-header", "", "header name that must be present")
	requireValue := flag.String("require-value", "", "exact header value required (ignored when require-header is empty)")
	flag.Parse()

	mux := http.NewServeMux()

	// Unauthenticated liveness probe so test scripts can poll until the
	// server is accepting connections without needing a valid credential.
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// Health + model listing share the same auth check so Olla's
	// health probes exercise the credential path.
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !authorised(w, r, *requireHeader, *requireValue) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "mock-model", "object": "model", "created": time.Now().Unix()},
			},
		})
	})

	// Chat completions: the route Olla proxies inference requests through.
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if !authorised(w, r, *requireHeader, *requireValue) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "mock-cmpl-001",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "mock-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "mock-backend: auth accepted",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
		})
	})

	slog.Info("mockbackend listening",
		"addr", *addr,
		"require_header", *requireHeader,
	)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "mockbackend: %v\n", err)
		os.Exit(1)
	}
}

// authorised checks the required credential and writes a 401 response when it
// is absent or wrong. Returns true when the request may proceed.
func authorised(w http.ResponseWriter, r *http.Request, header, value string) bool {
	if header == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get(header))
	if got == value {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="mockbackend"`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": fmt.Sprintf("missing or invalid %s header", header),
			"type":    "authentication_error",
			"code":    "invalid_api_key",
		},
	})
	return false
}
