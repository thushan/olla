package factory

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackendTransport_NeverUsesEnvironmentProxy(t *testing.T) {
	t.Parallel()

	tr := NewBackendTransport(BackendClientOptions{MaxIdleConns: 3, IdleConnTimeout: time.Second})
	assert.Nil(t, tr.Proxy, "credentialled clients must not take HTTP(S)_PROXY from the environment")
	assert.Equal(t, 3, tr.MaxIdleConns)

	for name, client := range map[string]*http.Client{
		"backend client":  NewBackendHTTPClient(BackendClientOptions{}),
		"shared health":   NewSharedClientFactory().GetHealthClient(),
		"shared discover": NewSharedClientFactory().GetDiscoveryClient(),
	} {
		got, ok := client.Transport.(*http.Transport)
		require.Truef(t, ok, "%s: own transport required, not http.DefaultTransport", name)
		assert.Nilf(t, got.Proxy, "%s: Proxy must be nil", name)
		assert.NotNilf(t, client.CheckRedirect, "%s: redirect policy required", name)
	}
}

func TestRefuseCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		from   string
		to     string
		follow bool
	}{
		{"same origin", "http://backend:11434/api/tags", "http://backend:11434/api/tags/", true},
		{"different host", "http://backend:11434/x", "http://evil:11434/x", false},
		{"different port", "http://backend:11434/x", "http://backend:9999/x", false},
		{"http to https upgrade on the same host", "http://backend:11434/x", "https://backend:11434/x", true},
		{"explicit default ports upgrade", "http://backend:80/x", "https://backend:443/x", true},
		{"explicit port upgrades to the bare form", "http://backend:80/x", "https://backend/x", true},
		{"bare form upgrades to explicit default port", "http://backend/x", "https://backend:443/x", true},
		{"explicit default port is the same origin", "http://backend:80/x", "http://backend/x", true},
		{"host case does not matter", "http://Backend/x", "http://backend/x", true},
		{"mixed-case dotted host", "http://Host.Example/x", "http://host.example/x", true},
		{"unicode fold pair is a different host", "http://σ.example/x", "http://ς.example/x", false},
		{"kelvin sign is not ASCII k", "http://\u212a.example/x", "http://k.example/x", false},
		{"ipv6 zone case is significant", "http://[fe80::1%25Eth0]/x", "http://[fe80::1%25eth0]/x", false},
		{"ipv6 same zone and address case", "http://[FE80::1%25eth0]/x", "http://[fe80::1%25eth0]/x", true},
		{"default http port to a non-default https port", "http://backend/x", "https://backend:80/x", false},
		{"upgrade to a different port", "http://backend:11434/x", "https://backend:8443/x", false},
		{"upgrade to a different host", "http://backend/x", "https://evil/x", false},
		{"https to http downgrade", "https://backend/x", "http://backend/x", false},
		{"other scheme", "http://backend/x", "ftp://backend/x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig, err := http.NewRequest(http.MethodGet, tt.from, http.NoBody)
			require.NoError(t, err)
			next, err := http.NewRequest(http.MethodGet, tt.to, http.NoBody)
			require.NoError(t, err)

			err = RefuseCrossOriginRedirect(next, []*http.Request{orig})
			if tt.follow {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, http.ErrUseLastResponse)
			}
		})
	}

	t.Run("downgrade after an upgrade stays refused", func(t *testing.T) {
		t.Parallel()
		first, _ := http.NewRequest(http.MethodGet, "http://b/x", http.NoBody)
		upgraded, _ := http.NewRequest(http.MethodGet, "https://b/x", http.NoBody)
		back, _ := http.NewRequest(http.MethodGet, "http://b/x", http.NoBody)
		assert.ErrorIs(t, RefuseCrossOriginRedirect(back, []*http.Request{first, upgraded}), http.ErrUseLastResponse)
	})

	t.Run("hop limit", func(t *testing.T) {
		t.Parallel()
		orig, _ := http.NewRequest(http.MethodGet, "http://b/x", http.NoBody)
		via := make([]*http.Request, maxBackendRedirects)
		for i := range via {
			via[i] = orig
		}
		assert.ErrorIs(t, RefuseCrossOriginRedirect(orig, via), http.ErrUseLastResponse)
	})
}

// A real cross-host redirect must not replay API-key or custom headers.
func TestNewBackendHTTPClient_DoesNotReplayHeadersAcrossHosts(t *testing.T) {
	t.Parallel()

	leaked := make(chan http.Header, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked <- r.Header.Clone()
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.RedirectHandler(target.URL+"/api/tags", http.StatusFound))
	t.Cleanup(origin.Close)

	req, err := http.NewRequest(http.MethodGet, origin.URL, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-Api-Key", "sk-SECRET-123")

	resp, err := NewBackendHTTPClient(BackendClientOptions{Timeout: 5 * time.Second}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusFound, resp.StatusCode, "the 3xx is returned, not followed")
	select {
	case h := <-leaked:
		t.Fatalf("redirect target received a request carrying %q", h.Get("X-Api-Key"))
	default:
	}
}
