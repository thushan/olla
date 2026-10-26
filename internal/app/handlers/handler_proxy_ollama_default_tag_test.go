package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

// Ollama lists a default-tag pull as "name:latest" while clients ask for "name".
// Nothing normalised that, so the bare name was a 404 on every route. These tests
// walk the route table so a path that bypasses the registry rule shows up here.

const (
	tagOllamaTags = `{"models":[
		{"name":"all-minilm:latest","capabilities":["embedding"]},
		{"name":"mistral-small3.2:latest","capabilities":["completion"]},
		{"name":"qwen2.5:0.5b","capabilities":["completion"]}]}`
	tagVLLMModels = `{"data":[{"id":"Qwen/Qwen3-32B","object":"model"},{"id":"BAAI/bge-m3","object":"model"}]}`

	tagBareEmbed = "all-minilm"
	tagBareChat  = "mistral-small3.2"
)

var tagStrategies = []struct {
	name string
	cfg  config.ModelRoutingStrategy
}{
	{"strict", config.ModelRoutingStrategy{Type: "strict"}},
	{"optimistic-compatible-only", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorCompatibleOnly}}},
	{"optimistic-all", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorAll}}},
}

// tagFleet builds an application over a real unified registry holding one Ollama
// and one vLLM host, parsed through the shipped profiles like production discovery.
type tagFleet struct {
	app       *Application
	registry  *registry.UnifiedMemoryModelRegistry
	endpoints []*domain.Endpoint
	trans     *anthropic.Translator
}

func newTagFleet(t *testing.T, strategy config.ModelRoutingStrategy, urls map[string]string, sticky bool, passthrough bool) *tagFleet {
	t.Helper()

	log := &mockStyledLogger{}
	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	pathInspector := inspector.NewPathInspector(factory, log)
	chain := inspector.NewChain(log)
	chain.AddInspector(pathInspector)
	bodyInspector, err := inspector.NewBodyInspector(log)
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)

	trans, err := anthropic.NewTranslator(log, config.AnthropicTranslatorConfig{PassthroughEnabled: passthrough})
	require.NoError(t, err)

	cfg := strategy
	unified := registry.NewUnifiedMemoryModelRegistry(log, nil, &cfg, nil)

	ctx := context.Background()
	var endpoints []*domain.Endpoint
	var registered []*domain.ModelInfo
	for _, spec := range []struct{ name, typ, models string }{
		{"ollama-1", domain.ProfileOllama, tagOllamaTags},
		{"vllm-1", domain.ProfileVLLM, tagVLLMModels},
	} {
		raw := urls[spec.name]
		if raw == "" {
			raw = "http://" + spec.name + ".test:8080"
		}
		u, perr := url.Parse(raw)
		require.NoError(t, perr)
		ep := &domain.Endpoint{Name: spec.name, URL: u, URLString: u.String(), Type: spec.typ, Status: domain.StatusHealthy}
		endpoints = append(endpoints, ep)

		prof, perr := factory.GetProfile(spec.typ)
		require.NoError(t, perr)
		models, perr := prof.ParseModelsResponse([]byte(spec.models))
		require.NoError(t, perr)
		require.NoError(t, unified.RegisterModelsWithEndpoint(ctx, ep, models))
		registered = append(registered, models...)
	}
	// Unification is asynchronous: wait until every native name resolves.
	require.Eventually(t, func() bool {
		for _, m := range registered {
			if u, uerr := unified.GetUnifiedModel(ctx, m.Name); uerr != nil || u == nil {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)

	appCfg := &config.Config{}
	if sticky {
		appCfg.Proxy.StickySessions = config.StickySessionConfig{Enabled: true, KeySources: []string{"session_header"}, MaxSessions: 100, IdleTTLSeconds: 60, PrefixHashBytes: 512}
	}

	app := &Application{
		Config:           appCfg,
		logger:           log,
		inspectorChain:   chain,
		profileFactory:   factory,
		profileLookup:    factory,
		discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: endpoints},
		modelRegistry:    unified,
		repository:       &mockRepository{endpoints: endpoints},
		statsCollector:   &mockStatsCollector{},
		StartTime:        time.Now(),
	}
	return &tagFleet{app: app, registry: unified, endpoints: endpoints, trans: trans}
}

// seen is what the proxy engine was handed for one request.
type seen struct {
	ctx       context.Context
	decision  *domain.ModelRoutingDecision
	body      string
	endpoints []string
}

func (f *tagFleet) capture() *seen {
	got := &seen{}
	f.app.proxyService = &mockProxyService{proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, st *ports.RequestStats, _ logger.StyledLogger) error {
		got.ctx = ctx
		got.decision = st.RoutingDecision
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			got.body = string(b)
		}
		for _, ep := range eps {
			got.endpoints = append(got.endpoints, ep.Name)
		}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
		return nil
	}}
	return got
}

