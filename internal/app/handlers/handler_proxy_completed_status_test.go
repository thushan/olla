package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

// The per-request "Request completed" line is the operator's record of a proxied request,
// so it must say what status the client got and a backend 5xx must not read as a clean
// completion. These tests drive real engines over every route kind and read the log back.

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

type logLine struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	// kept loose: slog encodes ints as JSON numbers, and "status" is a string on completed lines
	HTTPStatus *float64 `json:"http_status"`
	Status     any      `json:"status"`
}

func (b *lockedBuffer) lines(t *testing.T, msg string) []logLine {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []logLine
	for _, raw := range strings.Split(b.buf.String(), "\n") {
		if raw == "" {
			continue
		}
		var l logLine
		require.NoError(t, json.Unmarshal([]byte(raw), &l), raw)
		if l.Msg == msg {
			out = append(out, l)
		}
	}
	return out
}

type statusRoute struct {
	name    string
	handler string // proxy, provider, anthropic
	path    string
	// passthrough only matters for the anthropic route
	passthrough bool
}

func statusRoutes() []statusRoute {
	return []statusRoute{
		{name: "proxy", handler: "proxy", path: "/olla/proxy/v1/chat/completions"},
		{name: "provider", handler: "provider", path: "/olla/ollama/v1/chat/completions"},
		{name: "anthropic-passthrough", handler: "anthropic", path: "/olla/anthropic/v1/messages", passthrough: true},
		{name: "anthropic-translation", handler: "anthropic", path: "/olla/anthropic/v1/messages"},
	}
}

type statusHarness struct {
	fleet *tagFleet
	logs  *lockedBuffer
	route statusRoute
}

func newStatusHarness(t *testing.T, engine string, route statusRoute, backendURL string) *statusHarness {
	t.Helper()

	fleet := newTagFleet(t, tagStrategies[0].cfg, map[string]string{"ollama-1": backendURL}, false, route.passthrough)

	logs := &lockedBuffer{}
	log := logger.NewStyledLogger(slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), &theme.Theme{}, false)
	collector := stats.NewCollector(log)
	selector, err := balancer.NewFactory(collector).Create(balancer.DefaultBalancerPriority)
	require.NoError(t, err)

	fleet.app.logger = log
	fleet.app.rejectLog = newRejectLogs(log, nil)
	fleet.app.statsCollector = collector
	fleet.app.discoveryService = &mockDiscoveryServiceWithHealthy{endpoints: fleet.endpoints[:1]}

	switch engine {
	case "sherpa":
		cfg := &sherpa.Configuration{}
		cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
		svc, serr := sherpa.NewService(fleet.app.discoveryService, selector, cfg, collector, nil, log)
		require.NoError(t, serr)
		fleet.app.proxyService = svc
	default:
		cfg := &olla.Configuration{}
		cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
		svc, serr := olla.NewService(fleet.app.discoveryService, selector, cfg, collector, nil, log)
		require.NoError(t, serr)
		t.Cleanup(svc.Cleanup)
		fleet.app.proxyService = svc
	}
	return &statusHarness{fleet: fleet, logs: logs, route: route}
}

func (h *statusHarness) post(model string, stream bool) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`, model, stream)
	req := httptest.NewRequest(http.MethodPost, h.route.path, strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()

	switch h.route.handler {
	case "anthropic":
		h.fleet.app.translationHandler(h.fleet.trans).ServeHTTP(w, req)
	case "proxy":
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
		h.fleet.app.proxyHandler(w, req)
	default:
		h.fleet.app.providerProxyHandler(w, req)
	}
	return w
}

// statusBackend answers every request with the given status. When stream is set it writes
// an event-stream in two flushed chunks, so the status is committed at the first write.
func statusBackend(t *testing.T, status int, stream bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stream {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.WriteHeader(status)
			for _, chunk := range []string{
				`data: {"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}` + "\n\n",
				"data: [DONE]\n\n",
			} {
				_, _ = w.Write([]byte(chunk))
				w.(http.Flusher).Flush()
			}
			return
		}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRequestCompleted_CarriesBackendStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		status    int
		stream    bool
		wantLevel string
	}{
		{"200 json", http.StatusOK, false, "INFO"},
		{"200 streaming", http.StatusOK, true, "INFO"},
		{"backend 429 is the caller's problem", http.StatusTooManyRequests, false, "INFO"},
		{"backend 502", http.StatusBadGateway, false, "WARN"},
		{"backend 503", http.StatusServiceUnavailable, false, "WARN"},
		{"backend 502 streaming", http.StatusBadGateway, true, "WARN"},
	}

	for _, engine := range []string{"sherpa", "olla"} {
		for _, route := range statusRoutes() {
			for _, tc := range cases {
				t.Run(engine+"/"+route.name+"/"+tc.name, func(t *testing.T) {
					t.Parallel()

					backend := statusBackend(t, tc.status, tc.stream)
					h := newStatusHarness(t, engine, route, backend.URL)

					w := h.post(tagBareChat, tc.stream)

					completed := h.logs.lines(t, "Request completed")
					require.Len(t, completed, 1, "one completion line per request, response %d: %s", w.Code, w.Body.String())
					got := completed[0]
					assert.Equal(t, tc.wantLevel, got.Level)
					require.NotNil(t, got.HTTPStatus, "completion line must carry the status")
					assert.EqualValues(t, w.Code, *got.HTTPStatus, "logged status must be the one the client received")
					if tc.wantLevel == "WARN" {
						assert.GreaterOrEqual(t, w.Code, 500)
					}
					assert.Empty(t, h.logs.lines(t, "Request rejected"), "a proxied request is not a rejection")
				})
			}
		}
	}
}

