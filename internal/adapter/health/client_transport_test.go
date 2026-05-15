package health

import (
	"net/http"
	"reflect"
	"runtime"
	"testing"

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
		t.Fatal("health client is not *http.Client — test needs updating")
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("http.Client.Transport is not *http.Transport — test needs updating")
	}

	return transport
}

// TestHealthClientTransport_ProxyFromEnvironment asserts that the default health
// checker transport honours HTTPS_PROXY/HTTP_PROXY/NO_PROXY so probes reach
// backends that sit behind a corporate network proxy.
func TestHealthClientTransport_ProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	transport := extractTransport(t)

	if transport.Proxy == nil {
		t.Fatal("health transport.Proxy is nil — HTTPS_PROXY/HTTP_PROXY will be silently ignored")
	}

	got := funcName(transport.Proxy)
	want := funcName(http.ProxyFromEnvironment)
	if got != want {
		t.Errorf("transport.Proxy = %s, want %s (http.ProxyFromEnvironment)", got, want)
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
		t.Errorf("health transport.ResponseHeaderTimeout is %v — a backend that stalls after accept will block health probes indefinitely",
			transport.ResponseHeaderTimeout)
	}

	const want = DefaultHealthCheckerResponseHeaderTimeout
	if transport.ResponseHeaderTimeout != want {
		t.Errorf("transport.ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, want)
	}
}
