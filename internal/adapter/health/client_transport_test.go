package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// funcName extracts the full symbol name of a function value for comparison.
func funcName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// newHealthTestLogger returns a quiet logger for transport tests.
func newHealthTestLogger(t *testing.T) logger.StyledLogger {
	t.Helper()
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, cleanup, _ := logger.New(loggerCfg)
	t.Cleanup(cleanup)
	return logger.NewPlainStyledLogger(log)
}

// extractTransport pulls the *http.Transport from the health checker built by
// NewHTTPHealthCheckerWithDefaults. The HTTPClient interface doesn't expose the
// transport, so we type-assert through the concrete *http.Client field.
func extractTransport(t *testing.T) *http.Transport {
	t.Helper()

	checker := NewHTTPHealthCheckerWithDefaults(newMockRepository(), newHealthTestLogger(t))

	// The client field is an HTTPClient interface; NewHTTPHealthCheckerWithDefaults
	// always passes a *http.Client so the assertion is safe in tests.
	httpClient, ok := checker.healthClient.client.(*http.Client)
	if !ok {
		t.Fatal("health client is not *http.Client; update this test if the type changes")
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("http.Client.Transport is not *http.Transport; update this test if the type changes")
	}

	return transport
}

// TestHealthClientTransport_NoProxyFromEnvironment asserts that the default health
// checker transport does NOT route through environment proxies. Health probes carry
// auth credentials; routing them via an operator-configured proxy risks leaking those
// credentials to a third party.
func TestHealthClientTransport_NoProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	transport := extractTransport(t)

	if transport.Proxy != nil {
		t.Errorf("health transport.Proxy = %s, want nil: credentials must not transit an env proxy",
			funcName(transport.Proxy))
	}
}

// TestHealthClientTransport_ResponseHeaderTimeout asserts that the default health
// checker transport has a finite ResponseHeaderTimeout. Without it a backend that
// accepts the TCP connection but withholds response headers blocks health probes
// indefinitely, masking downtime from the scheduler.
func TestHealthClientTransport_ResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	transport := extractTransport(t)

	if transport.ResponseHeaderTimeout <= 0 {
		t.Errorf("health transport.ResponseHeaderTimeout is %v; a backend that stalls after accept will block health probes indefinitely",
			transport.ResponseHeaderTimeout)
	}

	const want = DefaultHealthCheckerResponseHeaderTimeout
	if transport.ResponseHeaderTimeout != want {
		t.Errorf("transport.ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, want)
	}
}

// TestHealthClient_RefusesCrossHostRedirect proves a probe is never replayed at a
// host the operator did not configure: Go strips only Authorization and Cookie on a
// cross-host redirect, so an API-key header would otherwise reach the target.
func TestHealthClient_RefusesCrossHostRedirect(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	var leakedKey atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		leakedKey.Store(r.Header.Get("X-Api-Key"))
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(origin.Close)

	checker := NewHTTPHealthCheckerWithDefaults(newMockRepository(), newHealthTestLogger(t))
	ep := makeEndpoint(t, origin.URL)
	ep.AuthHeaderName = "X-Api-Key"
	ep.AuthHeaderValue = "sk-SECRET-123"

	result, _ := checker.Check(context.Background(), ep)

	if hits.Load() != 0 {
		t.Fatalf("redirect target was reached %d times with X-Api-Key=%v", hits.Load(), leakedKey.Load())
	}
	if result.Status == domain.StatusHealthy {
		t.Errorf("a redirect to another host must not count as healthy, got %v", result.Status)
	}
}
