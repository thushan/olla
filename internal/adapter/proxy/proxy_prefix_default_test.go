package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Characterisation of the proxy Configuration's path-prefix default. An unset
// ProxyPrefix on the configuration production wires through the factory
// resolves to the literal "route_prefix" (see GetProxyPrefix), a sentinel no
// real request path starts with, so the default never strips anything: URL
// building only strips when ops configure a prefix explicitly. The default
// must stay that literal across the context-key rework. (The engines' own
// embedded BaseProxyConfig defaults an unset prefix to "/olla/" instead; the
// factory always sets the value explicitly, so that default is not reached
// through this path.)
func TestGetProxyPrefix_DefaultAndExplicit(t *testing.T) {
	t.Parallel()

	if got := (&Configuration{}).GetProxyPrefix(); got != "route_prefix" {
		t.Errorf("default proxy prefix = %q, want the literal \"route_prefix\"", got)
	}
	if got := (&Configuration{ProxyPrefix: "/olla/proxy/"}).GetProxyPrefix(); got != "/olla/proxy/" {
		t.Errorf("explicit proxy prefix = %q, want it returned unchanged", got)
	}
}

func TestBuildTargetURL_DefaultPrefixNeverStripsRealPaths(t *testing.T) {
	t.Parallel()

	endpoint := createTestEndpoint("test", "http://backend.test:8080", domain.StatusHealthy)

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	u := common.BuildTargetURL(req, endpoint, (&Configuration{}).GetProxyPrefix())
	if u.Path != "/v1/chat/completions" {
		t.Errorf("path = %q, want unchanged: the default prefix must not strip real request paths", u.Path)
	}

	prefixed := httptest.NewRequest("POST", "/olla/proxy/v1/chat/completions", nil)
	u = common.BuildTargetURL(prefixed, endpoint, "/olla/proxy/")
	if u.Path != "/v1/chat/completions" {
		t.Errorf("path = %q, want the explicit prefix stripped", u.Path)
	}
}

// End to end through the factory with an unset prefix: the sentinel default is
// inert, so the backend receives the request path exactly as the handler
// produced it. A path beginning /olla/ proves the effective prefix is the
// "route_prefix" sentinel rather than the engine config's own "/olla/" default.
func TestFactoryBuiltEngine_UnsetProxyPrefixLeavesPathUnchanged(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{DefaultProxySherpa, DefaultProxyOlla} {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			var receivedPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"id":"x","object":"chat.completion","choices":[]}`)
			}))
			defer upstream.Close()

			svc, err := NewFactory(createTestStatsCollector(), nil, createTestLogger()).
				Create(engine, &mockDiscoveryService{}, &mockEndpointSelector{}, &Configuration{})
			if err != nil {
				t.Fatalf("failed to create %s service: %v", engine, err)
			}

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			req := httptest.NewRequest("POST", "/olla/api/chat", nil)
			w := httptest.NewRecorder()
			stats := &ports.RequestStats{RequestID: "prefix-default", StartTime: time.Now()}

			if err := svc.ProxyRequestToEndpoints(t.Context(), w, req, []*domain.Endpoint{endpoint}, stats, createTestLogger()); err != nil {
				t.Fatalf("proxy request failed: %v", err)
			}
			if receivedPath != "/olla/api/chat" {
				t.Errorf("backend received path %q, want the request path unchanged", receivedPath)
			}
		})
	}
}
