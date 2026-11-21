package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A backend that sends headers (and maybe some chunks) then goes silent with the
// connection open must be cut at proxy.read_timeout and handled like a dead
// backend: provider-format error event, read error at the client, no [DONE].
// The Olla engine used to block in Body.Read forever.

// stallingBackend writes the parts, flushes, then holds the connection open
// silently until the test ends.
func stallingBackend(t *testing.T, parts []string) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for _, p := range parts {
			_, _ = io.WriteString(w, p)
			w.(http.Flusher).Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

func TestMidStreamBackendStall_ClientCanDetectTruncation(t *testing.T) {
	t.Parallel()
	const readTimeout = 300 * time.Millisecond
	openaiErr := []string{`data: {"error":{"message":"upstream connection lost`, `"code":"upstream_stream_interrupted"`}
	anthropicErr := []string{"event: error\n", `"type":"api_error"`}

	for _, engine := range []string{"olla", "sherpa"} {
		for _, rt := range statusRoutes() {
			for _, afterChunks := range []int{0, 3} {
				name := engine + "/" + rt.name
				if afterChunks == 0 {
					name += "/before-any-chunk"
				} else {
					name += "/after-chunks"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					backend := stallingBackend(t, sseParts(afterChunks))
					h := newStatusHarness(t, engine, rt, backend.URL, readTimeout)
					front := newFront(t, h)

					start := time.Now()
					res := doStreamRequest(t, front.URL+rt.path, streamReqBody)
					elapsed := time.Since(start)

					assert.Less(t, elapsed, 10*readTimeout, "stalled backend was not cut near read_timeout")
					if rt.name == "anthropic-translation" && afterChunks == 0 {
						// The translator has written nothing yet, so this is still a
						// pre-commit failure: a proper 502 JSON, not a stream event.
						assert.Equal(t, http.StatusBadGateway, res.status)
						assert.Contains(t, res.body, "stopped responding")
						assert.False(t, front.aborted.Load())
						return
					}
					require.Error(t, res.readErr, "client saw a clean EOF after the backend stalled")
					want := openaiErr
					if rt.handler == "anthropic" {
						want = anthropicErr
					}
					for _, w := range want {
						assert.Contains(t, res.body, w)
					}
					assert.NotContains(t, res.body, "[DONE]")
					assert.NotContains(t, res.body, "message_stop")
					if afterChunks > 0 {
						assert.Contains(t, res.body, "tok0")
					}
					assert.True(t, front.aborted.Load(), "handler must end with http.ErrAbortHandler")
					assert.NotEmpty(t, h.logs.lines(t, midStreamFailLog))
				})
			}
		}
	}
}

// A slow but steady stream, with every gap shorter than read_timeout, must not be cut
// even though its total duration is several times the timeout.
func TestSlowSteadyStream_NotCutByReadTimeout(t *testing.T) {
	t.Parallel()
	const readTimeout = 400 * time.Millisecond
	for _, engine := range []string{"olla", "sherpa"} {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				for _, p := range sseParts(8) {
					_, _ = io.WriteString(w, p)
					w.(http.Flusher).Flush()
					time.Sleep(readTimeout / 4)
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(backend.Close)
			rt := statusRoutes()[0]
			h := newStatusHarness(t, engine, rt, backend.URL, readTimeout)
			front := newFront(t, h)

			res := doStreamRequest(t, front.URL+rt.path, streamReqBody)

			require.NoError(t, res.readErr)
			assert.Contains(t, res.body, "tok7")
			assert.Contains(t, res.body, "[DONE]")
			assert.False(t, front.aborted.Load())
		})
	}
}