type tagRoute struct {
	name string
	path string
	// bare is the model's untagged name; the tagged spelling is bare + ":latest"
	bare string
	// handler picks which entry point serves the route
	handler string
}

func tagRoutes() []tagRoute {
	var routes []tagRoute
	add := func(prefix, handler string, embed, chat []string) {
		for _, p := range embed {
			routes = append(routes, tagRoute{prefix + p, prefix + p, tagBareEmbed, handler})
		}
		for _, p := range chat {
			routes = append(routes, tagRoute{prefix + p, prefix + p, tagBareChat, handler})
		}
	}
	add("/olla/proxy", "proxy", []string{"/v1/embeddings", "/api/embed", "/api/embeddings"}, []string{"/v1/chat/completions", "/api/chat", "/api/generate"})
	add("/olla/ollama", "provider", []string{"/v1/embeddings", "/api/embed", "/api/embeddings"}, []string{"/v1/chat/completions", "/api/chat", "/api/generate"})
	// The OpenAI-compatible prefixes also serve Ollama hosts
	add("/olla/openai", "provider", []string{"/v1/embeddings"}, []string{"/v1/chat/completions"})
	add("/olla/openai-compatible", "provider", []string{"/v1/embeddings"}, []string{"/v1/chat/completions"})
	routes = append(routes,
		tagRoute{"anthropic messages", "/olla/anthropic/v1/messages", tagBareChat, "anthropic"},
		tagRoute{"model detail", "/olla/models/", tagBareEmbed, "detail"},
	)
	return routes
}

func (f *tagFleet) do(t *testing.T, route tagRoute, model string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()

	switch route.handler {
	case "detail":
		mux := http.NewServeMux()
		mux.HandleFunc("GET /olla/models/", f.app.unifiedModelByAliasHandler)
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route.path+url.PathEscape(model), nil))
		return w
	case "anthropic":
		body := `{"model":"` + model + `","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		f.app.translationHandler(f.trans).ServeHTTP(w, req)
		return w
	}

	body := `{"model":"` + model + `","input":"hi","prompt":"hi","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if route.handler == "proxy" {
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
		f.app.proxyHandler(w, req)
	} else {
		f.app.providerProxyHandler(w, req)
	}
	return w
}

func TestProxyRoutes_OllamaBareNameResolvesToLatest(t *testing.T) {
	t.Parallel()

	for _, route := range tagRoutes() {
		for _, strategy := range tagStrategies {
			for _, spelling := range []string{"bare", "tagged"} {
				t.Run(route.name+"/"+strategy.name+"/"+spelling, func(t *testing.T) {
					t.Parallel()

					fleet := newTagFleet(t, strategy.cfg, nil, false, false)
					got := fleet.capture()
					model := route.bare
					if spelling == "tagged" {
						model += ":latest"
					}

					w := fleet.do(t, route, model)

					require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
					if route.handler == "detail" {
						assert.Contains(t, w.Body.String(), `"id":"`+route.bare+`:latest"`)
						return
					}
					assert.Equal(t, []string{"ollama-1"}, got.endpoints, "only the Ollama host serves it")
					require.NotNil(t, got.decision)
					assert.Equal(t, constants.RoutingReasonModelFound, got.decision.Reason)
					assert.Contains(t, got.body, `"model":"`+model+`"`, "the client's spelling is forwarded untouched")

					canonical, hasCanonical := got.ctx.Value(constants.ContextCanonicalModelKey).(core.CanonicalModel)
					if spelling == "bare" {
						require.True(t, hasCanonical, "a bare name is labelled with its native name for stats")
						assert.Equal(t, route.bare+":latest", canonical.Native)
					} else {
						assert.False(t, hasCanonical, "an exact name needs no relabelling")
					}
				})
			}
		}
	}
}

// A genuinely unknown model is still refused: the fallback only ever adds ":latest".
func TestProxyRoutes_OllamaBareName_UnknownStillRejected(t *testing.T) {
	t.Parallel()

	for _, route := range tagRoutes() {
		for _, strategy := range tagStrategies[:2] {
			t.Run(route.name+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()

				fleet := newTagFleet(t, strategy.cfg, nil, false, false)
				got := fleet.capture()

				w := fleet.do(t, route, "nonexistent-xyz")

				assert.Equal(t, http.StatusNotFound, w.Code, "body: %s", w.Body.String())
				assert.Empty(t, got.endpoints)
			})
		}
	}
}

// A vLLM host's "name:latest" must not answer a bare name, and a bare name must not
// leak onto /olla/vllm/, which cannot host Ollama.
func TestProxyRoutes_OllamaBareName_DoesNotCrossPlatforms(t *testing.T) {
	t.Parallel()

	fleet := newTagFleet(t, tagStrategies[0].cfg, nil, false, false)
	got := fleet.capture()

	w := fleet.do(t, tagRoute{path: "/olla/vllm/v1/embeddings", handler: "provider"}, tagBareEmbed)
	assert.NotEqual(t, http.StatusOK, w.Code, "vLLM prefix must not reach the Ollama host")
	assert.Empty(t, got.endpoints)
}

