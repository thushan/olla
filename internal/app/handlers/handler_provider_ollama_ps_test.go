package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/router"
)

func psBackend(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func psEndpoint(name, typ string, srv *httptest.Server, status domain.EndpointStatus) *domain.Endpoint {
	u, _ := url.Parse(srv.URL)
	return &domain.Endpoint{Name: name, Type: typ, URL: u, URLString: srv.URL, Status: status}
}

func runPS(t *testing.T, ctx context.Context, eps ...*domain.Endpoint) *httptest.ResponseRecorder {
	t.Helper()
	app := &Application{logger: &mockStyledLogger{}, repository: &mockRepository{endpoints: eps}}
	req := httptest.NewRequest(http.MethodGet, "/olla/ollama/api/ps", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	app.ollamaRunningModelsHandler(w, req)
	return w
}

type psResponse struct {
	Models []map[string]any `json:"models"`
}

func TestOllamaRunningModels_MergesPerInstance(t *testing.T) {
	t.Parallel()

	a := psBackend(t, 200, `{"models":[{"name":"llama3:8b","size_vram":10},{"name":"qwen3:8b"}]}`)
	b := psBackend(t, 200, `{"models":[{"name":"llama3:8b","size_vram":20}]}`)
	other := psBackend(t, 200, `{"models":[{"name":"not-ollama"}]}`)
	down := psBackend(t, 200, `{"models":[{"name":"unhealthy"}]}`)

	w := runPS(t, context.Background(),
		psEndpoint("a", "ollama", a, domain.StatusHealthy),
		psEndpoint("b", "ollama", b, domain.StatusHealthy),
		psEndpoint("v", "vllm", other, domain.StatusHealthy),
		psEndpoint("d", "ollama", down, domain.StatusUnhealthy),
	)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, w.Header().Get(headerOllaPartial))

	var got psResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Models, 3, "the same model on two hosts stays as two entries")
	assert.Equal(t, "a", got.Models[0][ollamaPSEndpointField])
	assert.Equal(t, "llama3:8b", got.Models[0]["name"])
	assert.EqualValues(t, 10, got.Models[0]["size_vram"], "fields pass through untouched")
	assert.Equal(t, "b", got.Models[2][ollamaPSEndpointField])
}

func TestOllamaRunningModels_SkipsFailingInstances(t *testing.T) {
	t.Parallel()

	good := psBackend(t, 200, `{"models":[{"name":"llama3:8b"}]}`)
	for name, bad := range map[string]*httptest.Server{
		"status":    psBackend(t, 500, `boom`),
		"malformed": psBackend(t, 200, `{"models":`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := runPS(t, context.Background(),
				psEndpoint("good", "ollama", good, domain.StatusHealthy),
				psEndpoint("bad", "ollama", bad, domain.StatusHealthy))

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "1 of 2 endpoints failed", w.Header().Get(headerOllaPartial))
			var got psResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Len(t, got.Models, 1)
			assert.Equal(t, "good", got.Models[0][ollamaPSEndpointField])
		})
	}
}

