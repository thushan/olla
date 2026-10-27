package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
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
		{domain.ProfileOpenAICompatible, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{domain.ProfileVLLM, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens"}},
		{domain.ProfileVLLMMLX, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{domain.ProfileLlamaCpp, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{domain.ProfileSGLang, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens", "/olla/proxy/v1/rerank"}},
		{"lmdeploy", []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages", "/olla/proxy/v1/messages/count_tokens"}},
		{domain.ProfileLmStudio, []string{"/olla/proxy/v1/responses", "/olla/proxy/v1/messages"}},
		{"litellm", []string{"/olla/proxy/v1/responses"}},
		{domain.ProfileLemonade, []string{"/olla/proxy/v1/responses"}},
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

// Ollama and vLLM both declare /v1/messages and /v1/responses, so a model-less vision
// request on those paths now has a wider candidate set. The #236 rules must hold over
// it: the vision host gets the request while it is up, and once it is down the request
// is refused with capability_unavailable rather than sprayed at the chat-only host.
func TestProxyRouting_DeclaredPaths_CapabilityRefusalOverWiderCandidates(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	const (
		anthropicVision = `{"max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]}`
		responsesVision = `{"input":[{"role":"user","content":[{"type":"input_text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`
	)
	paths := []struct{ path, body string }{
		{"/olla/proxy/v1/messages", anthropicVision},
		{"/olla/proxy/v1/responses", responsesVision},
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
