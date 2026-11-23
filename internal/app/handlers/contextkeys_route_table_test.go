package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/theme"
)

// Characterisation of the context keys the typed-key rework retypes, pinned
// through the real route table (docs/specs/go-1.27-modernise.md, WP-6):
// route_prefix as the proxy registration, the provider handler's raw-alias
// override and the translator registration supply it or not; model_alias_map
// as the forwarded body's model rewrite; and which request ID the pipeline
// logs. Everything is asserted on what the backends and the client observe.

const (
	ctxKeysOllamaModels   = `{"models":[{"name":"mistral-small3.2:latest","capabilities":["completion"]}]}`
	ctxKeysLmStudioModels = `{"data":[{"id":"Qwen/Qwen3-32B","object":"model"}]}`

	// One model per OpenAI-compatible profile the fleet serves, unique to that
	// profile's backend so strict model routing narrows to it alone.
	ctxKeysVllmModel         = "llama3.1-vllm"
	ctxKeysVllmMlxModel      = "llama3.1-vllm-mlx"
	ctxKeysSglangModel       = "llama3.1-sglang"
	ctxKeysLmdeployModel     = "llama3.1-lmdeploy"
	ctxKeysLlamacppModel     = "llama3.1-llamacpp"
	ctxKeysLitellmModel      = "llama3.1-litellm"
	ctxKeysOpenAICompatModel = "llama3.1-openai"
	ctxKeysRapidMlxModel     = "llama3.1-rapid-mlx"
	ctxKeysOmlxModel         = "llama3.1-omlx"
	ctxKeysDmrModel          = "llama3.1-dmr"

	// Aliases resolve to backend-specific model names; the forwarded body must
	// carry the name of whichever backend the balancer picked.
	aliasOllamaChat = "ollama-chat"
	aliasFleetChat  = "fleet-chat"
	ollamaTarget    = "mistral-small3.2:latest"
	lmStudioTarget  = "Qwen/Qwen3-32B"
)

// backendHit records what one backend observed.
type backendHit struct {
	path    string
	request map[string]any
	rawBody string
}

type recordingBackend struct {
	name   string
	server *httptest.Server
	mu     sync.Mutex
	hits   []backendHit
}

