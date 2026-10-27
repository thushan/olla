package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

// These tests drive the real proxy engines against backends that only serve the paths their
// real counterparts do. The mock proxies used elsewhere never build a URL, so a request sent
// to the wrong path went unnoticed: DMR and Lemonade 404 on the translator's fixed
// /v1/chat/completions.

const (
	dmrChatPath      = "/engines/v1/chat/completions"
	lemonadeChatPath = "/api/v1/chat/completions"
	rootChatPath     = "/v1/chat/completions"
)

// pathBackend answers only the paths in serves and records every path it was asked for.
type pathBackend struct {
	*httptest.Server
	mu   sync.Mutex
	hits []string
}

const (
	pathBackendChatJSON = `{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	pathBackendMessage  = `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	pathBackendSSE      = "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\n\n" +
		"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\n" +
		"data: [DONE]\n\n"
)

func newPathBackend(t *testing.T, serves ...string) *pathBackend {
	t.Helper()
	b := &pathBackend{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.hits = append(b.hits, r.URL.Path)
		b.mu.Unlock()
		for _, p := range serves {
			if r.URL.Path != p {
				continue
			}
			switch {
			case strings.HasSuffix(p, "/v1/messages"):
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
				_, _ = w.Write([]byte(pathBackendMessage))
			case strings.Contains(string(body), `"stream":true`):
				w.Header().Set(constants.HeaderContentType, "text/event-stream")
				_, _ = w.Write([]byte(pathBackendSSE))
			default:
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
				_, _ = w.Write([]byte(pathBackendChatJSON))
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *pathBackend) paths() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.hits...)
}

// closedBackendURL returns an address nothing listens on, so connecting is refused and the
// engines fail over. A port freed by a closed httptest server can be reissued to another
// parallel test, so a privileged port is used instead.
func closedBackendURL(t *testing.T) string {
	t.Helper()
	return "http://127.0.0.1:1"
}

func liveEndpoint(t *testing.T, name, typ, rawURL string, priority int) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{Name: name, Type: typ, URL: u, URLString: rawURL, Priority: priority, Status: domain.StatusHealthy}
}

type translationPathApp struct {
	app     *Application
	handler http.Handler
	trans   *anthropic.Translator
	log     *warnCounter
}

// newTranslationPathApp wires the shipped profiles, a real PathInspector and the chosen real
// engine. hosts maps model name to the endpoints that host it; strict picks the model routing
// strategy (strict rejects, optimistic falls back to every healthy endpoint).
func newTranslationPathApp(t *testing.T, engine string, strict, passthrough bool, endpoints []*domain.Endpoint, hosts map[string][]*domain.Endpoint) *translationPathApp {
	t.Helper()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	appLog := newWarnCounter()
	pathInspector := inspector.NewPathInspector(factory, appLog)
	chain := inspector.NewChain(appLog)
	chain.AddInspector(pathInspector)
	bodyInspector, err := inspector.NewBodyInspector(appLog)
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)

	trans, err := anthropic.NewTranslator(appLog, config.AnthropicTranslatorConfig{PassthroughEnabled: passthrough})
	require.NoError(t, err)

	endpointsForModel := make(map[string][]string, len(hosts))
	for model, eps := range hosts {
		for _, ep := range eps {
			endpointsForModel[model] = append(endpointsForModel[model], ep.URLString)
		}
	}

	engineLog := logger.NewStyledLogger(slog.New(slog.NewTextHandler(io.Discard, nil)), &theme.Theme{}, false)
	collector := stats.NewCollector(engineLog)
	selector, err := balancer.NewFactory(collector).Create(balancer.DefaultBalancerPriority)
	require.NoError(t, err)
	discovery := &mockDiscoveryServiceWithFunc{getHealthyEndpointsFunc: func(context.Context) ([]*domain.Endpoint, error) {
		return endpoints, nil
	}}

	app := &Application{
		Config:           &config.Config{},
		logger:           appLog,
		rejectLog:        newRejectLogs(appLog, nil),
		inspectorChain:   chain,
		pathInspector:    pathInspector,
		profileFactory:   factory,
		profileLookup:    factory,
		discoveryService: discovery,
		modelRegistry:    &mockSimpleModelRegistry{strict: strict, endpointsForModel: endpointsForModel},
		statsCollector:   collector,
	}

	switch engine {
	case "sherpa":
		cfg := &sherpa.Configuration{}
		cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
		svc, serr := sherpa.NewService(discovery, selector, cfg, collector, nil, engineLog)
		require.NoError(t, serr)
		app.proxyService = svc
	case "olla":
		cfg := &olla.Configuration{}
		cfg.ResponseTimeout, cfg.ReadTimeout, cfg.StreamBufferSize = 5*time.Second, 5*time.Second, 8192
		svc, serr := olla.NewService(discovery, selector, cfg, collector, nil, engineLog)
		require.NoError(t, serr)
		t.Cleanup(svc.Cleanup)
		app.proxyService = svc
	default:
		t.Fatalf("unknown engine %q", engine)
	}

	return &translationPathApp{app: app, handler: app.translationHandler(trans), trans: trans, log: appLog}
}

