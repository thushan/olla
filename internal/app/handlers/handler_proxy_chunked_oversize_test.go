package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

// TestHandleProxyError_ChunkedOversizedBodyMapsTo413 pins the fix for the
// campaign finding: a chunked (no Content-Length) request body that exceeds
// the configured cap is caught mid-read by the http.MaxBytesReader installed
// on r.Body by security.SizeValidator.CreateMiddleware - not by the
// pre-check, which only fires for a known Content-Length. Before this fix,
// the resulting *http.MaxBytesError travelled up through retry.go's body
// preservation as a generic wrapped error and handleProxyError mapped every
// non-nil error to a 502 "Proxy error: ..." response. It must now be
// recognised via errors.As and answered with the same 413 the pre-check uses.
func TestHandleProxyError_ChunkedOversizedBodyMapsTo413(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()

	// Reproduce exactly what retry.go's preserveRequestBody does: read the
	// MaxBytesReader-wrapped body via io.ReadAll and wrap any error with
	// "failed to read request body: %w", so the test proves the fix against
	// the real wrapping chain, not an idealised one.
	limited := http.MaxBytesReader(rr, io.NopCloser(strings.NewReader(strings.Repeat("x", 20))), 10)
	_, readErr := io.ReadAll(limited)
	if readErr == nil {
		t.Fatal("expected http.MaxBytesReader to reject a body over its limit")
	}
	var mbErr *http.MaxBytesError
	if !errors.As(readErr, &mbErr) {
		t.Fatalf("expected io.ReadAll over a MaxBytesReader to yield *http.MaxBytesError, got %T: %v", readErr, readErr)
	}
	wrapped := fmt.Errorf("failed to read request body: %w", readErr)

	var app *Application
	app.handleProxyError(rr, wrapped)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d (body: %q)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Request body too large") {
		t.Errorf("expected the same 413 message as the pre-check path, got %q", rr.Body.String())
	}
}

// TestHandleProxyError_GenericErrorStays502 guards against over-broadening
// the 413 mapping: an ordinary proxy failure (e.g. upstream connection
// refused) must still surface as the existing 502 "Proxy error: ..." shape.
func TestHandleProxyError_GenericErrorStays502(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	var app *Application
	app.handleProxyError(rr, errors.New("dial tcp: connection refused"))

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for a non-size-limit error, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Proxy error:") {
		t.Errorf("expected the generic proxy error message, got %q", rr.Body.String())
	}
}

// TestSizeValidator_ChunkedProxyBody_EndToEnd drives the real
// security.SizeValidator.CreateMiddleware (the proxy-route middleware,
// distinct from CreateNonProxyMiddleware) followed by a handler that mimics
// the proxy engine's real behaviour: read the whole body via io.ReadAll (as
// retry.go's preserveRequestBody does), and on error route it through
// handleProxyError exactly as handler_proxy.go does. This is the end-to-end
// shape the campaign's "chunked oversized body -> 502 not 413" finding
// described - oversized chunked bodies must now 413, and bodies under the
// cap must reach the handler and succeed normally.
func TestSizeValidator_ChunkedProxyBody_EndToEnd(t *testing.T) {
	t.Parallel()

	const cap = 100

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockLogger := logger.NewStyledLogger(testLogger, &theme.Theme{}, false)

	validator := security.NewSizeValidator(config.ServerRequestLimits{
		MaxBodySize: cap,
	}, nil, mockLogger)

	var app *Application
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			app.handleProxyError(w, fmt.Errorf("failed to read request body: %w", err))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := validator.CreateMiddleware()(next)

	t.Run("chunked over cap returns 413", func(t *testing.T) {
		t.Parallel()

		body := strings.Repeat("x", cap+50)
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", strings.NewReader(body))
		req.ContentLength = -1 // chunked: no known Content-Length

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413 for an oversized chunked body, got %d (body: %q)", rr.Code, rr.Body.String())
		}
	})

	t.Run("chunked under cap returns 200", func(t *testing.T) {
		t.Parallel()

		body := strings.Repeat("x", cap-10)
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", strings.NewReader(body))
		req.ContentLength = -1

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for a chunked body under the cap, got %d (body: %q)", rr.Code, rr.Body.String())
		}
	})
}