// recordingBackends answers in whichever wire format the path implies: an
// OpenAI chat completion on /v1/chat/completions, an Anthropic message on
// /v1/messages, and an SSE stream when asked to.
func newRecordingBackend(t *testing.T, name string, sse bool) *recordingBackend {
	t.Helper()
	b := &recordingBackend{name: name}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)

		b.mu.Lock()
		b.hits = append(b.hits, backendHit{path: r.URL.Path, request: req, rawBody: string(raw)})
		b.mu.Unlock()

		switch {
		case sse:
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for _, chunk := range []string{
				`data: {"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"content":"hi"}}]}` + "\n\n",
				"data: [DONE]\n\n",
			} {
				_, _ = w.Write([]byte(chunk))
				w.(http.Flusher).Flush()
			}
		case r.URL.Path == "/v1/messages":
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"`+lmStudioTarget+`","usage":{"input_tokens":1,"output_tokens":1}}`)
		default:
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		}
	}))
	t.Cleanup(b.server.Close)
	return b
}

func (b *recordingBackend) recorded() []backendHit {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backendHit, len(b.hits))
	copy(out, b.hits)
	return out
}

func (b *recordingBackend) url() string { return b.server.URL }

// ctxKeysFleet is a full application over a real unified registry, the shipped
// profiles, a real Olla engine and one recording backend per served profile,
// wired through the production route-registration path (proxy route, provider
// routes from the profile YAML, translator route).
type ctxKeysFleet struct {
	app      *Application
	mux      *http.ServeMux
	ollama   *recordingBackend
	lmStudio *recordingBackend
	// providers maps profile name to that profile's backend, ollama and
	// lm-studio included, so the provider strip test can reach every served
	// backend by profile.
	providers    map[string]*recordingBackend
	logs         *rawLogBuffer
	styledLogger logger.StyledLogger
}

// rawLogBuffer captures JSON log lines so tests can assert on full records.
type rawLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *rawLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// recordsFor returns the parsed records whose msg matches.
func (b *rawLogBuffer) recordsFor(t *testing.T, msg string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for line := range strings.SplitSeq(b.buf.String(), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (%q)", err, line)
		}
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

func newCtxKeysFleet(t *testing.T, passthrough, sseBackends bool) *ctxKeysFleet {
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

	strategy := config.ModelRoutingStrategy{Type: "strict"}
	unified := registry.NewUnifiedMemoryModelRegistry(log, nil, &strategy, nil)

	ollamaBackend := newRecordingBackend(t, "ollama-1", sseBackends)
	lmStudioBackend := newRecordingBackend(t, "lmstudio-1", sseBackends)

	ctx := t.Context()
	var endpoints []*domain.Endpoint
	var registered []*domain.ModelInfo
	providers := map[string]*recordingBackend{}

	// Every OpenAI-compatible shipped profile gets an endpoint of its own:
	// their model-listing parsers share the OpenAI list shape, and a plain
	// recording backend can serve their chat requests. ollama-native (one
	// endpoint, above) completes the served set. lemonade and
	// text-embeddings-inference are deliberately absent: their wire formats
	// need real protocol handling, so the provider strip test presence-asserts
	// their prefixes instead of driving them.
	openAIListModel := func(model string) string {
		return fmt.Sprintf(`{"data":[{"id":%q}]}`, model)
	}

	for _, spec := range []struct {
		name, typ, models string
		backend           *recordingBackend
	}{
		{"ollama-1", domain.ProfileOllama, ctxKeysOllamaModels, ollamaBackend},
		{"lmstudio-1", domain.ProfileLmStudio, ctxKeysLmStudioModels, lmStudioBackend},
		{"vllm-1", domain.ProfileVLLM, openAIListModel(ctxKeysVllmModel), newRecordingBackend(t, "vllm-1", sseBackends)},
		{"vllm-mlx-1", domain.ProfileVLLMMLX, openAIListModel(ctxKeysVllmMlxModel), newRecordingBackend(t, "vllm-mlx-1", sseBackends)},
		{"sglang-1", domain.ProfileSGLang, openAIListModel(ctxKeysSglangModel), newRecordingBackend(t, "sglang-1", sseBackends)},
		{"lmdeploy-1", domain.ProfileLMDeploy, openAIListModel(ctxKeysLmdeployModel), newRecordingBackend(t, "lmdeploy-1", sseBackends)},
		{"llamacpp-1", domain.ProfileLlamaCpp, openAIListModel(ctxKeysLlamacppModel), newRecordingBackend(t, "llamacpp-1", sseBackends)},
		{"litellm-1", "litellm", openAIListModel(ctxKeysLitellmModel), newRecordingBackend(t, "litellm-1", sseBackends)},
		{"openai-1", domain.ProfileOpenAICompatible, openAIListModel(ctxKeysOpenAICompatModel), newRecordingBackend(t, "openai-1", sseBackends)},
		{"rapid-mlx-1", domain.ProfileRapidMLX, openAIListModel(ctxKeysRapidMlxModel), newRecordingBackend(t, "rapid-mlx-1", sseBackends)},
		{"omlx-1", domain.ProfileOMLX, openAIListModel(ctxKeysOmlxModel), newRecordingBackend(t, "omlx-1", sseBackends)},
		{"dmr-1", domain.ProfileDockerMR, openAIListModel(ctxKeysDmrModel), newRecordingBackend(t, "dmr-1", sseBackends)},
	} {
		rawURL := spec.backend.url()
		u, perr := url.Parse(rawURL)
		require.NoError(t, perr)
		ep := &domain.Endpoint{Name: spec.name, URL: u, URLString: u.String(), Type: spec.typ, Status: domain.StatusHealthy}
		endpoints = append(endpoints, ep)

		prof, perr := factory.GetProfile(spec.typ)
		require.NoError(t, perr)
		models, perr := prof.ParseModelsResponse([]byte(spec.models))
		require.NoError(t, perr)
		require.NoError(t, unified.RegisterModelsWithEndpoint(ctx, ep, models))
		registered = append(registered, models...)
		providers[spec.typ] = spec.backend
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

	aliases := map[string][]string{
		aliasOllamaChat: {ollamaTarget},
		aliasFleetChat:  {ollamaTarget, lmStudioTarget},
	}

	logs := &rawLogBuffer{}
	styledLogger := logger.NewStyledLogger(
		slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		&theme.Theme{}, false)
	collector := stats.NewCollector(styledLogger)
	selector, err := balancer.NewFactory(collector).Create(balancer.DefaultBalancerPriority)
	require.NoError(t, err)

	engineCfg := &olla.Configuration{}
	engineCfg.ResponseTimeout, engineCfg.ReadTimeout, engineCfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
	svc, serr := olla.NewService(&mockDiscoveryServiceWithHealthy{endpoints: endpoints}, selector, engineCfg, collector, nil, styledLogger)
	require.NoError(t, serr)
	t.Cleanup(svc.Cleanup)

	app := &Application{
		Config:           &config.Config{},
		logger:           styledLogger,
		inspectorChain:   chain,
		profileFactory:   factory,
		profileLookup:    factory,
		discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: endpoints},
		modelRegistry:    unified,
		aliasResolver:    registry.NewAliasResolverWithUnification(aliases, styledLogger, false),
		repository:       &mockRepository{endpoints: endpoints},
		statsCollector:   collector,
		rejectLog:        newRejectLogs(styledLogger, nil),
		proxyService:     svc,
		StartTime:        time.Now(),
	}

	// The real route table: the legacy proxy route, provider routes built from
	// the shipped profile YAML (per-prefix base paths included), and the
	// Anthropic translator route registered the way the translator registry
	// registers it (no route-prefix wrapper).
	reg := router.NewRouteRegistry(styledLogger)
	app.routeRegistry = reg
	reg.RegisterProxyRoute("/olla/proxy/", app.proxyHandler, "Olla API proxy endpoint", "POST")
	app.registerProviderRoutes()
	reg.RegisterTranslatorRoute("/olla/anthropic/v1/messages", app.translationHandler(trans), "anthropic Messages API", "POST", nil, 0)

	mux := http.NewServeMux()
	reg.WireUp(mux)

	return &ctxKeysFleet{
		app:          app,
		mux:          mux,
		ollama:       ollamaBackend,
		lmStudio:     lmStudioBackend,
		providers:    providers,
		logs:         logs,
		styledLogger: styledLogger,
	}
}

func (f *ctxKeysFleet) post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// assertNoHitsExcept fails when any served backend other than the named
// profiles received a request: with twelve profiles in the fleet, a test that
// counts hits on two backends must also show the other ten stayed idle.
func (f *ctxKeysFleet) assertNoHitsExcept(t *testing.T, except ...string) {
	t.Helper()
	for profileName, b := range f.providers {
		if slices.Contains(except, profileName) {
			continue
		}
		assert.Empty(t, b.recorded(), "the %s backend must not serve this request", profileName)
	}
}

// Proxy registration supplies the route prefix: a request through the legacy
// /olla/proxy/ route reaches the backend with that prefix stripped, without
// the test installing anything in the context.
func TestRouteTable_ProxyRouteStripsRegisteredPrefix(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, false)

	w := f.post(t, "/olla/proxy/v1/chat/completions",
		`{"model":"mistral-small3.2","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	hits := f.ollama.recorded()
	require.Len(t, hits, 1, "the ollama backend must serve the request exactly once")
	assert.Equal(t, "/v1/chat/completions", hits[0].path,
		"the proxy route registration's prefix must be stripped before forwarding")
}