func (f *translationPathApp) post(t *testing.T, model string, stream bool) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 16,
		"stream":     stream,
		"messages":   []map[string]any{{"role": "user", "content": "hi"}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", bytes.NewReader(b))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.RemoteAddr = "203.0.113.7:4000"
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

// requireTranslatedOK checks the response is a translated Anthropic message (not passthrough).
func requireTranslatedOK(t *testing.T, w *httptest.ResponseRecorder, stream bool) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Empty(t, w.Header().Get(constants.HeaderXOllaMode), "must be translation, not passthrough")
	if stream {
		assert.Contains(t, w.Body.String(), "message_stop")
		return
	}
	var msg map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &msg))
	assert.Equal(t, "message", msg["type"])
}

var (
	translationEngines = []string{"sherpa", "olla"}
	translationModes   = []struct {
		name   string
		strict bool
	}{{"strict", true}, {"optimistic", false}}
	translationStreaming = []struct {
		name   string
		stream bool
	}{{"non-streaming", false}, {"streaming", true}}
)

// A mixed fleet: each model lives on one backend, and translation must reach that backend
// on the chat path its profile declares. Nothing else may be touched.
func TestTranslation_MixedFleetLandsOnEachBackendsChatPath(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		for _, mode := range translationModes {
			for _, st := range translationStreaming {
				t.Run(engine+"/"+mode.name+"/"+st.name, func(t *testing.T) {
					t.Parallel()

					backends := map[string]*pathBackend{
						"docker-model-runner": newPathBackend(t, dmrChatPath),
						"lemonade":            newPathBackend(t, lemonadeChatPath),
						"vllm":                newPathBackend(t, rootChatPath),
						"ollama":              newPathBackend(t, rootChatPath),
					}
					wantPath := map[string]string{
						"docker-model-runner": dmrChatPath,
						"lemonade":            lemonadeChatPath,
						"vllm":                rootChatPath,
						"ollama":              rootChatPath,
					}
					var eps []*domain.Endpoint
					hosts := map[string][]*domain.Endpoint{}
					for typ, b := range backends {
						ep := liveEndpoint(t, typ+"-host", typ, b.URL, 50)
						eps = append(eps, ep)
						hosts["model-"+typ] = []*domain.Endpoint{ep}
					}
					f := newTranslationPathApp(t, engine, mode.strict, false, eps, hosts)

					for typ := range backends {
						w := f.post(t, "model-"+typ, st.stream)
						requireTranslatedOK(t, w, st.stream)
					}
					for typ, b := range backends {
						assert.Equal(t, []string{wantPath[typ]}, b.paths(), "%s must be hit once, on its own chat path", typ)
					}
				})
			}
		}
	}
}

