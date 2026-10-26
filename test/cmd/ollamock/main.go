// ollamock is a multi-protocol mock LLM backend for end-to-end validation of
// Olla's proxy and routing logic without requiring real inference infrastructure.
//
// It speaks Ollama, LM Studio, OpenAI-compatible, Lemonade, and Anthropic
// wire formats and supports controllable failure injection via a control plane.
//
// Usage:
//
//	go run ./test/cmd/ollamock \
//	    --addr 127.0.0.1:19431 \
//	    --name mock-a \
//	    --models llama3.2,phi4 \
//	    --ttft-ms 50 \
//	    --tps 20 \
//	    --stream-chunks 5
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19431", "listen address")
	name := flag.String("name", "mock-a", "instance marker embedded in every response")
	models := flag.String("models", "test-model", "comma-separated list of model names to serve")
	ttftMS := flag.Int("ttft-ms", 0, "delay in milliseconds before first streamed byte")
	tps := flag.Int("tps", 0, "tokens per second pacing (0 = instant)")
	streamChunks := flag.Int("stream-chunks", 5, "number of content chunks per streamed response")
	teiKind := flag.String("tei-kind", teiKindEmbedding, "kind of model a TEI instance reports in /info and serves: embedding, reranker or classifier")
	flag.Parse()

	if !slices.Contains([]string{teiKindEmbedding, teiKindReranker, teiKindClassifier}, *teiKind) {
		slog.Error("invalid --tei-kind", "value", *teiKind, "want", "embedding, reranker or classifier")
		os.Exit(2)
	}

	modelList := parseModels(*models)

	cfg := serverConfig{
		name:         *name,
		models:       modelList,
		ttftMS:       *ttftMS,
		tps:          *tps,
		streamChunks: *streamChunks,
		teiKind:      *teiKind,
	}

	srv := newServer(cfg)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("ollamock started",
		"addr", *addr,
		"name", *name,
		"models", modelList,
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("ollamock listen error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("ollamock shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("ollamock shutdown error", "err", err)
	}
}

func parseModels(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = []string{"test-model"}
	}
	return out
}
