package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// Stage 1 narrows candidates to the profiles that declare the request path, so a
// backend that serves a path without declaring it vanishes whenever any other
// profile in the fleet declares it. These tests run the shipped profiles through the
// real inspector chain and unified registry to prove each newly declared host stays
// a candidate, and that widening the sets does not loosen the #236 refusals.

var profilePathStrategies = []struct {
	name string
	cfg  config.ModelRoutingStrategy
}{
	{"strict", config.ModelRoutingStrategy{Type: "strict"}},
	{"optimistic-all", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorAll}}},
}

func offeredNames(eps []*domain.Endpoint) []string {
	names := make([]string, 0, len(eps))
	for _, ep := range eps {
		names = append(names, ep.Name)
	}
	return names
}

const (
	openAIModelsOnlyHost = `{"data":[{"id":"host-only-model","object":"model"}]}`
	hostOnlyBody         = `{"model":"host-only-model","input":"hi","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"query":"q","documents":["a"]}`
)

// Ollama declares /v1/responses and /v1/messages, so any other host that serves them
// has to declare them too or a request naming its model finds no candidate.
func TestProxyRouting_DeclaredPaths_NamedModelReachesServingHost(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	cases := []struct {
		typ   string
		paths []string
	}{
		{domain.ProfileOpenAICompatible, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages"}},
		{domain.ProfileVLLM, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens"}},
		{domain.ProfileVLLMMLX, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{domain.ProfileLlamaCpp, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{domain.ProfileSGLang, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{"lmdeploy", []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens"}},
		{domain.ProfileLmStudio, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages"}},
		{"litellm", []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens"}},
		{domain.ProfileLemonade, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages"}},
		{"docker-model-runner", []string{"/olla/proxy/v1/responses", "/olla/proxy/anthropic/v1/messages", "/olla/proxy/anthropic/v1/messages/count_tokens"}},
	}

	for _, tc := range cases {
		for _, path := range tc.paths {
			for _, strategy := range profilePathStrategies {
				t.Run(tc.typ+path+"/"+strategy.name, func(t *testing.T) {
					t.Parallel()
					// two declaring neighbours, so the target is only kept if it declares the path itself
					fleet := []fleetEndpoint{
						{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsPlainChat},
						{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel},
						{name: "target-1", typ: tc.typ, modelsJSON: openAIModelsOnlyHost},
					}
					w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, fleet, path, hostOnlyBody)

					require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
					assert.Equal(t, []string{"target-1"}, offeredNames(selected))
				})
			}
		}
	}
}

// Ollama serves /api/ps and /api/version itself; declaring them narrows a proxy call
// to Ollama hosts instead of spraying an Ollama-native route at vLLM and friends.
func TestProxyRouting_OllamaNativePathsNarrowToOllama(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	for _, path := range []string{"/olla/proxy/api/ps", "/olla/proxy/api/version"} {
		for _, strategy := range profilePathStrategies {
			t.Run(strings.TrimPrefix(path, "/olla/proxy")+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()
				fleet := []fleetEndpoint{
					{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsPlainChat},
					{name: "ollama-2", typ: domain.ProfileOllama, modelsJSON: `{"models":[{"name":"qwen3:8b","capabilities":["completion"]}]}`},
					{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel},
				}
				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, fleet, path, `{}`)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				assert.ElementsMatch(t, []string{"ollama-1", "ollama-2"}, offeredNames(selected))
			})
		}
	}
}

// Ollama and LM Studio are implicitly compatible with openai-compatible's paths, so a
// path declared there makes them candidates. Ollama and LM Studio do not serve
// /v1/rerank or /v1/messages/count_tokens (both 404), hence the generic profile does not
// declare them and a model-less request stays with the hosts that do.
func TestProxyRouting_DeclaredPaths_GenericProfileDoesNotAdmitNonServingHosts(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	cases := []struct {
		path, body string
		want       []string
	}{
		{"/olla/proxy/v1/rerank", `{"query":"q","documents":["a"]}`, []string{"vllm-1"}},
		{"/olla/proxy/v1/messages/count_tokens", `{"messages":[{"role":"user","content":"hi"}]}`, []string{"vllm-1"}},
	}
	for _, tc := range cases {
		for _, strategy := range profilePathStrategies {
			t.Run(tc.path+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()
				fleet := []fleetEndpoint{
					{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsPlainChat},
					{name: "lmstudio-1", typ: domain.ProfileLmStudio, modelsJSON: lmStudioNomic},
					{name: "oai-1", typ: domain.ProfileOpenAICompatible, modelsJSON: openAIModelsOnlyHost},
					{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel},
				}
				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, fleet, tc.path, tc.body)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				assert.Equal(t, tc.want, offeredNames(selected))
			})
		}
	}
}

// Ollama and vLLM both declare /v1/messages, so a model-less vision request on that
// path now has a wider candidate set. The #236 rules must hold over
// it: the vision host gets the request while it is up, and once it is down the request
// is refused with capability_unavailable rather than sprayed at the chat-only host.
func TestProxyRouting_DeclaredPaths_CapabilityRefusalOverWiderCandidates(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	const (
		anthropicVision = `{"max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]}`
	)
	paths := []struct{ path, body string }{
		{"/olla/proxy/v1/messages", anthropicVision},
	}

	for _, tc := range paths {
		for _, strategy := range profilePathStrategies {
			t.Run(tc.path+"/"+strategy.name+"/vision host up", func(t *testing.T) {
				t.Parallel()
				fleet := []fleetEndpoint{
					{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsEmbedAndVision},
					{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel},
				}
				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, fleet, tc.path, tc.body)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				assert.Equal(t, []string{"ollama-1"}, offeredNames(selected))
			})

			t.Run(tc.path+"/"+strategy.name+"/vision host down is refused", func(t *testing.T) {
				t.Parallel()
				fleet := []fleetEndpoint{
					{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsEmbedAndVision, offline: true},
					{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel},
				}
				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, fleet, tc.path, tc.body)

				require.Equal(t, http.StatusServiceUnavailable, w.Code, "body: %s", w.Body.String())
				assert.Empty(t, selected, "a refused request never reaches the proxy engine")
				assert.Equal(t, constants.RoutingReasonCapabilityUnavailable, w.Header().Get(constants.HeaderXOllaRoutingReason))
				assert.Equal(t, ports.RoutingActionRejected, w.Header().Get(constants.HeaderXOllaRoutingDecision))
			})
		}
	}
}

// /olla/anthropic/v1/messages is Olla's own route. The path inspector suffix-matches it
// against profiles that declare /v1/messages, so once more profiles declare it the
// backends that don't (Lemonade, LiteLLM) were evicted from translation. Naming a model
// hosted only on such a backend must still select it, under strict routing (which
// would 503) and under the optimistic fallback (which would send it to Ollama).
func TestTranslationHandler_DeclaredPaths_DoesNotEvictUndeclaringBackends(t *testing.T) {
	t.Parallel()

	for _, typ := range []string{"lemonade", "litellm", domain.ProfileVLLM, domain.ProfileLlamaCpp, domain.ProfileLmStudio, domain.ProfileOpenAICompatible} {
		for _, strict := range []bool{true, false} {
			name := typ + "/optimistic"
			if strict {
				name = typ + "/strict"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				factory, err := profile.NewFactory("../../../config/profiles")
				require.NoError(t, err)
				log := &mockStyledLogger{}
				pathInspector := inspector.NewPathInspector(factory, log)
				chain := inspector.NewChain(log)
				chain.AddInspector(pathInspector)
				bodyInspector, err := inspector.NewBodyInspector(log)
				require.NoError(t, err)
				chain.AddInspector(bodyInspector)
				trans, err := anthropic.NewTranslator(log, config.AnthropicTranslatorConfig{PassthroughEnabled: false})
				require.NoError(t, err)

				mk := func(name, typ string) *domain.Endpoint {
					raw := "http://" + name + ".test:8080"
					u, _ := url.Parse(raw)
					return &domain.Endpoint{Name: name, Type: typ, URL: u, URLString: raw, Status: domain.StatusHealthy}
				}
				fleet := []*domain.Endpoint{mk("ollama-1", domain.ProfileOllama), mk("target-1", typ)}

				var offered []string
				app := &Application{
					Config:         &config.Config{},
					logger:         log,
					inspectorChain: chain,
					pathInspector:  pathInspector,
					profileFactory: factory,
					profileLookup:  factory,
					discoveryService: &mockDiscoveryServiceWithFunc{getHealthyEndpointsFunc: func(context.Context) ([]*domain.Endpoint, error) {
						return fleet, nil
					}},
					modelRegistry:  &mockSimpleModelRegistry{strict: strict, endpointsForModel: map[string][]string{"host-only-model": {fleet[1].URLString}}},
					statsCollector: &mockStatsCollector{},
					proxyService: &mockProxyService{proxyFunc: func(_ context.Context, w http.ResponseWriter, _ *http.Request, eps []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
						offered = offeredNames(eps)
						w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
						return nil
					}},
				}

				body := `{"model":"host-only-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
				req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(body))
				req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
				w := httptest.NewRecorder()
				app.translationHandler(trans).ServeHTTP(w, req)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				assert.Equal(t, []string{"target-1"}, offered)
			})
		}
	}
}
