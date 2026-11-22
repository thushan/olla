package olla

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// stallingBackend sends headers plus the given number of chunks, then goes
// silent with the connection open.
func stallingBackend(t *testing.T, chunks int) *domain.Endpoint {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for range chunks {
			_, _ = w.Write([]byte("data: {\"x\":1}\n\n"))
			w.(http.Flusher).Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return endpointFor(t, srv.URL)
}

// streamResponse only checked its read deadline between reads, so a backend that
// went silent inside Body.Read blocked the request forever.
func TestStalledMidStream_TimesOutWithinReadTimeout(t *testing.T) {
	for name, chunks := range map[string]int{"before-any-chunk": 0, "after-chunks": 2} {
		t.Run(name, func(t *testing.T) {
			const readTimeout = 300 * time.Millisecond
			ep := stallingBackend(t, chunks)
			svc := newCancelTestService(t, func(c *Configuration) { c.ReadTimeout = readTimeout })

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			rec := httptest.NewRecorder()
			ctx := context.Background()
			done := make(chan error, 1)
			go func() {
				done <- svc.ProxyRequestToEndpoints(ctx, rec, req, []*domain.Endpoint{ep}, &ports.RequestStats{StartTime: time.Now()}, createTestLogger())
			}()

			select {
			case err := <-done:
				require.Error(t, err)
				assert.Contains(t, err.Error(), "stopped responding")
				assert.False(t, core.IsClientAbort(ctx, err), "a backend stall is not a client abort")
				assert.Positive(t, svc.GetProxyStats().FailedRequests, "stall must be recorded as a failed request")
			case <-time.After(5 * readTimeout):
				t.Fatalf("proxy still blocked after read timeout %v: stalled backend never timed out", readTimeout)
			}
		})
	}
}
