package sherpa

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// staleKeepAliveBackend answers the first request on a connection and then
// reads the next one and closes without replying: a keep-alive idle timeout
// racing a POST.
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
						return
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

func newStaleTestService(t *testing.T, disc *testDiscoveryService, sel *testSelector) *Service {
	t.Helper()
	config := &Configuration{}
	config.ProxyPrefix = "/olla/"
	config.ConnectionTimeout = time.Second
	config.ConnectionKeepAlive = time.Second
	config.ResponseTimeout = 2 * time.Second
	config.ReadTimeout = 2 * time.Second
	config.StreamBufferSize = 8192
	svc, err := NewService(disc, sel, config, nil, nil, createRetryTestLogger())
	require.NoError(t, err)
	t.Cleanup(svc.Cleanup)
	return svc
}

func postVia(t *testing.T, svc *Service, body string, eps ...*domain.Endpoint) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/olla/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	err := svc.ProxyRequestToEndpointsWithRetry(req.Context(), rec, req, eps, &ports.RequestStats{StartTime: time.Now()}, createRetryTestLogger())
	return rec, err
}

// Idle keep-alive race: EOF on a reused pooled connection fails over but must
// not demote the healthy backend.
func TestSherpaEOFOnReusedConnection_FailsOverWithoutDemotion(t *testing.T) {
	stale := staleKeepAliveBackend(t)

	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(string(b))
		_, _ = w.Write([]byte("success"))
	}))
	defer srv.Close()
	su, _ := url.Parse(srv.URL)
	okEP := &domain.Endpoint{Name: "success", URL: su, URLString: srv.URL, Status: domain.StatusHealthy}

	disc := &testDiscoveryService{}
	sel := &testSelector{}
	svc := newStaleTestService(t, disc, sel)

	rec, err := postVia(t, svc, "warmup", stale)
	require.NoError(t, err)
	require.Equal(t, "ok", rec.Body.String())

	// testSelector indexes into the candidate list, so reset it for the second call.
	sel.selectIndex = 0
	rec, err = postVia(t, svc, `{"prompt":"hi"}`, stale, okEP)

	require.NoError(t, err)
	assert.Equal(t, "success", rec.Body.String())
	assert.Equal(t, `{"prompt":"hi"}`, got.Load(), "POST body must reach the second backend intact")
	assert.Empty(t, disc.updateStatusCalls, "reused-connection EOF must not demote a healthy endpoint")
}

func TestSherpaEOFOnReusedConnection_SingleEndpointStaysHealthy(t *testing.T) {
	stale := staleKeepAliveBackend(t)
	disc := &testDiscoveryService{}
	sel := &testSelector{}
	svc := newStaleTestService(t, disc, sel)

	_, err := postVia(t, svc, "warmup", stale)
	require.NoError(t, err)

	sel.selectIndex = 0
	_, err = postVia(t, svc, "second", stale)

	require.Error(t, err)
	assert.Empty(t, disc.updateStatusCalls)
	assert.Equal(t, domain.StatusHealthy, stale.Status)
}
