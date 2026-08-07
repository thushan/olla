package sherpa

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
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
		nil, // discovery service, not needed for transport tests
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

// TestSherpaTransport_NoProxyFromEnvironment asserts that the Sherpa proxy
// transport does NOT honour HTTP_PROXY/HTTPS_PROXY. Olla targets local
// inference backends; routing credentialled requests through an outbound proxy
// on plain HTTP is a credential-exposure risk. Health probes keep the env proxy.
func TestSherpaTransport_NoProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	svc := newSherpaServiceForTransportTest(t)

	if svc.transport.Proxy != nil {
		got := funcName(svc.transport.Proxy)
		t.Errorf("Sherpa transport.Proxy = %s, want nil: proxy requests must not be routed through env proxy", got)
	}
}

// TestSherpaTransport_ResponseHeaderTimeout asserts that the Sherpa transport
// has a finite ResponseHeaderTimeout. Without it, a backend that accepts the
// TCP connection but withholds response headers blocks the goroutine indefinitely.
func TestSherpaTransport_ResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	svc := newSherpaServiceForTransportTest(t)

	if svc.transport.ResponseHeaderTimeout <= 0 {
		t.Errorf("transport.ResponseHeaderTimeout is %v; backends that stall after accept will hang indefinitely",
			svc.transport.ResponseHeaderTimeout)
	}

	const want = DefaultResponseHeaderTimeout
	if svc.transport.ResponseHeaderTimeout != want {
		t.Errorf("transport.ResponseHeaderTimeout = %v, want %v", svc.transport.ResponseHeaderTimeout, want)
	}
}

// TestUpdateConfig_ZeroValueSherpaConfigResolvesSherpaDefaults pins the raw-copy
// pattern in UpdateConfig for *sherpa.Configuration inputs: an unset (zero-value)
// field must stay zero on the raw struct rather than getting the getter's
// resolved default baked in, so a later reload that legitimately clears the
// field back to "unset" is not permanently stuck on today's default. The
// getters still resolve to the correct Sherpa/base defaults either way.
func TestUpdateConfig_ZeroValueSherpaConfigResolvesSherpaDefaults(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	svc.configuration = &Configuration{}
	svc.UpdateConfig(&Configuration{})

	got := svc.configuration
	if got.StreamBufferSize != 0 {
		t.Errorf("StreamBufferSize = %d, want 0 (raw field must stay unset)", got.StreamBufferSize)
	}
	if got.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 (raw field must stay unset)", got.ReadTimeout)
	}
	if got.GetStreamBufferSize() != proxyconfig.DefaultStreamBufferSize {
		t.Errorf("GetStreamBufferSize() = %d, want %d", got.GetStreamBufferSize(), proxyconfig.DefaultStreamBufferSize)
	}
	if got.GetReadTimeout() != proxyconfig.DefaultReadTimeout {
		t.Errorf("GetReadTimeout() = %v, want %v", got.GetReadTimeout(), proxyconfig.DefaultReadTimeout)
	}
}