// Each attempt builds its own URL, so failing over between backends that mount the API
// differently must not carry the first backend's path to the second.
func TestTranslation_FailoverUsesEachEndpointsOwnPath(t *testing.T) {
	t.Parallel()

	type pair struct {
		name         string
		deadType     string
		liveType     string
		livePath     string
		liveServes   string
		deadPriority int
	}
	pairs := []pair{
		{"dead DMR to vLLM", "docker-model-runner", "vllm", rootChatPath, rootChatPath, 100},
		{"dead vLLM to DMR", "vllm", "docker-model-runner", dmrChatPath, dmrChatPath, 100},
		{"dead Lemonade to Ollama", "lemonade", "ollama", rootChatPath, rootChatPath, 100},
		{"dead Ollama to Lemonade", "ollama", "lemonade", lemonadeChatPath, lemonadeChatPath, 100},
	}

	for _, engine := range translationEngines {
		for _, mode := range translationModes {
			for _, st := range translationStreaming {
				for _, p := range pairs {
					t.Run(engine+"/"+mode.name+"/"+st.name+"/"+p.name, func(t *testing.T) {
						t.Parallel()

						live := newPathBackend(t, p.liveServes)
						dead := liveEndpoint(t, "dead-host", p.deadType, closedBackendURL(t), p.deadPriority)
						up := liveEndpoint(t, "live-host", p.liveType, live.URL, 50)
						eps := []*domain.Endpoint{up, dead}
						hosts := map[string][]*domain.Endpoint{"shared-model": eps}
						f := newTranslationPathApp(t, engine, mode.strict, false, eps, hosts)

						w := f.post(t, "shared-model", st.stream)

						requireTranslatedOK(t, w, st.stream)
						assert.Equal(t, []string{p.livePath}, live.paths())
					})
				}
			}
		}
	}
}

// Backends with native Anthropic support keep getting /v1/messages passthrough (DMR on its
// own /anthropic prefix), decided by anthropic_support and not by the chat path.
func TestTranslation_PassthroughHostsUnaffected(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		for _, mode := range translationModes {
			for _, st := range translationStreaming {
				t.Run(engine+"/"+mode.name+"/"+st.name, func(t *testing.T) {
					t.Parallel()

					vllm := newPathBackend(t, "/v1/messages", rootChatPath)
					lem := newPathBackend(t, lemonadeChatPath)
					// the higher-priority Lemonade has no native Anthropic support, so passthrough
					// must still pick the capable vLLM rather than translate to Lemonade
					eps := []*domain.Endpoint{
						liveEndpoint(t, "lemonade-host", "lemonade", lem.URL, 100),
						liveEndpoint(t, "vllm-host", "vllm", vllm.URL, 50),
					}
					f := newTranslationPathApp(t, engine, mode.strict, true, eps, map[string][]*domain.Endpoint{"shared-model": eps})

					w := f.post(t, "shared-model", st.stream)

					require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
					assert.Equal(t, string(constants.TranslatorModePassthrough), w.Header().Get(constants.HeaderXOllaMode))
					assert.Equal(t, []string{"/v1/messages"}, vllm.paths())
					assert.Empty(t, lem.paths())
				})
			}
		}
	}

	for _, engine := range translationEngines {
		t.Run(engine+"/DMR keeps its own native path", func(t *testing.T) {
			t.Parallel()

			dmr := newPathBackend(t, "/anthropic/v1/messages")
			eps := []*domain.Endpoint{liveEndpoint(t, "dmr-host", "docker-model-runner", dmr.URL, 100)}
			f := newTranslationPathApp(t, engine, true, true, eps, map[string][]*domain.Endpoint{"shared-model": eps})

			w := f.post(t, "shared-model", false)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, string(constants.TranslatorModePassthrough), w.Header().Get(constants.HeaderXOllaMode))
			assert.Equal(t, []string{"/anthropic/v1/messages"}, dmr.paths())
		})
	}
}

// With passthrough on but declined for this request, the capable backends fall back to
// translation and are still sent to their chat path.
func TestTranslation_PassthroughDeclinedFallsBackToChatPath(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			dmr := newPathBackend(t, dmrChatPath)
			eps := []*domain.Endpoint{liveEndpoint(t, "dmr-host", "docker-model-runner", dmr.URL, 100)}
			f := newTranslationPathApp(t, engine, true, false, eps, map[string][]*domain.Endpoint{"shared-model": eps})

			w := f.post(t, "shared-model", false)

			requireTranslatedOK(t, w, false)
			assert.Equal(t, []string{dmrChatPath}, dmr.paths())
		})
	}
}

