package sherpa

import (
	"net/http"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// funcName extracts the full symbol name of a function value for comparison.
// http.ProxyFromEnvironment is a named function so the pointer is stable across builds.
func funcName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// newSherpaServiceForTransportTest builds a real Sherpa service via NewService so
// the transport tests exercise the production construction path.
func newSherpaServiceForTransportTest(t *testing.T) *Service {
	t.Helper()

	cfg := &Configuration{}
	cfg.ConnectionTimeout = 2 * time.Second
	cfg.ConnectionKeepAlive = 30 * time.Second
	cfg.StreamBufferSize = 8192

	svc, err := NewService(
		nil, // discovery service — not needed for transport tests
		&mockEndpointSelector{},
		cfg,
		nil, // stats collector
		nil, // metrics extractor
		createTestLogger(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Cleanup)
	return svc
}

// TestSherpaTransport_ProxyFromEnvironment asserts that the Sherpa transport
// honours HTTPS_PROXY/HTTP_PROXY/NO_PROXY env vars. Without this, outbound
// connections silently bypass any corporate network proxy.
func TestSherpaTransport_ProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	svc := newSherpaServiceForTransportTest(t)

	if svc.transport.Proxy == nil {
		t.Fatal("Sherpa transport.Proxy is nil — HTTPS_PROXY/HTTP_PROXY will be silently ignored")
	}

	got := funcName(svc.transport.Proxy)
	want := funcName(http.ProxyFromEnvironment)
	if got != want {
		t.Errorf("transport.Proxy = %s, want %s (http.ProxyFromEnvironment)", got, want)
	}
}

// TestSherpaTransport_ResponseHeaderTimeout asserts that the Sherpa transport
// has a finite ResponseHeaderTimeout. Without it, a backend that accepts the
// TCP connection but withholds response headers blocks the goroutine indefinitely.
func TestSherpaTransport_ResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	svc := newSherpaServiceForTransportTest(t)

	if svc.transport.ResponseHeaderTimeout <= 0 {
		t.Errorf("transport.ResponseHeaderTimeout is %v — backends that stall after accept will hang indefinitely",
			svc.transport.ResponseHeaderTimeout)
	}

	const want = DefaultResponseHeaderTimeout
	if svc.transport.ResponseHeaderTimeout != want {
		t.Errorf("transport.ResponseHeaderTimeout = %v, want %v", svc.transport.ResponseHeaderTimeout, want)
	}
}