func TestOllamaRunningModels_TimeoutDoesNotHoldTheAnswer(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); hang.Close() })
	good := psBackend(t, 200, `{"models":[{"name":"llama3:8b"}]}`)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	w := runPS(t, ctx,
		psEndpoint("good", "ollama", good, domain.StatusHealthy),
		psEndpoint("hang", "ollama", hang, domain.StatusHealthy))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "1 of 2 endpoints failed", w.Header().Get(headerOllaPartial))
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestOllamaRunningModels_AllFailingIs502_NoInstancesIsEmpty(t *testing.T) {
	t.Parallel()

	bad := psBackend(t, 500, `boom`)
	w := runPS(t, context.Background(), psEndpoint("bad", "ollama", bad, domain.StatusHealthy))
	assert.Equal(t, http.StatusBadGateway, w.Code)

	w = runPS(t, context.Background(), psEndpoint("v", "vllm", bad, domain.StatusHealthy))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"models":[]}`, w.Body.String())
}

func TestOllamaRunningModels_SendsEndpointAuthAndRefusesRedirects(t *testing.T) {
	t.Parallel()

	var gotAuth string
	authed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(authed.Close)
	ep := psEndpoint("authed", "ollama", authed, domain.StatusHealthy)
	ep.AuthHeaderName, ep.AuthHeaderValue = "Authorization", "Bearer tok"
	require.Equal(t, http.StatusOK, runPS(t, context.Background(), ep).Code)
	assert.Equal(t, "Bearer tok", gotAuth)

	target := psBackend(t, 200, `{"models":[{"name":"elsewhere"}]}`)
	redirector := httptest.NewServer(http.RedirectHandler(target.URL+"/api/ps", http.StatusFound))
	t.Cleanup(redirector.Close)
	w := runPS(t, context.Background(), psEndpoint("r", "ollama", redirector, domain.StatusHealthy))
	assert.Equal(t, http.StatusBadGateway, w.Code, "a redirect is a failure, never followed")
}

func TestOllamaRunningModels_RejectsOtherMethods(t *testing.T) {
	t.Parallel()
	app := &Application{logger: &mockStyledLogger{}, repository: &mockRepository{}}
	w := httptest.NewRecorder()
	app.ollamaRunningModelsHandler(w, httptest.NewRequest(http.MethodPost, "/olla/ollama/api/ps", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func ollamaRouteApp(t *testing.T) (*router.RouteRegistry, *http.ServeMux) {
	t.Helper()
	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)
	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:           &config.Config{},
		logger:           &mockStyledLogger{},
		routeRegistry:    reg,
		profileFactory:   factory,
		repository:       &mockRepository{},
		converterFactory: converter.NewConverterFactory(),
	}
	app.registerProviderRoutes()
	mux := http.NewServeMux()
	reg.WireUp(mux)
	return reg, mux
}

// /api/ps is aggregated; /api/list is not an Ollama endpoint and is no longer
// claimed, so it falls through to the provider catch-all (and on to Ollama's own 404).
func TestRegisterProviderRoutes_OllamaPSAggregatedAndNoAPIList(t *testing.T) {
	t.Parallel()

	reg, mux := ollamaRouteApp(t)
	routes := reg.GetRoutes()
	assert.Contains(t, routes, "/olla/ollama/api/ps")
	assert.NotContains(t, routes, "/olla/ollama/api/list")

	_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/olla/ollama/api/list", nil))
	assert.Equal(t, "/olla/ollama/", pattern, "api/list must reach the catch-all, not a stub")
}

// Model management is refused for every method and labelled ANY, blob upload
// (the first step of api/create) included.
func TestRegisterProviderRoutes_OllamaModelManagementIs501AndLabelledANY(t *testing.T) {
	t.Parallel()

	reg, mux := ollamaRouteApp(t)
	for _, path := range []string{"api/pull", "api/push", "api/create", "api/copy", "api/delete", "api/blobs/"} {
		route := "/olla/ollama/" + path
		require.Contains(t, reg.GetRoutes(), route)
		assert.Equal(t, routeMethodAny, reg.GetRoutes()[route].Method, route)
	}
	assert.Equal(t, routeMethodAny, reg.GetRoutes()["/olla/ollama/api/show"].Method)

	for _, path := range []string{
		"/olla/ollama/api/pull", "/olla/ollama/api/push", "/olla/ollama/api/create",
		"/olla/ollama/api/copy", "/olla/ollama/api/delete",
		"/olla/ollama/api/blobs/sha256:0123abcd",
	} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead} {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			assert.Equal(t, http.StatusNotImplemented, rec.Code, "%s %s", method, path)
		}
	}
}

// Both the profile-driven table and the static fallback (used when profiles
// cannot be loaded) must agree: api/ps in, api/list out, blobs refused.
func TestStaticProviders_OllamaRoutes(t *testing.T) {
	t.Parallel()

	var paths []string
	methods := map[string]string{}
	for _, r := range getStaticProviders(&Application{})[constants.ProviderTypeOllama].routes {
		paths = append(paths, r.path)
		methods[r.path] = r.method
	}
	assert.Contains(t, paths, "api/ps")
	assert.Contains(t, paths, "api/blobs/")
	assert.NotContains(t, paths, "api/list")
	for _, p := range []string{"api/pull", "api/push", "api/create", "api/copy", "api/delete", "api/blobs/", "api/show"} {
		assert.Equal(t, routeMethodAny, methods[p], p)
	}
}

// The merged body must be exactly what Ollama sent plus the tag: nothing dropped,
// nothing reshaped, including fields this code knows nothing about.
func TestOllamaRunningModels_ShapeMatchesOllamaPlusTag(t *testing.T) {
	t.Parallel()

	const entry = `{"name":"llama3:8b","model":"llama3:8b","size":5137025024,"digest":"abc",` +
		`"details":{"parent_model":"","format":"gguf","family":"llama","families":["llama"],` +
		`"parameter_size":"8.0B","quantization_level":"Q4_0"},"expires_at":"2026-10-27T10:00:00Z",` +
		`"size_vram":5137025024,"context_length":4096,"future_field":[1,{"x":null}]}`
	a := psBackend(t, 200, `{"models":[`+entry+`]}`)

	w := runPS(t, context.Background(), psEndpoint("a", "ollama", a, domain.StatusHealthy))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var got struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Models, 1)
	assert.JSONEq(t, `"a"`, string(got.Models[0][ollamaPSEndpointField]))
	delete(got.Models[0], ollamaPSEndpointField)
	rest, err := json.Marshal(got.Models[0])
	require.NoError(t, err)
	assert.JSONEq(t, entry, string(rest))

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &top))
	assert.Len(t, top, 1, "envelope is just {models}")
}

func TestOllamaRunningModels_EmptyResultsStayAnEmptyArray(t *testing.T) {
	t.Parallel()

	a := psBackend(t, 200, `{"models":[]}`)
	b := psBackend(t, 200, `{}`)
	w := runPS(t, context.Background(),
		psEndpoint("a", "ollama", a, domain.StatusHealthy),
		psEndpoint("b", "ollama", b, domain.StatusHealthy))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"models":[]}`, w.Body.String(), "[] not null, or Ollama clients fail to range it")
	assert.Empty(t, w.Header().Get(headerOllaPartial))
}

