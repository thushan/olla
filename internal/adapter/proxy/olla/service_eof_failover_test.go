package olla

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// demotionRecorder is a discovery service that records which endpoints the
// retry handler demoted, so tests can assert the health state effect.
type demotionRecorder struct {
	mockDiscoveryService
	mu      sync.Mutex
	demoted []string
}

func (d *demotionRecorder) UpdateEndpointStatus(_ context.Context, endpoint *domain.Endpoint) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.demoted = append(d.demoted, endpoint.Name)
	return nil
}

func (d *demotionRecorder) names() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.demoted...)
}

// staleKeepAliveBackend answers the first request on a connection normally and
// then reads the next request on that same connection and closes without
// replying: exactly what a keep-alive idle timeout racing a POST looks like.
func staleKeepAliveBackend(t *testing.T) *domain.Endpoint {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				br := bufio.NewReader(conn)
				for i := 0; ; i++ {
					req, err := http.ReadRequest(br)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					if i > 0 {
						return // drop the reused connection without a response
					}
					_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
				}
			}()
		}
	}()

	u, err := url.Parse("http://" + ln.Addr().String())
	require.NoError(t, err)
	return &domain.Endpoint{Name: "stale", URL: u, URLString: u.String(), Status: domain.StatusHealthy}
}

func recordingBackend(t *testing.T, gotBody *atomic.Value) *domain.Endpoint {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		_, _ = w.Write([]byte("success"))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return &domain.Endpoint{Name: "success", URL: u, URLString: srv.URL, Status: domain.StatusHealthy}
}

func newEOFTestService(t *testing.T, disc *demotionRecorder) *Service {
	t.Helper()
	config := &Configuration{}
	config.StreamBufferSize = 8192
	svc, err := NewService(disc, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	require.NoError(t, err)
	t.Cleanup(svc.Cleanup)
	return svc
}

func postVia(t *testing.T, svc *Service, body string, eps ...*domain.Endpoint) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/olla/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	err := svc.ProxyRequestToEndpoints(req.Context(), rec, req, eps, &ports.RequestStats{StartTime: time.Now()}, createTestLogger())
	return rec, err
}

// Fresh-connection EOF: the backend accepted us and hung up before replying.
// It fails over and the endpoint is still demoted and charged to the breaker.
func TestOllaEOFOnFreshConnection_FailsOverAndDemotes(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer failing.Close()
	fu, _ := url.Parse(failing.URL)
	failEP := &domain.Endpoint{Name: "failing", URL: fu, URLString: failing.URL, Status: domain.StatusHealthy}

	var got atomic.Value
	okEP := recordingBackend(t, &got)
	disc := &demotionRecorder{}
	svc := newEOFTestService(t, disc)

	rec, err := postVia(t, svc, `{"prompt":"hi"}`, failEP, okEP)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
	assert.Equal(t, `{"prompt":"hi"}`, got.Load(), "POST body must reach the second backend intact")
	assert.Equal(t, []string{"failing"}, disc.names(), "fresh-connection EOF must demote the endpoint")
	assert.Equal(t, int64(1), svc.GetCircuitBreaker("failing").Failures())
}

// Idle keep-alive race: EOF on a reused pooled connection fails over but must
// leave the healthy backend's health and breaker untouched.
func TestOllaEOFOnReusedConnection_FailsOverWithoutDemotion(t *testing.T) {
	stale := staleKeepAliveBackend(t)
	var got atomic.Value
	okEP := recordingBackend(t, &got)
	disc := &demotionRecorder{}
	svc := newEOFTestService(t, disc)

	// Prime the pool with a connection to the stale backend.
	rec, err := postVia(t, svc, "warmup", stale)
	require.NoError(t, err)
	require.Equal(t, "ok", rec.Body.String())

	rec, err = postVia(t, svc, `{"prompt":"hi"}`, stale, okEP)

	require.NoError(t, err)
	assert.Equal(t, "success", rec.Body.String())
	assert.Equal(t, `{"prompt":"hi"}`, got.Load(), "POST body must reach the second backend intact")
	assert.Empty(t, disc.names(), "reused-connection EOF must not demote a healthy endpoint")
	assert.Zero(t, svc.GetCircuitBreaker("stale").Failures(), "reused-connection EOF must not count toward the breaker")
}

// With a single endpoint there is nowhere to fail over to, but the endpoint
// must still not be marked offline: that was the outage the race caused.
func TestOllaEOFOnReusedConnection_SingleEndpointStaysHealthy(t *testing.T) {
	stale := staleKeepAliveBackend(t)
	disc := &demotionRecorder{}
	svc := newEOFTestService(t, disc)

	_, err := postVia(t, svc, "warmup", stale)
	require.NoError(t, err)

	_, err = postVia(t, svc, "second", stale)

	require.Error(t, err)
	assert.Empty(t, disc.names())
	assert.Equal(t, domain.StatusHealthy, stale.Status)
	assert.Zero(t, svc.GetCircuitBreaker("stale").Failures())
}
