package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
)

// A backend that dies after the response has started must be detectable by the
// client: the body read errors (connection aborted) and, where the body has an
// in-band error channel, a provider-format error event precedes the abort. A clean
// EOF is indistinguishable from a complete response, which is the defect under test.

const midStreamFailLog = "Backend failed after response started, aborting client connection"

// dyingBackend writes the given pre-death payload (flushing after each part) and
// then drops the connection without the chunked terminator.
func dyingBackend(t *testing.T, contentType string, contentLength int, parts []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", contentType)
		if contentLength > 0 {
			w.Header().Set("Content-Length", fmt.Sprint(contentLength))
		}
		w.WriteHeader(http.StatusOK)
		for _, p := range parts {
			_, _ = io.WriteString(w, p)
			w.(http.Flusher).Flush()
		}
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sseParts(n int) []string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"tok%d \"}}]}\n\n", i)
	}
	return parts
}

// frontServer serves the route's handler and records whether it ended in the
// net/http abort panic, re-panicking so the connection is really dropped.
type frontServer struct {
	*httptest.Server
	aborted atomic.Bool
}

func newFront(t *testing.T, h *statusHarness) *frontServer {
	t.Helper()
	f := &frontServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					f.aborted.Store(true)
				}
				panic(rec)
			}
		}()
		switch h.route.handler {
		case "anthropic":
			h.fleet.app.translationHandler(h.fleet.trans).ServeHTTP(w, r)
		case "proxy":
			h.fleet.app.proxyHandler(w, r.WithContext(context.WithValue(r.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/")))
		default:
			h.fleet.app.providerProxyHandler(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

type clientResult struct {
	status  int
	body    string
	readErr error
}

func doStreamRequest(t *testing.T, url, body string) clientResult {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got, readErr := io.ReadAll(resp.Body)
	return clientResult{status: resp.StatusCode, body: string(got), readErr: readErr}
}

const streamReqBody = `{"model":"mistral-small3.2","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`

func TestMidStreamBackendDeath_ClientCanDetectTruncation(t *testing.T) {
	t.Parallel()
	type scenario struct {
		name        string
		route       statusRoute
		contentType string
		length      int
		parts       []string
		// wantEvent must appear in the body; wantAbsent must not.
		wantEvent  []string
		wantAbsent []string
	}
	openaiErr := []string{`data: {"error":{"message":"upstream connection lost`, `"code":"upstream_stream_interrupted"`}
	anthropicErr := []string{"event: error\n", `"type":"error"`, `"type":"api_error"`}
	routes := statusRoutes()
	proxyRoute, providerRoute := routes[0], routes[1]
	ollamaNative := statusRoute{name: "ollama-native", handler: "provider", path: "/olla/ollama/api/chat"}
	jsonRoute := statusRoute{name: "provider-json", handler: "provider", path: "/olla/ollama/v1/chat/completions"}

	var scenarios []scenario
	for _, rt := range routes {
		s := scenario{name: "sse/" + rt.name, route: rt, contentType: "text/event-stream", parts: sseParts(5), wantAbsent: []string{"[DONE]", "message_stop", "message_delta"}}
		if rt.handler == "anthropic" {
			s.wantEvent = anthropicErr
		} else {
			s.wantEvent = openaiErr
		}
		scenarios = append(scenarios, s)
	}
	scenarios = append(scenarios,
		scenario{name: "ndjson/ollama-native", route: ollamaNative, contentType: "application/x-ndjson",
			parts:     []string{`{"model":"m","message":{"content":"a"},"done":false}` + "\n", `{"model":"m","message":{"content":"b"},"done":false}` + "\n"},
			wantEvent: []string{`{"error":"upstream connection lost during streaming response"}`}, wantAbsent: []string{`"done":true`}},
		scenario{name: "ndjson/proxy", route: proxyRoute, contentType: "application/x-ndjson",
			parts:     []string{`{"response":"a","done":false}` + "\n"},
			wantEvent: []string{`{"error":"upstream connection lost during streaming response"}`}},
		// Plain JSON has no in-band error channel: the abort alone must signal it.
		scenario{name: "json-chunked/provider", route: jsonRoute, contentType: "application/json",
			parts: []string{`{"choices":[{"message":{"content":"par`, `tial`}, wantAbsent: []string{"upstream connection lost", "[DONE]"}},
		scenario{name: "json-content-length/provider", route: jsonRoute, contentType: "application/json", length: 200,
			parts: []string{`{"choices":[{"message":{"content":"par`}, wantAbsent: []string{"upstream connection lost", "[DONE]"}},
		scenario{name: "json-chunked/proxy", route: proxyRoute, contentType: "application/json",
			parts: []string{`{"partial":`}, wantAbsent: []string{"upstream connection lost"}},
	)
	_ = providerRoute

	for _, engine := range []string{"olla", "sherpa"} {
		for _, sc := range scenarios {
			t.Run(engine+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				backend := dyingBackend(t, sc.contentType, sc.length, sc.parts)
				h := newStatusHarness(t, engine, sc.route, backend.URL)
				front := newFront(t, h)

				res := doStreamRequest(t, front.URL+sc.route.path, streamReqBody)
				t.Logf("status=%d readErr=%v tail=%q", res.status, res.readErr, tail(res.body, 160))

				require.Error(t, res.readErr, "client saw a clean EOF after the backend died mid-stream")
				for _, want := range sc.wantEvent {
					assert.Contains(t, res.body, want)
				}
				for _, bad := range sc.wantAbsent {
					assert.NotContains(t, res.body, bad)
				}
				assert.True(t, front.aborted.Load(), "handler must end with http.ErrAbortHandler")
				assert.NotEmpty(t, h.logs.lines(t, midStreamFailLog), "failure must be logged once the abort is raised")
			})
		}
	}
}

// A client that goes away is not a backend failure: no error event, no abort and
// no mid-stream failure log.
func TestMidStreamClientCancel_NoEventNoAbort(t *testing.T) {
	t.Parallel()
	for _, rt := range statusRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			t.Parallel()
			release := make(chan struct{})
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, sseParts(1)[0])
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(backend.Close)
			defer close(release)

			h := newStatusHarness(t, "olla", rt, backend.URL)
			front := newFront(t, h)

			ctx, cancel := context.WithCancel(context.Background())
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, front.URL+rt.path, strings.NewReader(streamReqBody))
			require.NoError(t, err)
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			buf := make([]byte, 1)
			_, err = resp.Body.Read(buf)
			require.NoError(t, err)
			cancel()
			resp.Body.Close()

			// The handler finishes once the cancelled request unwinds.
			require.Eventually(t, func() bool {
				return len(h.logs.lines(t, "Request failed")) > 0 || len(h.logs.lines(t, "Request completed")) > 0
			}, 5*time.Second, 20*time.Millisecond)
			assert.False(t, front.aborted.Load(), "client cancel must not abort")
			assert.Empty(t, h.logs.lines(t, midStreamFailLog), "client cancel is not a backend failure")
		})
	}
}

// Before any byte is committed the old behaviour stands: a JSON 502, no abort.
func TestPreCommitBackendFailure_Still502(t *testing.T) {
	t.Parallel()
	for _, rt := range statusRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			t.Parallel()
			dead := httptest.NewServer(http.NotFoundHandler())
			url := dead.URL
			dead.Close() // connection refused

			h := newStatusHarness(t, "olla", rt, url)
			front := newFront(t, h)
			res := doStreamRequest(t, front.URL+rt.path, streamReqBody)

			assert.Equal(t, http.StatusBadGateway, res.status)
			assert.NoError(t, res.readErr)
			assert.Contains(t, res.body, "error")
			assert.False(t, front.aborted.Load(), "nothing was committed, so nothing to abort")
			assert.Empty(t, h.logs.lines(t, midStreamFailLog))
		})
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
