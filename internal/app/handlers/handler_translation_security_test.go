package handlers

// Regression tests for Phase 3 security hardening items:
//   2. Upstream error-body read is capped at MaxUpstreamErrorBodyBytes.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// TestStreamingBackendError_ErrorBodyCapped verifies that handleStreamingBackendError
// does not deadlock when the upstream sends more than MaxUpstreamErrorBodyBytes.
// Without io.LimitReader the handler would read indefinitely; with it, reading stops
// at the cap and the pipeReader is closed to unblock the proxy goroutine.
func TestStreamingBackendError_ErrorBodyCapped(t *testing.T) {
	t.Parallel()

	proxyFunc := func(
		_ context.Context,
		w http.ResponseWriter,
		_ *http.Request,
		_ []*domain.Endpoint,
		_ *ports.RequestStats,
		_ logger.StyledLogger,
	) error {
		w.WriteHeader(http.StatusInternalServerError)
		// Write exactly the cap so we sit on the LimitReader boundary.
		// The write may be cut short when the reader closes the pipe; that is
		// intentional — the proxy error is discarded in the error path.
		buf := make([]byte, constants.MaxUpstreamErrorBodyBytes)
		copy(buf, `{"error":{"message":"big-error"}}`)
		_, _ = w.Write(buf)
		return nil
	}

	trans := &mockTranslator{
		name:                  "cap-test-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.WriteHeader(statusCode)
		},
		transformRequestFunc: func(_ context.Context, _ *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]any{
					"model":    "test-model",
					"stream":   true,
					"messages": []any{map[string]any{"role": "user", "content": "hi"}},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     &mockProxyService{proxyFunc: proxyFunc},
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	reqBody := `{"model":"test-model","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	// Run the handler in a goroutine so we can apply a local deadline. A deadlock
	// (missing LimitReader + CloseWithError) would hang here indefinitely.
	type result struct {
		code int
	}
	ch := make(chan result, 1)
	go func() {
		rec := httptest.NewRecorder()
		app.translationHandler(trans).ServeHTTP(rec, req)
		ch <- result{code: rec.Code}
	}()

	select {
	case r := <-ch:
		if r.code != http.StatusInternalServerError {
			t.Errorf("expected 500 from backend error path, got %d", r.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler deadlocked — LimitReader cap or CloseWithError may be missing")
	}
}