// Every proxy prefix every shipped profile registers is present in the route
// table, and every profile the fleet serves strips its prefix. The test
// enumerates the registry's actual route table (the same GetRoutes the status
// endpoints read) rather than naming routes: presence is asserted for every
// prefix the profile YAML registers, and each served profile gets a strip
// subtest. So a registration or prefix regression on any shipped profile fails
// here, and a strip regression fails on every served profile; lemonade and the
// two text-embeddings-inference prefixes are registration-only, because their
// wire formats need protocol-specific requests the recording backends cannot
// serve. The provider handler overrides the registration wrapper's prefix with
// the raw alias spelling from the URL, so whatever spelling a profile
// registers (lmstudio, lm-studio, lm_studio) strips correctly.
func TestRouteTable_ProviderRoutesStripTheirRegisteredPrefix(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, false)

	// The profiles the fleet serves, each with a model unique to its backend
	// and the chat path that profile serves (Docker Model Runner mounts its
	// OpenAI surface under /engines/v1 rather than the root).
	served := map[string]struct {
		model string
		path  string
	}{
		domain.ProfileOllama:           {"mistral-small3.2", "/v1/chat/completions"},
		domain.ProfileLmStudio:         {lmStudioTarget, "/v1/chat/completions"},
		domain.ProfileVLLM:             {ctxKeysVllmModel, "/v1/chat/completions"},
		domain.ProfileVLLMMLX:          {ctxKeysVllmMlxModel, "/v1/chat/completions"},
		domain.ProfileSGLang:           {ctxKeysSglangModel, "/v1/chat/completions"},
		domain.ProfileLMDeploy:         {ctxKeysLmdeployModel, "/v1/chat/completions"},
		domain.ProfileLlamaCpp:         {ctxKeysLlamacppModel, "/v1/chat/completions"},
		"litellm":                      {ctxKeysLitellmModel, "/v1/chat/completions"},
		domain.ProfileOpenAICompatible: {ctxKeysOpenAICompatModel, "/v1/chat/completions"},
		domain.ProfileRapidMLX:         {ctxKeysRapidMlxModel, "/v1/chat/completions"},
		domain.ProfileOMLX:             {ctxKeysOmlxModel, "/v1/chat/completions"},
		domain.ProfileDockerMR:         {ctxKeysDmrModel, "/engines/v1/chat/completions"},
	}

	// prefix -> profile, from the same profile YAML the registration read.
	// GetAvailableProfiles hides openai-compatible (a protocol spec, not a
	// provider), but registration appends it by hand (server_routes.go
	// registerProviderRoutes); mirror that so the openai and
	// openai-compatible prefixes are covered too.
	prefixToProfile := map[string]string{}
	profileNames := append(f.app.profileFactory.GetAvailableProfiles(), domain.ProfileOpenAICompatible)
	for _, profileName := range profileNames {
		profile, err := f.app.profileFactory.GetProfile(profileName)
		require.NoError(t, err)
		profileCfg := profile.GetConfig()
		require.NotNil(t, profileCfg)
		for _, prefix := range profileCfg.Routing.Prefixes {
			prefixToProfile[prefix] = profileName
		}
	}

	type providerRoute struct {
		route   string
		model   string
		path    string
		backend *recordingBackend
	}
	registeredPrefixes := map[string]bool{}
	var routes []providerRoute
	for route, info := range f.app.routeRegistry.GetRoutes() {
		// Provider catch-alls are the proxy-registered "/olla/<prefix>/" routes;
		// every other proxy route under a provider prefix (api/ps and friends)
		// has a longer path and no profile-prefix spelling.
		seg, ok := strings.CutPrefix(route, constants.DefaultOllaProxyPathPrefix)
		if !ok || !info.IsProxy || !strings.HasSuffix(seg, constants.DefaultPathPrefix) {
			continue
		}
		prefix := strings.TrimSuffix(seg, constants.DefaultPathPrefix)
		registeredPrefixes[prefix] = true
		if svc, ok := served[prefixToProfile[prefix]]; ok {
			routes = append(routes, providerRoute{
				route:   route,
				model:   svc.model,
				path:    svc.path,
				backend: f.providers[prefixToProfile[prefix]],
			})
		}
	}

	// Presence for every shipped profile, served or not: each prefix the YAML
	// registers must have its provider catch-all in the registered route table,
	// so a dropped registration or a dropped prefix on any profile fails here.
	for prefix, profileName := range prefixToProfile {
		require.True(t, registeredPrefixes[prefix],
			"prefix %s (profile %s) is registered in the profile YAML but its provider catch-all is missing from the route table", prefix, profileName)
	}

	// Deterministic order so a failure reads the same way on every run.
	slices.SortFunc(routes, func(a, b providerRoute) int {
		return strings.Compare(a.route, b.route)
	})

	// Sequential on purpose: several routes share a backend, so each subtest
	// asserts on the hit that arrived after its own request.
	for _, r := range routes {
		t.Run(r.route, func(t *testing.T) {
			before := map[*recordingBackend]int{}
			for _, b := range f.providers {
				before[b] = len(b.recorded())
			}
			w := f.post(t, r.route+strings.TrimPrefix(r.path, "/"),
				`{"model":"`+r.model+`","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

			require.Equal(t, http.StatusOK, w.Code, "route %s, body: %s", r.route, w.Body.String())
			hits := r.backend.recorded()
			require.Len(t, hits, before[r.backend]+1, "route %s must reach its backend exactly once", r.route)
			assert.Equal(t, r.path, hits[len(hits)-1].path,
				"route %s must strip its registered prefix before forwarding", r.route)
			// Cross-hit guard: a provider route must confine the request to its
			// own profile's backend, so no other served backend may see a hit
			// (restores the old lm-studio-must-not-reach-ollama assertion,
			// widened to every served profile).
			for profileName, b := range f.providers {
				if b == r.backend {
					continue
				}
				assert.Len(t, b.recorded(), before[b],
					"route %s must not reach the %s backend", r.route, profileName)
			}
		})
	}
}

// Translator registration installs no route prefix, and none is needed: the
// translator path rewrites the URL itself (passthrough uses the backend
// profile's native Anthropic path). The /olla/anthropic prefix never reaches
// the backend, with no context stripping involved.
func TestRouteTable_TranslatorRoutePathComesFromBackendProfile(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, false)

	w := f.post(t, "/olla/anthropic/v1/messages",
		`{"model":"`+lmStudioTarget+`","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, "passthrough", w.Header().Get(constants.HeaderXOllaMode),
		"the lm-studio profile declares native Anthropic support, so this must passthrough")

	hits := f.lmStudio.recorded()
	require.Len(t, hits, 1, "the lm-studio backend must serve the request exactly once")
	assert.Equal(t, "/v1/messages", hits[0].path,
		"the backend must receive its configured native Anthropic path, not the client's /olla/anthropic path")
	assert.Equal(t, lmStudioTarget, hits[0].request["model"],
		"passthrough must forward the body unchanged")
}

// A probe handler registered each way separates "a route prefix was injected
// at registration" from "none was": the probe applies the same stripping the
// proxy path applies and records what it saw. The proxy-registered probe's
// path comes back stripped; the translator-registered probe's comes back
// whole, so translator registration injects no route prefix. This is the
// outside-observable discriminator for the absence case the end-to-end
// translator test above cannot show (its backend path comes from the
// backend profile, not from context stripping).
func TestRouteTable_TranslatorRegistrationInjectsNoRoutePrefix(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, false)

	var mu sync.Mutex
	var proxySeen, translatorSeen string
	probe := func(seen *string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			*seen = util.StripRoutePrefix(r.Context(), r.URL.Path, constants.ContextRoutePrefixKey)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}
	}

	reg := f.app.routeRegistry
	reg.RegisterProxyRoute("/olla/probe/", probe(&proxySeen), "prefix probe (proxy registration)", "POST")
	reg.RegisterTranslatorRoute("/olla/anthropic/v1/messages/probe", probe(&translatorSeen), "prefix probe (translator registration)", "POST", nil, 0)

	// The fleet wired its mux before the probes existed; wire a fresh mux from
	// the same registry so both probe routes are reachable.
	f.mux = http.NewServeMux()
	reg.WireUp(f.mux)

	w := f.post(t, "/olla/probe/check", "")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	w = f.post(t, "/olla/anthropic/v1/messages/probe", "")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "/check", proxySeen,
		"the proxy registration's prefix must be injected and stripped")
	assert.Equal(t, "/olla/anthropic/v1/messages/probe", translatorSeen,
		"translator registration must inject no route prefix: the probe sees the whole path")
}

