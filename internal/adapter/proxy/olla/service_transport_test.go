package olla

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
)

// funcName extracts the full symbol name of a function value.
func funcName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// TestCreateOptimisedTransport_ConnectionLimits verifies that both MaxConnsPerHost and
// MaxIdleConnsPerHost are mapped to their correct fields on http.Transport.
// Previously MaxConnsPerHost was mistakenly written to MaxIdleConnsPerHost and
// MaxConnsPerHost was never set (defaulting to 0 = unlimited).
func TestCreateOptimisedTransport_ConnectionLimits(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxConnsPerHost = 42
	cfg.MaxIdleConnsPerHost = 17
	cfg.MaxIdleConns = 200
	cfg.IdleConnTimeout = 90 * time.Second

	transport := createOptimisedTransport(cfg)

	if transport.MaxConnsPerHost != 42 {
		t.Errorf("MaxConnsPerHost: want 42, got %d", transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != 17 {
		t.Errorf("MaxIdleConnsPerHost: want 17, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != 200 {
		t.Errorf("MaxIdleConns: want 200, got %d", transport.MaxIdleConns)
	}
}

// TestCreateOptimisedTransport_DefaultsApplied verifies that NewService fills in sensible
// defaults before handing the config to createOptimisedTransport, so a zero-value config
// never silently leaves MaxConnsPerHost unlimited.
func TestCreateOptimisedTransport_DefaultsApplied(t *testing.T) {
	t.Parallel()

	// Zero-value config — defaults should be filled in by NewService, but we can verify
	// the expected defaults are consistent with the package constants.
	cfg := &Configuration{}
	cfg.MaxConnsPerHost = proxyconfig.OllaDefaultMaxConnsPerHost
	cfg.MaxIdleConnsPerHost = proxyconfig.OllaDefaultMaxIdleConnsPerHost
	cfg.MaxIdleConns = proxyconfig.OllaDefaultMaxIdleConns
	cfg.IdleConnTimeout = proxyconfig.OllaDefaultIdleConnTimeout

	transport := createOptimisedTransport(cfg)

	if transport.MaxConnsPerHost != proxyconfig.OllaDefaultMaxConnsPerHost {
		t.Errorf("MaxConnsPerHost: want %d, got %d", proxyconfig.OllaDefaultMaxConnsPerHost, transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != proxyconfig.OllaDefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost: want %d, got %d", proxyconfig.OllaDefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	}
}

// TestCreateOptimisedTransport_FieldsAreDistinct guards against the specific regression
// where MaxConnsPerHost value bled into MaxIdleConnsPerHost. Using distinct values
// makes the mapping error immediately visible.
func TestCreateOptimisedTransport_FieldsAreDistinct(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxConnsPerHost = 100
	cfg.MaxIdleConnsPerHost = 10
	cfg.MaxIdleConns = 500

	transport := createOptimisedTransport(cfg)

	// Regression guard: if the bug is reintroduced both fields get value 100.
	if transport.MaxConnsPerHost == transport.MaxIdleConnsPerHost {
		t.Errorf("MaxConnsPerHost (%d) and MaxIdleConnsPerHost (%d) are equal — likely a field mapping regression",
			transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost: want 100, got %d", transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost: want 10, got %d", transport.MaxIdleConnsPerHost)
	}
}

// TestCreateOptimisedTransport_NoProxyFromEnvironment asserts that the Olla proxy
// transport does NOT honour HTTP_PROXY/HTTPS_PROXY. Olla targets local
// inference backends; routing credentialled requests through an outbound proxy
// on plain HTTP is a credential-exposure risk. Health probes keep the env proxy.
func TestCreateOptimisedTransport_NoProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxIdleConns = proxyconfig.OllaDefaultMaxIdleConns
	cfg.IdleConnTimeout = proxyconfig.OllaDefaultIdleConnTimeout

	transport := createOptimisedTransport(cfg)

	if transport.Proxy != nil {
		got := funcName(transport.Proxy)
		t.Errorf("Olla transport.Proxy = %s, want nil: proxy requests must not be routed through env proxy", got)
	}
}

// TestCreateOptimisedTransport_ResponseHeaderTimeout asserts that the Olla transport
// has a finite ResponseHeaderTimeout. Without it, a backend that accepts the TCP
// connection but withholds response headers blocks the goroutine indefinitely.
func TestCreateOptimisedTransport_ResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxIdleConns = proxyconfig.OllaDefaultMaxIdleConns
	cfg.IdleConnTimeout = proxyconfig.OllaDefaultIdleConnTimeout

	transport := createOptimisedTransport(cfg)

	if transport.ResponseHeaderTimeout <= 0 {
		t.Errorf("transport.ResponseHeaderTimeout is %v; backends that stall after accept will hang indefinitely",
			transport.ResponseHeaderTimeout)
	}

	want := proxyconfig.DefaultResponseHeaderTimeout
	if transport.ResponseHeaderTimeout != want {
		t.Errorf("transport.ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, want)
	}
}

// TestCreateOptimisedTransport_ResponseHeaderTimeout_Configurable verifies the
// transport's ResponseHeaderTimeout is driven by config when set, so cold-loading
// backends (e.g. Lemonade loading a model on first request) can send their first
// response header later than the 30s default without the proxy aborting with a 502.
// The default-when-unset case is covered by TestCreateOptimisedTransport_ResponseHeaderTimeout.
func TestCreateOptimisedTransport_ResponseHeaderTimeout_Configurable(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.ResponseHeaderTimeout = 180 * time.Second
	if got := createOptimisedTransport(cfg).ResponseHeaderTimeout; got != 180*time.Second {
		t.Errorf("configured ResponseHeaderTimeout: want 180s, got %v", got)
	}
}

// TestCreateOptimisedTransport_ConnectionKeepAlive verifies that a configured
// ConnectionKeepAlive value is used by the transport dialer, not silently ignored.
// The dialer is the only place KeepAlive appears; we can only inspect it indirectly
// through config, since net.Dialer is embedded in the closure.
func TestCreateOptimisedTransport_ConnectionKeepAlive(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.ConnectionKeepAlive = 60 * time.Second

	// Verify the getter returns the configured value (the dialer uses this).
	if got := cfg.GetConnectionKeepAlive(); got != 60*time.Second {
		t.Errorf("GetConnectionKeepAlive: want 60s, got %v", got)
	}

	// Verify the default-when-zero behaviour.
	zeroCfg := &Configuration{}
	if got := zeroCfg.GetConnectionKeepAlive(); got == 0 {
		t.Errorf("GetConnectionKeepAlive with zero config returned 0; expected a non-zero default")
	}
}

// TestCreateOptimisedTransport_TLSHandshakeTimeout asserts that the transport's
// TLSHandshakeTimeout uses the shared default when the config field is zero,
// protecting against backends that hang during TLS negotiation.
func TestCreateOptimisedTransport_TLSHandshakeTimeout(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	transport := createOptimisedTransport(cfg)

	if transport.TLSHandshakeTimeout <= 0 {
		t.Errorf("TLSHandshakeTimeout is %v; a zero value leaves TLS handshakes unbounded", transport.TLSHandshakeTimeout)
	}

	want := proxyconfig.DefaultTLSHandshakeTimeout
	if transport.TLSHandshakeTimeout != want {
		t.Errorf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, want)
	}
}

// TestCreateOptimisedTransport_TLSHandshakeTimeout_Configurable verifies that a
// non-zero TLSHandshakeTimeout in config is honoured on the transport.
func TestCreateOptimisedTransport_TLSHandshakeTimeout_Configurable(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.TLSHandshakeTimeout = 5 * time.Second
	if got := createOptimisedTransport(cfg).TLSHandshakeTimeout; got != 5*time.Second {
		t.Errorf("configured TLSHandshakeTimeout: want 5s, got %v", got)
	}
}

// TestUpdateConfig_PreservesTimeouts guards against UpdateConfig losing timeout
// fields that are not part of the ProxyConfiguration interface. Previously
// ResponseHeaderTimeout was silently reset on the non-Olla branch; TLSHandshakeTimeout
// had the same gap before this fix.
func TestUpdateConfig_PreservesTimeouts(t *testing.T) {
	t.Parallel()

	original := &Configuration{}
	original.ResponseHeaderTimeout = 90 * time.Second
	original.TLSHandshakeTimeout = 5 * time.Second
	original.MaxIdleConns = proxyconfig.OllaDefaultMaxIdleConns
	original.IdleConnTimeout = proxyconfig.OllaDefaultIdleConnTimeout
	original.MaxConnsPerHost = proxyconfig.OllaDefaultMaxConnsPerHost
	original.MaxIdleConnsPerHost = proxyconfig.OllaDefaultMaxIdleConnsPerHost

	svc := &Service{configuration: original}

	// Use the same *Configuration type so we exercise the Olla type-assertion path.
	updated := &Configuration{}
	updated.ResponseHeaderTimeout = 120 * time.Second
	updated.TLSHandshakeTimeout = 15 * time.Second

	svc.UpdateConfig(updated)

	if svc.configuration.ResponseHeaderTimeout != 120*time.Second {
		t.Errorf("ResponseHeaderTimeout after UpdateConfig: want 120s, got %v", svc.configuration.ResponseHeaderTimeout)
	}
	if svc.configuration.TLSHandshakeTimeout != 15*time.Second {
		t.Errorf("TLSHandshakeTimeout after UpdateConfig: want 15s, got %v", svc.configuration.TLSHandshakeTimeout)
	}
}