// Credentials belong to one endpoint: a second endpoint must never see them.
func TestOllamaRunningModels_CredentialsStayPerEndpoint(t *testing.T) {
	t.Parallel()

	seen := func(dst *atomic.Value) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dst.Store(r.Header.Get("X-Api-Key") + "|" + r.Header.Get("X-Custom"))
			_, _ = w.Write([]byte(`{"models":[]}`))
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	var gotA, gotB atomic.Value
	gotA.Store("unset")
	gotB.Store("unset")
	epA := psEndpoint("a", "ollama", seen(&gotA), domain.StatusHealthy)
	epA.AuthHeaderName, epA.AuthHeaderValue = "X-Api-Key", "key-a"
	epA.Headers = map[string]string{"X-Custom": "only-a"}
	epB := psEndpoint("b", "ollama", seen(&gotB), domain.StatusHealthy)

	require.Equal(t, http.StatusOK, runPS(t, context.Background(), epA, epB).Code)
	assert.Equal(t, "key-a|only-a", gotA.Load())
	assert.Equal(t, "|", gotB.Load(), "endpoint b must not receive a's credentials")
}

// Not parallel: it counts process-wide goroutines. A slow, a failing and a
// healthy instance exercise every exit path of the fan-out; none may leave a
// goroutine (or an unclosed response body, which pins a transport read loop) behind.
func TestOllamaRunningModels_NoGoroutineLeak(t *testing.T) {
	good := psBackend(t, 200, `{"models":[{"name":"m"}]}`)
	bad := psBackend(t, 500, strings.Repeat("x", 1<<16))
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(release) })

	ollamaPSClient.CloseIdleConnections()
	before := runtime.NumGoroutine()
	// The handler's own context cancel reclaims connections on return, which would
	// mask an unclosed body, so the error path is driven directly with a context
	// that never ends.
	for range 5 {
		_, err := fetchOllamaPS(context.Background(), psEndpoint("b", "ollama", bad, domain.StatusHealthy))
		require.ErrorIs(t, err, errOllamaPSStatus)
	}
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		runPS(t, ctx,
			psEndpoint("g", "ollama", good, domain.StatusHealthy),
			psEndpoint("s", "ollama", slow, domain.StatusHealthy))
		cancel()
	}
	ollamaPSClient.CloseIdleConnections()
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+2 },
		5*time.Second, 25*time.Millisecond, "goroutines grew from %d to %d", before, runtime.NumGoroutine())
}