// A streaming request through the proxy route works with no writer of the
// stream context key anywhere in production: streaming is decided by the
// response's content type alone.
func TestRouteTable_StreamingWithoutStreamContextKeyWriter(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, true)

	w := f.post(t, "/olla/proxy/v1/chat/completions",
		`{"model":"mistral-small3.2","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, "text/event-stream", w.Header().Get(constants.HeaderContentType),
		"an SSE backend response must stream without any stream key in the context")
	assert.Contains(t, w.Body.String(), "chat.completion.chunk")
	assert.Len(t, f.ollama.recorded(), 1)
}

// The alias rewrite map: an alias resolves to backend-specific model names and
// the forwarded body must carry the name of the backend that was selected,
// on both the proxy route and the translated Anthropic route.
func TestRouteTable_AliasRewritesForwardedBodyModel(t *testing.T) {
	t.Parallel()

	t.Run("proxy route rewrites per selected backend", func(t *testing.T) {
		t.Parallel()
		f := newCtxKeysFleet(t, true, false)

		w := f.post(t, "/olla/proxy/v1/chat/completions",
			`{"model":"`+aliasFleetChat+`","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		// Every served backend counts: the other ten must have stayed idle for
		// the one-hit total below to mean "exactly one backend served it".
		f.assertNoHitsExcept(t, domain.ProfileOllama, domain.ProfileLmStudio)
		ollamaHits := f.ollama.recorded()
		lmStudioHits := f.lmStudio.recorded()
		require.Equal(t, 1, len(ollamaHits)+len(lmStudioHits),
			"exactly one backend must serve the request")

		targetByBackend := map[string]string{
			"ollama-1":   ollamaTarget,
			"lmstudio-1": lmStudioTarget,
		}
		for name, hits := range map[string][]backendHit{"ollama-1": ollamaHits, "lmstudio-1": lmStudioHits} {
			for _, hit := range hits {
				assert.Equal(t, targetByBackend[name], hit.request["model"],
					"the body forwarded to %s must carry its backend-specific model name", name)
			}
		}
	})

	t.Run("translated anthropic request rewrites for the backend", func(t *testing.T) {
		t.Parallel()
		// Passthrough off so the request must translate: the alias map the proxy
		// stage stored on the routing profile is what the translation handler
		// reads back and installs in the context for the selected backend.
		f := newCtxKeysFleet(t, false, false)

		w := f.post(t, "/olla/anthropic/v1/messages",
			`{"model":"`+aliasOllamaChat+`","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Empty(t, w.Header().Get(constants.HeaderXOllaMode), "this request must go through translation, not passthrough")

		hits := f.ollama.recorded()
		require.Len(t, hits, 1, "the alias target lives on the ollama backend")
		assert.Equal(t, "/v1/chat/completions", hits[0].path,
			"the translated request must go to the OpenAI chat path")
		assert.Equal(t, ollamaTarget, hits[0].request["model"],
			"the translated body must carry the backend's model name after the alias rewrite")
		f.assertNoHitsExcept(t, domain.ProfileOllama)
	})
}

// Which request ID the pipeline uses: the proxy reuses the ID the access-log
// middleware assigned from the inbound X-Request-ID header rather than
// generating its own, and both lifecycle log lines (middleware and proxy
// handler) name that same ID under the request_id log key.
func TestRouteTable_RequestIDFlowsFromAccessLogMiddleware(t *testing.T) {
	t.Parallel()
	f := newCtxKeysFleet(t, true, false)

	const inboundID = "contextkeys-req-0042"
	handler := middleware.CombinedLoggingMiddleware(f.styledLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mux.ServeHTTP(w, r)
	}))

	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions",
		strings.NewReader(`{"model":"mistral-small3.2","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderXRequestID, inboundID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, inboundID, w.Header().Get(constants.HeaderXOllaRequestID),
		"the middleware-assigned request ID must survive the proxy pipeline unchanged")
	require.Len(t, f.ollama.recorded(), 1)

	// The handler's completion line and the access-log middleware's quiet
	// completion and access-log lines must all name the middleware-assigned
	// ID: the proxy handler reuses it rather than generating a fresh one.
	for _, msg := range []string{"Request completed", "HTTP request completed", "Access log"} {
		records := f.logs.recordsFor(t, msg)
		require.NotEmpty(t, records, "expected a %q log line", msg)
		for _, rec := range records {
			assert.Equal(t, inboundID, rec["request_id"],
				"the %q line must carry the middleware-assigned request ID: %v", msg, rec)
		}
	}
}
