package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
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
	app.handleProxyError(core.NewResponseStartedWriter(rr), wrapped)

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
	app.handleProxyError(core.NewResponseStartedWriter(rr), errors.New("dial tcp: connection refused"))

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for a non-size-limit error, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Proxy error:") {
		t.Errorf("expected the generic proxy error message, got %q", rr.Body.String())
	}
}

// TestHandleProxyError_HeaderSetButNotStarted_StillMapsError pins the fix for
// the campaign finding that the old guard checked Content-Type presence as a
// proxy for "response already committed". Header().Set never commits anything
// - only WriteHeader/Write do - so a handler that sets Content-Type ahead of
// time (a common pattern) then hits an error before writing anything used to
// have its error response silently swallowed by the old guard. The
// ResponseStartedWriter-based guard must still map the error correctly here.
func TestHandleProxyError_HeaderSetButNotStarted_StillMapsError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	tracker := core.NewResponseStartedWriter(rr)

	// Set Content-Type without writing a status or body: this is exactly the
	// state the old Content-Type-presence guard would have mistaken for "already
	// committed" and no-opped on.
	tracker.Header().Set("Content-Type", "application/json")

	var app *Application
	app.handleProxyError(tracker, errors.New("dial tcp: connection refused"))

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 to still be written despite Content-Type being pre-set, got %d (body: %q)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Proxy error:") {
		t.Errorf("expected the generic proxy error message, got %q", rr.Body.String())
	}
}

// TestHandleProxyError_ResponseAlreadyStarted_NoOps guards the one legitimate
// case the old Content-Type check was protecting against: once real bytes (or
// a status line) have reached the client, handleProxyError must not attempt a
// second write - appending an HTML/plain-text error onto a response the client
// has already started receiving corrupts it. The ResponseStartedWriter-based
// guard must still no-op here.
func TestHandleProxyError_ResponseAlreadyStarted_NoOps(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	tracker := core.NewResponseStartedWriter(rr)

	tracker.WriteHeader(http.StatusOK)
	_, _ = tracker.Write([]byte(`{"partial":`))

	var app *Application
	app.handleProxyError(tracker, errors.New("dial tcp: connection refused"))

	if rr.Code != http.StatusOK {
		t.Errorf("handleProxyError must not overwrite a status already sent to the client, got %d", rr.Code)
	}
	if rr.Body.String() != `{"partial":` {
		t.Errorf("handleProxyError must not append to a body already in flight, got %q", rr.Body.String())
	}
}

// mustParseURL parses s or fails the test - a valid httptest.Server URL is
// never expected to fail here, so this keeps the caller free of error noise.
func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", s, err)
	}
	return u
}

// singleEndpointSelector always returns the one endpoint it was built with.
// Minimal stand-in for domain.EndpointSelector - the real balancer package is
// out of scope for this test, which is proving the size-limit -> 413 mapping
// through a real proxy engine, not routing strategy.
type singleEndpointSelector struct {
	endpoint *domain.Endpoint
}

func (s *singleEndpointSelector) Select(_ context.Context, _ []*domain.Endpoint) (*domain.Endpoint, error) {
	return s.endpoint, nil
}
func (s *singleEndpointSelector) Name() string                            { return "single" }
func (s *singleEndpointSelector) IncrementConnections(_ *domain.Endpoint) {}
func (s *singleEndpointSelector) DecrementConnections(_ *domain.Endpoint) {}

// TestSizeValidator_ChunkedProxyBody_EndToEnd drives the real
// security.SizeValidator.CreateMiddleware (the proxy-route middleware,
// distinct from CreateNonProxyMiddleware) in front of the real Application.proxyHandler,
// backed by a real Sherpa proxy engine forwarding to a real httptest upstream.
// A fake inline handler previously stood in for the proxy engine here and never
// exercised retry.go's actual preserveRequestBody - the real code path that
// produces the wrapped *http.MaxBytesError in production - so this is the
// end-to-end shape the campaign's "chunked oversized body -> 502 not 413"
// finding described: oversized chunked bodies must now 413, and bodies under
// the cap must reach the real upstream and succeed normally.
func TestSizeValidator_ChunkedProxyBody_EndToEnd(t *testing.T) {
	t.Parallel()

	const cap = 100

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockLogger := logger.NewStyledLogger(testLogger, &theme.Theme{}, false)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	endpoint := &domain.Endpoint{
		Name:      "test-upstream",
		URL:       mustParseURL(t, upstream.URL),
		URLString: upstream.URL,
		Type:      "ollama",
		Status:    domain.StatusHealthy,
	}

	sherpaConfig := &sherpa.Configuration{}
	sherpaConfig.ResponseTimeout = 5 * time.Second
	sherpaConfig.ReadTimeout = 5 * time.Second
	sherpaConfig.StreamBufferSize = 8192

	collector := stats.NewCollector(mockLogger)
	selector := &singleEndpointSelector{endpoint: endpoint}
	discovery := &mockDiscoveryServiceWithFunc{
		getHealthyEndpointsFunc: func(_ context.Context) ([]*domain.Endpoint, error) {
			return []*domain.Endpoint{endpoint}, nil
		},
	}

	realProxy, err := sherpa.NewService(discovery, selector, sherpaConfig, collector, nil, mockLogger)
	if err != nil {
		t.Fatalf("failed to create Sherpa proxy: %v", err)
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     realProxy,
		discoveryService: discovery,
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		Config: &config.Config{
			Server: config.ServerConfig{
				RateLimits: config.ServerRateLimits{},
			},
		},
		StartTime: time.Now(),
	}

	validator := security.NewSizeValidator(config.ServerRequestLimits{
		MaxBodySize: cap,
	}, nil, mockLogger)

	handler := validator.CreateMiddleware()(http.HandlerFunc(app.proxyHandler))

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
		if !strings.Contains(rr.Body.String(), "Request body too large") {
			t.Errorf("expected the same 413 message as the pre-check path, got %q", rr.Body.String())
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
			t.Errorf("expected 200 for a chunked body under the cap, got %d (body: %q, real upstream reached: %v)", rr.Code, rr.Body.String(), strings.Contains(rr.Body.String(), "ok"))
		}
	})
}
