package factory

import (
	"net/http"
	"time"
)

const (
	HealthCheckTimeout = 5 * time.Second
	DiscoveryTimeout   = 30 * time.Second

	// maxBackendRedirects mirrors net/http's own limit for followed redirects.
	maxBackendRedirects = 10
)

// BackendClientOptions tunes the pool and timeouts of a credentialled backend
// client. Zero values leave the corresponding net/http default.
type BackendClientOptions struct {
	Timeout               time.Duration
	IdleConnTimeout       time.Duration
	ResponseHeaderTimeout time.Duration
	TLSHandshakeTimeout   time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

// NewBackendTransport builds the transport every client that carries endpoint
// credentials (health probes, model discovery) is built on. It has no
// Proxy func: http.DefaultTransport would hand Authorization and API-key headers
// to whatever HTTP(S)_PROXY the environment names, and Olla only ever talks to
// the backends the operator configured. The proxy engines build their own
// tuned transports and are deliberately not routed through here.
func NewBackendTransport(opts BackendClientOptions) *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		IdleConnTimeout:       opts.IdleConnTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		TLSHandshakeTimeout:   opts.TLSHandshakeTimeout,
	}
}

// NewBackendHTTPClient wraps NewBackendTransport with the redirect policy:
// same-origin redirects are followed, anything else is not. Go strips only
// Authorization and Cookie when a redirect crosses hosts, so API-key and
// custom headers would otherwise be replayed at a host the operator never
// configured.
func NewBackendHTTPClient(opts BackendClientOptions) *http.Client {
	return &http.Client{
		Timeout:       opts.Timeout,
		Transport:     NewBackendTransport(opts),
		CheckRedirect: RefuseCrossOriginRedirect,
	}
}

// RefuseCrossOriginRedirect is an http.Client.CheckRedirect that follows a
// redirect only when it keeps the host:port of the original request and the
// scheme, with one exception: http may upgrade to https on the same host:port,
// which a backend behind a TLS-terminating front end does routinely and which
// can only move credentials onto a safer channel. Any other scheme, host or
// port change, and any downgrade from a hop that was already https, returns the
// 3xx response unfollowed so the caller sees a non-success status.
func RefuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxBackendRedirects {
		return http.ErrUseLastResponse
	}
	origin := via[0].URL
	if req.URL.Host != origin.Host {
		return http.ErrUseLastResponse
	}
	if req.URL.Scheme == origin.Scheme {
		// an upgrade earlier in the chain must not be undone by a later hop
		if prev := via[len(via)-1].URL; prev.Scheme == "https" && req.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		return nil
	}
	if origin.Scheme == "http" && req.URL.Scheme == "https" {
		return nil
	}
	return http.ErrUseLastResponse
}

type SharedClientFactory struct {
	healthClient    *http.Client
	discoveryClient *http.Client
}

func NewSharedClientFactory() *SharedClientFactory {
	// One transport (and so one pool) serves both clients; they differ only in
	// their overall timeout.
	sharedTransport := NewBackendTransport(BackendClientOptions{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	})

	return &SharedClientFactory{
		healthClient: &http.Client{
			Timeout:       HealthCheckTimeout,
			Transport:     sharedTransport,
			CheckRedirect: RefuseCrossOriginRedirect,
		},
		discoveryClient: &http.Client{
			Timeout:       DiscoveryTimeout,
			Transport:     sharedTransport,
			CheckRedirect: RefuseCrossOriginRedirect,
		},
	}
}

func (f *SharedClientFactory) GetHealthClient() *http.Client {
	return f.healthClient
}

func (f *SharedClientFactory) GetDiscoveryClient() *http.Client {
	return f.discoveryClient
}