// The endpoint's credentials travel on these requests, so the shared client must
// not take http.DefaultTransport's environment proxy, and must be one long-lived
// pooled client rather than something rebuilt per request.
func TestOllamaPSClient_PooledAndNeverProxied(t *testing.T) {
	transport, ok := ollamaPSClient.Transport.(*http.Transport)
	require.True(t, ok, "ollamaPSClient needs its own transport, not http.DefaultTransport")
	assert.Nil(t, transport.Proxy, "credentials must not be routed through an environment proxy")
	assert.Positive(t, transport.MaxIdleConnsPerHost)
	assert.Positive(t, transport.IdleConnTimeout)

	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	ep := &domain.Endpoint{Name: "a", URLString: srv.URL}
	for range 3 {
		_, err := fetchOllamaPS(context.Background(), ep)
		require.NoError(t, err)
	}
	assert.EqualValues(t, 1, conns.Load(), "sequential fetches should reuse one keep-alive connection")
}

type countingLogger struct {
	mockStyledLogger
	warns, debugs atomic.Int32
}

func (c *countingLogger) Warn(string, ...any)  { c.warns.Add(1) }
func (c *countingLogger) Debug(string, ...any) { c.debugs.Add(1) }

// A dashboard polling an instance whose /api/ps keeps failing must not write a
// WARN per poll (the flood #237 removed elsewhere): one WARN per endpoint per
// window, every failure still visible at Debug.
func TestOllamaRunningModels_FailureWarnIsSampled(t *testing.T) {
	t.Parallel()

	log := &countingLogger{}
	app := &Application{
		logger:     log,
		rejectLog:  newRejectLogs(log, newThrottleSet()),
		repository: &mockRepository{},
	}
	good := psBackend(t, 200, `{"models":[]}`)
	bad := psBackend(t, 500, `boom`)
	app.repository = &mockRepository{endpoints: []*domain.Endpoint{
		psEndpoint("good", "ollama", good, domain.StatusHealthy),
		psEndpoint("bad1", "ollama", bad, domain.StatusHealthy),
		psEndpoint("bad2", "ollama", bad, domain.StatusHealthy),
	}}

	const polls = 20
	for range polls {
		w := httptest.NewRecorder()
		app.ollamaRunningModelsHandler(w, httptest.NewRequest(http.MethodGet, "/olla/ollama/api/ps", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
	assert.EqualValues(t, 2, log.warns.Load(), "one WARN per failing endpoint, not per poll")
	assert.EqualValues(t, 2*polls, log.debugs.Load(), "every failure stays visible at Debug")
}

// The fan-out hits every Ollama host, so it must sit behind the same rate limiter
// as the proxy routes; a plain registration would leave it unthrottled.
func TestOllamaPS_RouteIsRateLimited(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)
	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:           &config.Config{},
		logger:           &mockStyledLogger{},
		routeRegistry:    reg,
		profileFactory:   factory,
		repository:       &mockRepository{},
		converterFactory: converter.NewConverterFactory(),
	}
	app.registerProviderRoutes()

	sec := &SecurityAdapters{securityAdapters: buildTightSecurityAdapters(t), logger: &mockStyledLogger{}}
	mux := http.NewServeMux()
	reg.WireUpWithSecurityChain(mux, sec)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/olla/ollama/api/ps", nil))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
}

// One instance cannot make Olla buffer more than ollamaPSMaxBody.
func TestFetchOllamaPS_BodyCap(t *testing.T) {
	t.Parallel()

	pad := strings.Repeat("x", ollamaPSMaxBody)
	big := psBackend(t, 200, `{"models":[{"name":"m","pad":"`+pad+`"}]}`)
	_, err := fetchOllamaPS(context.Background(), psEndpoint("big", "ollama", big, domain.StatusHealthy))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")

	small := psBackend(t, 200, `{"models":[{"name":"m","pad":"`+strings.Repeat("x", 1<<20)+`"}]}`)
	got, err := fetchOllamaPS(context.Background(), psEndpoint("small", "ollama", small, domain.StatusHealthy))
	require.NoError(t, err)
	assert.Len(t, got, 1)
}
