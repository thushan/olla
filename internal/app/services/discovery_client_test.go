package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func discoveryTestLogger(t *testing.T) logger.StyledLogger {
	t.Helper()
	log, cleanup, err := logger.New(&logger.Config{Level: "error", Theme: "default"})
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return logger.NewPlainStyledLogger(log)
}

// dialTo sends every connection except the one to proxyAddr to realAddr, so a
// request addressed to a non-loopback host reaches a local test server. Go never
// consults the environment proxy for loopback targets, so the endpoint URL must be
// non-loopback for the proxy decision to be exercised at all.
func dialTo(realAddr, proxyAddr string) func(context.Context, string, string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == proxyAddr {
			return d.DialContext(ctx, network, addr)
		}
		return d.DialContext(ctx, network, realAddr)
	}
}

func ollamaEndpoint(t *testing.T, rawURL string) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{
		Name: "ollama", URL: u, URLString: u.String(), Type: domain.ProfileOllama,
		AuthHeaderName: "X-Api-Key", AuthHeaderValue: "sk-SECRET-123",
		Headers: map[string]string{"X-Tenant": "acme"},
	}
}

// Not parallel: it sets HTTP_PROXY for the process (Go reads it once, so the control uses an explicit proxy).
func TestDiscoveryClient_IgnoresEnvironmentProxy(t *testing.T) {
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxied.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	var direct atomic.Int32
	var gotKey atomic.Value
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		gotKey.Store(r.Header.Get("X-Api-Key"))
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3:8b","size":1}]}`))
	}))
	t.Cleanup(backend.Close)

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	proxyAddr := proxy.Listener.Addr().String()
	backendAddr := backend.Listener.Addr().String()
	// TEST-NET-1: not loopback, so ProxyFromEnvironment would pick the proxy
	const endpointURL = "http://192.0.2.10:11434"

	profiles, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	run := func(client *http.Client) {
		client.Transport.(*http.Transport).DialContext = dialTo(backendAddr, proxyAddr)
		dc := discovery.NewHTTPModelDiscoveryClient(profiles, discoveryTestLogger(t), client)
		_, _ = dc.DiscoverModels(context.Background(), ollamaEndpoint(t, endpointURL))
	}

	// control: a client configured with a proxy hands the credentials to it, which
	// proves this test can see the leak it guards against. The proxy is explicit
	// because ProxyFromEnvironment reads the environment once per process, so
	// under -count>1 or after another test it would ignore the HTTP_PROXY set here.
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	stockTransport := http.DefaultTransport.(*http.Transport).Clone()
	stockTransport.Proxy = http.ProxyURL(proxyURL)
	stock := &http.Client{Transport: stockTransport, Timeout: 5 * time.Second}
	run(stock)
	require.Positive(t, proxied.Load(), "control: default transport should have used the environment proxy")
	proxied.Store(0)
	direct.Store(0)

	guarded := newDiscoveryHTTPClient(5 * time.Second)
	// the environment is cached per process, so the behavioural check below can
	// pass for the wrong reason; the structural one cannot
	assert.Nil(t, guarded.Transport.(*http.Transport).Proxy, "discovery transport must not consult proxy settings")
	run(guarded)
	assert.Zero(t, proxied.Load(), "discovery must not send credentialled requests through HTTP_PROXY")
	assert.EqualValues(t, 1, direct.Load(), "discovery reaches the backend directly")
	assert.Equal(t, "sk-SECRET-123", gotKey.Load())
}

func TestDiscoveryClient_DoesNotReplayCredentialsOnCrossHostRedirect(t *testing.T) {
	t.Parallel()

	var leaked atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("X-Api-Key") + "|" + r.Header.Get("X-Tenant"))
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.RedirectHandler(target.URL+"/api/tags", http.StatusFound))
	t.Cleanup(origin.Close)

	profiles, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	dc := discovery.NewHTTPModelDiscoveryClient(profiles, discoveryTestLogger(t), newDiscoveryHTTPClient(5*time.Second))
	_, err = dc.DiscoverModels(context.Background(), ollamaEndpoint(t, origin.URL))

	require.Error(t, err, "a redirect is a discovery failure, never followed")
	assert.Nil(t, leaked.Load(), "the redirect target must not receive the request, let alone the API key or custom headers")
}