// Olla's own refusals stay on #237's sampled path: a client repeating an unknown model gets
// one "Request rejected" WARN, no completion line, and no unthrottled INFO "Request received".
func TestRequestRejected_NotDuplicatedByCompletion(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{"sherpa", "olla"} {
		for _, route := range statusRoutes() {
			t.Run(engine+"/"+route.name, func(t *testing.T) {
				t.Parallel()

				backend := statusBackend(t, http.StatusOK, false)
				h := newStatusHarness(t, engine, route, backend.URL)

				for range 5 {
					w := h.post("nonexistent-xyz", false)
					require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
				}

				assert.Empty(t, h.logs.lines(t, "Request completed"))
				rejected := h.logs.lines(t, "Request rejected")
				require.Len(t, rejected, 1, "repeats are sampled, not one line each")
				assert.Equal(t, "WARN", rejected[0].Level)
				for _, l := range h.logs.lines(t, "Request received") {
					assert.NotEqual(t, "INFO", l.Level, "a rejected request must not leave a per-request INFO line")
				}
			})
		}
	}
}

// errorCapturingLogger records the fields of Error calls.
type errorCapturingLogger struct {
	mockStyledLogger
	errFields []any
}

func (e *errorCapturingLogger) Error(_ string, args ...any) { e.errFields = args }

// A failure after the response was committed must say what the client got.
func TestLogRequestResult_FailureCarriesHTTPStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		status int
		want   bool
	}{{http.StatusOK, true}, {0, false}} {
		el := &errorCapturingLogger{}
		cl := &capturingLogger{}
		pr := makeTestPR(cl)
		pr.requestLogger = el
		pr.upstreamStatus = tc.status

		(&Application{logger: cl}).logRequestResult(pr, fmt.Errorf("stream broke"))

		has := false
		for i := 0; i+1 < len(el.errFields); i += 2 {
			if el.errFields[i] == "http_status" {
				has = true
				assert.Equal(t, tc.status, el.errFields[i+1])
			}
		}
		assert.Equal(t, tc.want, has, "status %d fields %v", tc.status, el.errFields)
	}
}

// A backend that refuses the connection fails before anything is committed; the error
// response Olla writes is the status the client sees, and the ERROR line must carry it.
func TestRequestFailed_CarriesWrittenErrorStatus(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{"sherpa", "olla"} {
		for _, route := range statusRoutes() {
			t.Run(engine+"/"+route.name, func(t *testing.T) {
				t.Parallel()

				// Port 1 is never listening; a closed httptest port can be reissued to a
				// parallel test's server and answer instead.
				h := newStatusHarness(t, engine, route, "http://127.0.0.1:1")

				w := h.post(tagBareChat, false)

				failed := h.logs.lines(t, "Request failed")
				require.Len(t, failed, 1, "response %d: %s", w.Code, w.Body.String())
				assert.Equal(t, "ERROR", failed[0].Level)
				require.NotNil(t, failed[0].HTTPStatus, "failure line must carry the status written to the client")
				assert.EqualValues(t, w.Code, *failed[0].HTTPStatus)
				assert.Equal(t, http.StatusBadGateway, w.Code)
			})
		}
	}
}

// The counters behind /internal/status, /internal/status/endpoints, Prometheus and
// /internal/stats/translators must agree with the WARN rule above: a backend 5xx is a
// failure, a 4xx (429 included) is not, and one client request is counted once.
func TestStatsCounting_FollowsBackendStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		status      int
		stream      bool
		wantFailure bool
	}{
		{"200 json", http.StatusOK, false, false},
		{"200 streaming", http.StatusOK, true, false},
		{"backend 429", http.StatusTooManyRequests, false, false},
		{"backend 503", http.StatusServiceUnavailable, false, true},
		{"backend 503 streaming", http.StatusServiceUnavailable, true, true},
	}

	for _, engine := range []string{"sherpa", "olla"} {
		for _, route := range statusRoutes() {
			for _, tc := range cases {
				t.Run(engine+"/"+route.name+"/"+tc.name, func(t *testing.T) {
					t.Parallel()

					backend := statusBackend(t, tc.status, tc.stream)
					h := newStatusHarness(t, engine, route, backend.URL)

					w := h.post(tagBareChat, tc.stream)
					require.Equal(t, tc.status, w.Code, "the client gets the backend status: %s", w.Body.String())

					wantFailed, wantOK := int64(0), int64(1)
					if tc.wantFailure {
						wantFailed, wantOK = 1, 0
					}
					collector := h.fleet.app.statsCollector

					ps := collector.GetProxyStats()
					assert.Equal(t, int64(1), ps.TotalRequests, "proxy total")
					assert.Equal(t, wantOK, ps.SuccessfulRequests, "proxy successes")
					assert.Equal(t, wantFailed, ps.FailedRequests, "proxy failures")

					es, ok := collector.GetEndpointStats()[backend.URL]
					require.True(t, ok, "endpoint stats for %s", backend.URL)
					assert.Equal(t, int64(1), es.TotalRequests, "endpoint total")
					assert.Equal(t, wantFailed, es.FailedRequests, "endpoint failures")

					ts := collector.GetTranslatorStats()[h.fleet.trans.Name()]
					if route.handler != "anthropic" {
						assert.Zero(t, ts.TotalRequests, "non-translator routes leave translator stats alone")
						return
					}
					assert.Equal(t, int64(1), ts.TotalRequests, "translator total")
					assert.Equal(t, wantOK, ts.SuccessfulRequests, "translator successes")
					assert.Equal(t, wantFailed, ts.FailedRequests, "translator failures")
				})
			}
		}
	}
}