// A fleet with no backend that serves a chat path (TEI) is refused with a 404 before
// anything is dispatched, and the refusal goes through the sampled rejection log.
func TestTranslation_NoChatPathIsSampled404(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		for _, mode := range translationModes {
			for _, st := range translationStreaming {
				t.Run(engine+"/"+mode.name+"/"+st.name, func(t *testing.T) {
					t.Parallel()

					tei := newPathBackend(t, "/embed")
					eps := []*domain.Endpoint{liveEndpoint(t, "tei-host", "text-embeddings-inference", tei.URL, 100)}
					f := newTranslationPathApp(t, engine, mode.strict, false, eps, map[string][]*domain.Endpoint{"shared-model": eps})

					const total = 50
					for range total {
						w := f.post(t, "shared-model", st.stream)
						require.Equal(t, http.StatusNotFound, w.Code, "body: %s", w.Body.String())
						assert.Contains(t, w.Body.String(), "not_found_error")
					}

					assert.Empty(t, tei.paths(), "nothing may be dispatched to a backend that cannot serve chat")
					assert.Equal(t, 1, f.log.count("Request rejected"), "repeated refusals are one sampled line")
					assert.Zero(t, f.log.errorTotal(), "no ERROR per refused request")
					assert.Zero(t, f.log.count("No compatible endpoints found for path, falling back to all endpoints"),
						"the unsampled stage-1 widening WARN must not fire per request")
				})
			}
		}
	}
}

// An endpoint whose type has no profile is served as OpenAI-compatible. Scoping translation
// to the known chat profiles must not evict it, even when a model lives only on it.
func TestTranslation_CustomTypeEndpointStillServed(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		for _, mode := range translationModes {
			for _, st := range translationStreaming {
				t.Run(engine+"/"+mode.name+"/"+st.name, func(t *testing.T) {
					t.Parallel()

					custom := newPathBackend(t, rootChatPath)
					vllm := newPathBackend(t, rootChatPath)
					customEP := liveEndpoint(t, "custom-host", "my-custom-engine", custom.URL, 50)
					eps := []*domain.Endpoint{customEP, liveEndpoint(t, "vllm-host", "vllm", vllm.URL, 50)}
					f := newTranslationPathApp(t, engine, mode.strict, false, eps, map[string][]*domain.Endpoint{"custom-only": {customEP}})

					w := f.post(t, "custom-only", st.stream)

					requireTranslatedOK(t, w, st.stream)
					assert.Equal(t, []string{rootChatPath}, custom.paths())
					assert.Empty(t, vllm.paths())
				})
			}
		}
	}
}

// A base path on a preserve_path endpoint is joined onto the request path by the proxy, so the
// resolved chat path must not repeat it (/api + /api/v1/... would 404).
func TestTranslation_PreservePathBaseNotDuplicated(t *testing.T) {
	t.Parallel()

	cases := []struct{ typ, base, chat, wantHit string }{
		{"lemonade", "/api", lemonadeChatPath, lemonadeChatPath},
		{"docker-model-runner", "/engines", dmrChatPath, dmrChatPath},
		{"docker-model-runner", "/engines/llama.cpp", "/engines/llama.cpp/v1/chat/completions", "/engines/llama.cpp/v1/chat/completions"},
		{"docker-model-runner", "/engines/vllm/", "/engines/vllm/v1/chat/completions", "/engines/vllm/v1/chat/completions"},
		{"vllm", "/gw", rootChatPath, "/gw/v1/chat/completions"},
		{"lemonade", "/gw/api", lemonadeChatPath, "/gw/api/v1/chat/completions"},
		{"docker-model-runner", "/gw/engines/llama.cpp", "", "/gw/engines/llama.cpp/v1/chat/completions"},
		{"docker-model-runner", "/gw", "", "/gw/engines/v1/chat/completions"},
	}
	for _, engine := range translationEngines {
		for _, st := range translationStreaming {
			for _, c := range cases {
				t.Run(engine+"/"+st.name+"/"+c.typ, func(t *testing.T) {
					t.Parallel()

					b := newPathBackend(t, c.wantHit)
					ep := liveEndpoint(t, "host", c.typ, b.URL+c.base, 50)
					ep.PreservePath = true
					eps := []*domain.Endpoint{ep}
					f := newTranslationPathApp(t, engine, true, false, eps, map[string][]*domain.Endpoint{"shared-model": eps})

					w := f.post(t, "shared-model", st.stream)

					requireTranslatedOK(t, w, st.stream)
					assert.Equal(t, []string{c.wantHit}, b.paths())
				})
			}
		}
	}
}