// One conversation must keep its affinity whichever spelling the client uses.
func TestProxyRoutes_OllamaBareName_StickyKeyIsSpellingIndependent(t *testing.T) {
	t.Parallel()

	keyFor := func(model string) string {
		fleet := newTagFleet(t, tagStrategies[0].cfg, nil, true, false)
		got := fleet.capture()
		body := `{"model":"` + model + `","input":"hi"}`
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/embeddings", strings.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		req.Header.Set(constants.HeaderXOllaSessionID, "session-1")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
		w := httptest.NewRecorder()
		fleet.app.proxyHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		key, _ := got.ctx.Value(constants.ContextStickyKeyKey).(string)
		require.NotEmpty(t, key)
		return key
	}

	assert.Equal(t, keyFor(tagBareEmbed+":latest"), keyFor(tagBareEmbed), "foo and foo:latest share one sticky key")
}

// backendRecorder stands in for Ollama and records what it was sent.
type backendRecorder struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newBackendRecorder(t *testing.T) *backendRecorder {
	t.Helper()
	b := &backendRecorder{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.bodies = append(b.bodies, string(raw))
		b.mu.Unlock()
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		_, _ = w.Write([]byte(`{"model":"all-minilm:latest","embeddings":[[0.1]],"data":[{"embedding":[0.1]}]}`))
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *backendRecorder) received() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.bodies...)
}

// Real engines, real stats: the backend gets the client's spelling, stats aggregate
// under the native name, and an unknown model is a 404 on both engines.
func TestProxyEngines_OllamaBareName(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{"sherpa", "olla"} {
		for _, strategy := range tagStrategies[:2] {
			t.Run(engine+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()

				backend := newBackendRecorder(t)
				fleet := newTagFleet(t, strategy.cfg, map[string]string{"ollama-1": backend.URL}, false, false)

				log := logger.NewStyledLogger(slog.New(slog.NewTextHandler(io.Discard, nil)), &theme.Theme{}, false)
				collector := stats.NewCollector(log)
				selector, err := balancer.NewFactory(collector).Create(balancer.DefaultBalancerPriority)
				require.NoError(t, err)
				fleet.app.statsCollector = collector
				fleet.app.discoveryService = &mockDiscoveryServiceWithHealthy{endpoints: fleet.endpoints[:1]}
				discovery := fleet.app.discoveryService

				switch engine {
				case "sherpa":
					cfg := &sherpa.Configuration{}
					cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
					svc, serr := sherpa.NewService(discovery, selector, cfg, collector, nil, log)
					require.NoError(t, serr)
					fleet.app.proxyService = svc
				default:
					cfg := &olla.Configuration{}
					cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
					svc, serr := olla.NewService(discovery, selector, cfg, collector, nil, log)
					require.NoError(t, serr)
					t.Cleanup(svc.Cleanup)
					fleet.app.proxyService = svc
				}

				post := func(path, model string) *httptest.ResponseRecorder {
					body := `{"model":"` + model + `","input":"hi"}`
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w := httptest.NewRecorder()
					if strings.HasPrefix(path, "/olla/proxy/") {
						req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
						fleet.app.proxyHandler(w, req)
					} else {
						fleet.app.providerProxyHandler(w, req)
					}
					return w
				}

				require.Equal(t, http.StatusOK, post("/olla/proxy/v1/embeddings", "all-minilm").Code)
				require.Equal(t, http.StatusOK, post("/olla/ollama/api/embed", "all-minilm").Code)
				require.Equal(t, http.StatusOK, post("/olla/proxy/v1/embeddings", "all-minilm:latest").Code)

				bodies := backend.received()
				require.Len(t, bodies, 3)
				assert.Contains(t, bodies[0], `"model":"all-minilm"`, "bare stays bare on the wire")
				assert.NotContains(t, bodies[0], "all-minilm:latest")
				assert.Contains(t, bodies[1], `"model":"all-minilm"`)
				assert.Contains(t, bodies[2], `"model":"all-minilm:latest"`)

				modelStats := collector.GetModelStats()
				require.Contains(t, modelStats, "all-minilm:latest")
				assert.NotContains(t, modelStats, "all-minilm", "the bare spelling does not get its own row")
				assert.EqualValues(t, 3, modelStats["all-minilm:latest"].TotalRequests)

				unknown := post("/olla/proxy/v1/embeddings", "nonexistent-xyz")
				assert.Equal(t, http.StatusNotFound, unknown.Code, unknown.Body.String())
				assert.Len(t, backend.received(), 3, "an unknown model never reaches a backend")
			})
		}
	}
}