// Narrowing to chat-capable profiles must not turn a strict model_unavailable 503 (the model
// exists, but only on a backend that is down) into a 404 when the healthy fleet has no chat path.
func TestTranslation_StrictModelUnavailableStays503(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			tei := newPathBackend(t, "/embed")
			down := liveEndpoint(t, "vllm-down", "vllm", closedBackendURL(t), 50)
			healthy := []*domain.Endpoint{liveEndpoint(t, "tei-host", "text-embeddings-inference", tei.URL, 100)}
			f := newTranslationPathApp(t, engine, true, false, healthy, map[string][]*domain.Endpoint{"shared-model": {down}})

			w := f.post(t, "shared-model", false)

			require.Equal(t, http.StatusServiceUnavailable, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, "model_unavailable", w.Header().Get(constants.HeaderXOllaRoutingReason))
			assert.Empty(t, tei.paths())
		})
	}
}

// count_tokens never selects a backend, so the chat-path scoping must not touch it.
func TestTranslation_CountTokensUnaffected(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			dmr := newPathBackend(t, dmrChatPath)
			lem := newPathBackend(t, lemonadeChatPath)
			eps := []*domain.Endpoint{
				liveEndpoint(t, "dmr-host", "docker-model-runner", dmr.URL, 100),
				liveEndpoint(t, "lemonade-host", "lemonade", lem.URL, 50),
			}
			f := newTranslationPathApp(t, engine, true, true, eps, map[string][]*domain.Endpoint{"shared-model": eps})

			body := `{"model":"shared-model","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages/count_tokens", strings.NewReader(body))
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w := httptest.NewRecorder()
			f.app.tokenCountHandler(f.trans).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Empty(t, dmr.paths())
			assert.Empty(t, lem.paths())
		})
	}
}

func TestTranslationEndpointPaths(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)
	app := &Application{pathInspector: inspector.NewPathInspector(factory, newWarnCounter())}

	mk := func(name, typ string) *domain.Endpoint {
		return liveEndpoint(t, name, typ, "http://"+name+".test:8080", 1)
	}
	vllm, ollama := mk("vllm", "vllm"), mk("ollama", "ollama")
	dmr, lem, tei := mk("dmr", "docker-model-runner"), mk("lem", "lemonade"), mk("tei", "text-embeddings-inference")

	t.Run("uniform root fleet needs no map", func(t *testing.T) {
		t.Parallel()
		usable, paths := app.translationEndpointPaths([]*domain.Endpoint{vllm, ollama}, rootChatPath)
		assert.Len(t, usable, 2)
		assert.Nil(t, paths)
	})

	t.Run("prefixed backends get their own path and chat-less ones are dropped", func(t *testing.T) {
		t.Parallel()
		usable, paths := app.translationEndpointPaths([]*domain.Endpoint{vllm, dmr, lem, tei}, rootChatPath)
		assert.Equal(t, []*domain.Endpoint{vllm, dmr, lem}, usable)
		assert.Equal(t, map[string]string{
			vllm.URLString: rootChatPath,
			dmr.URLString:  dmrChatPath,
			lem.URLString:  lemonadeChatPath,
		}, paths)
	})

	t.Run("no path inspector leaves the list untouched", func(t *testing.T) {
		t.Parallel()
		usable, paths := (&Application{}).translationEndpointPaths([]*domain.Endpoint{tei}, rootChatPath)
		assert.Len(t, usable, 1)
		assert.Nil(t, paths)
	})
}
