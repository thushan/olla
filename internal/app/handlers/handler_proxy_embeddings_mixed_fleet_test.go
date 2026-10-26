package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// fleetEndpoint describes one backend in a routing-matrix fleet. modelsJSON is
// fed through the shipped profile's real parser, so typing and tagging come from
// the same code path production discovery uses.
type fleetEndpoint struct {
	name       string
	typ        string
	modelsJSON string
	// offline hosts keep their discovered models, as a real endpoint that has
	// stopped answering health checks does, but are not routable
	offline bool
	// busy hosts are left out of the healthy list, as GetHealthyEndpoints does
	busy bool
}

const (
	ollamaTagsEmbedAndVision = `{"models":[
		{"name":"all-minilm:latest","details":{"family":"bert","parameter_size":"23M"},"capabilities":["embedding"]},
		{"name":"llava:13b","details":{"family":"llava"},"capabilities":["completion","vision"]}]}`
	ollamaTagsPlainChat = `{"models":[{"name":"llama3.1:8b","capabilities":["completion"]}]}`
	vllmChatModel       = `{"data":[{"id":"Qwen/Qwen3-32B","object":"model"}]}`
	teiInfoBGESmall     = `{"model_id":"BAAI/bge-small-en-v1.5","model_type":{"embedding":{"pooling":"cls"}}}`
	vllmBGE             = `{"data":[{"id":"BAAI/bge-m3","object":"model"}]}`
	vllmQwenVL          = `{"data":[{"id":"Qwen/Qwen2.5-VL-7B-Instruct","object":"model"}]}`
	lmStudioNomic       = `{"data":[{"id":"text-embedding-nomic-embed-text-v1.5","object":"model","type":"embeddings","publisher":"nomic-ai","arch":"nomic-bert","compatibility_type":"gguf","quantization":"Q4_K_M","state":"not-loaded","max_context_length":2048}]}`
	teiInfoReranker     = `{"model_id":"/data/bge-reranker-base","served_model_name":"BAAI/bge-reranker-base","model_type":{"reranker":{}}}`
	teiInfoClassify     = `{"model_id":"/data/roberta-go-emotions","served_model_name":"SamLowe/roberta-base-go_emotions","model_type":{"classifier":{"id2label":{"0":"joy"}}}}`
	teiInfoSnowflake    = `{"model_id":"Snowflake/snowflake-arctic-embed-l-v2.0","model_type":{"embedding":{"pooling":"cls"}}}`
)

// TestProxyRouting_MixedFleetMatrix guards the regression where the capability
// stage filtered endpoints to those hosting ANY model carrying the required tag,
// regardless of which model the caller named. A tag on one backend's model then
// evicted every other backend, so strict routing 503ed (or optimistic fallback
// misrouted) a perfectly hosted, untagged model. A named model must reach a host
// of that model; model-less requests are still steered by capability.
func TestProxyRouting_MixedFleetMatrix(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	const (
		embeddingsPath = "/olla/proxy/v1/embeddings"
		chatPath       = "/olla/proxy/v1/chat/completions"
		visionContent  = `[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]`
	)
	embedBody := func(model string) string {
		if model == "" {
			return `{"input":"hello"}`
		}
		return `{"model":"` + model + `","input":"hello"}`
	}
	const (
		rerankBody = `{"query":"what is deep learning","texts":["deep learning is a subset of ml","paris is in france"]}`
		inputsBody = `{"inputs":"hello"}`
	)
	visionBody := func(model string) string {
		if model == "" {
			return `{"messages":[{"role":"user","content":` + visionContent + `}]}`
		}
		return `{"model":"` + model + `","messages":[{"role":"user","content":` + visionContent + `}]}`
	}

	ollamaEmbed := fleetEndpoint{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: ollamaTagsEmbedAndVision}
	vllmEmbed := fleetEndpoint{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmBGE}
	vllmVision := fleetEndpoint{name: "vllm-1", typ: domain.ProfileVLLM, modelsJSON: vllmQwenVL}
	tei := fleetEndpoint{name: "tei-1", typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: teiInfoSnowflake}
	teiEmbed := fleetEndpoint{name: "tei-embed", typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: teiInfoSnowflake}
	teiRerank := fleetEndpoint{name: "tei-rerank", typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: teiInfoReranker}
	teiClassify := fleetEndpoint{name: "tei-classify", typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: teiInfoClassify}
	teiTrio := []fleetEndpoint{teiEmbed, teiRerank, teiClassify}
	lmStudio := fleetEndpoint{name: "lmstudio-1", typ: constants.ProviderTypeLMStudio, modelsJSON: lmStudioNomic}

	tests := []struct {
		name         string
		path         string
		body         string
		wantEndpoint string
		fleet        []fleetEndpoint
	}{
		{"ollama-tagged embedding reaches ollama beside vllm", embeddingsPath, embedBody("all-minilm:latest"), "ollama-1", []fleetEndpoint{ollamaEmbed, vllmEmbed}},
		{"untagged vllm embedding reaches vllm beside tagged ollama", embeddingsPath, embedBody("BAAI/bge-m3"), "vllm-1", []fleetEndpoint{ollamaEmbed, vllmEmbed}},
		{"ollama-tagged embedding reaches ollama beside tei", embeddingsPath, embedBody("all-minilm:latest"), "ollama-1", []fleetEndpoint{ollamaEmbed, tei}},
		{"tei model reaches tei beside tagged ollama", embeddingsPath, embedBody("Snowflake/snowflake-arctic-embed-l-v2.0"), "tei-1", []fleetEndpoint{ollamaEmbed, tei}},
		{"tei model reaches tei beside vllm", embeddingsPath, embedBody("Snowflake/snowflake-arctic-embed-l-v2.0"), "tei-1", []fleetEndpoint{tei, vllmEmbed}},
		{"vllm embedding reaches vllm beside tei", embeddingsPath, embedBody("BAAI/bge-m3"), "vllm-1", []fleetEndpoint{tei, vllmEmbed}},
		{"lm studio embedding reaches lm studio beside vllm", embeddingsPath, embedBody("text-embedding-nomic-embed-text-v1.5"), "lmstudio-1", []fleetEndpoint{lmStudio, vllmEmbed}},
		{"vllm embedding reaches vllm beside lm studio", embeddingsPath, embedBody("BAAI/bge-m3"), "vllm-1", []fleetEndpoint{lmStudio, vllmEmbed}},
		{
			"hybrid ollama embedding model beside tei reaches ollama", embeddingsPath, embedBody("embeddinggemma:latest"), "ollama-1",
			[]fleetEndpoint{{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: `{"models":[{"name":"embeddinggemma:latest","capabilities":["embedding","completion"]}]}`}, tei},
		},
		{
			"capability-less old ollama embedding model beside tei reaches ollama", embeddingsPath, embedBody("nomic-embed-text:latest"), "ollama-1",
			[]fleetEndpoint{{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: `{"models":[{"name":"nomic-embed-text:latest"}]}`}, tei},
		},
		{"untagged vision model reaches its host beside llava host", chatPath, visionBody("Qwen/Qwen2.5-VL-7B-Instruct"), "vllm-1", []fleetEndpoint{ollamaEmbed, vllmVision}},
		{"llava reaches ollama beside untagged vision host", chatPath, visionBody("llava:13b"), "ollama-1", []fleetEndpoint{ollamaEmbed, vllmVision}},
		// TEI native bodies carry no model field: the path picks the kind. /predict is
		// served by classifiers and rerankers alike, so those fleets omit the reranker.
		{"model-less rerank goes to the reranker", "/olla/proxy/rerank", rerankBody, "tei-rerank", teiTrio},
		{"model-less predict goes to the classifier", "/olla/proxy/predict", inputsBody, "tei-classify", []fleetEndpoint{teiEmbed, teiClassify}},
		// TEI /predict also serves rerankers, so an embedder + reranker fleet must not fall back to the embedder (424)
		{"model-less predict reaches the reranker beside an embedder", "/olla/proxy/predict", inputsBody, "tei-rerank", []fleetEndpoint{teiEmbed, teiRerank}},
		{"model-less embed goes to the embedder", "/olla/proxy/embed", inputsBody, "tei-embed", teiTrio},
		{"model-less embed_all goes to the embedder", "/olla/proxy/embed_all", inputsBody, "tei-embed", teiTrio},
		{"tei provider route rerank goes to the reranker", "/olla/tei/rerank", rerankBody, "tei-rerank", teiTrio},
		{"tei provider route predict goes to the classifier", "/olla/tei/predict", inputsBody, "tei-classify", []fleetEndpoint{teiEmbed, teiClassify}},
		{"named embedding model reaches the embedder among tei kinds", embeddingsPath, embedBody("Snowflake/snowflake-arctic-embed-l-v2.0"), "tei-embed", teiTrio},
		{"model-less rerank beside vllm still reaches the reranker", "/olla/proxy/rerank", rerankBody, "tei-rerank", []fleetEndpoint{vllmEmbed, teiEmbed, teiRerank}},
		{"model-less embeddings steered to a real lm studio embedding model beside vllm", embeddingsPath, embedBody(""), "lmstudio-1", []fleetEndpoint{lmStudio, vllmEmbed}},
		{"model-less embeddings steered to tagged embedding host", embeddingsPath, embedBody(""), "ollama-1", []fleetEndpoint{ollamaEmbed, vllmEmbed}},
		{"model-less vision steered to vision-tagged host", chatPath, visionBody(""), "ollama-1", []fleetEndpoint{ollamaEmbed, vllmVision}},
	}

	strategies := []struct {
		name string
		cfg  config.ModelRoutingStrategy
	}{
		{"strict", config.ModelRoutingStrategy{Type: "strict"}},
		{"optimistic-compatible-only", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorCompatibleOnly}}},
		{"optimistic-all", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorAll}}},
	}

	for _, tt := range tests {
		for _, strategy := range strategies {
			t.Run(tt.name+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()

				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, tt.fleet, tt.path, tt.body)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				require.NotEmpty(t, selected, "proxy engine must receive candidate endpoints")
				for _, ep := range selected {
					assert.Equal(t, tt.wantEndpoint, ep.Name)
				}
			})
		}
	}
}

// runMixedFleet registers fleet in a real unified registry through the shipped profile
// parsers, sends one request through the proxy handler and returns the response and the
// endpoints the proxy engine was handed. Offline hosts keep their discovered models.
func runMixedFleet(t *testing.T, profileFactory profile.ProfileFactory, chain *inspector.Chain, styledLog *mockStyledLogger,
	strategyCfg config.ModelRoutingStrategy, fleet []fleetEndpoint, path, body string) (*httptest.ResponseRecorder, []*domain.Endpoint) {
	t.Helper()

	ctx := context.Background()
	cfg := strategyCfg
	unified := registry.NewUnifiedMemoryModelRegistry(styledLog, nil, &cfg, nil)

	healthy := make([]*domain.Endpoint, 0, len(fleet))
	all := make([]*domain.Endpoint, 0, len(fleet))
	wantModels := 0
	for _, fe := range fleet {
		u, perr := url.Parse("http://" + fe.name + ":8080")
		require.NoError(t, perr)
		ep := &domain.Endpoint{Name: fe.name, URL: u, URLString: u.String(), Type: fe.typ, Status: domain.StatusHealthy}
		all = append(all, ep)
		switch {
		case fe.offline:
			ep.Status = domain.StatusOffline
		case fe.busy:
			ep.Status = domain.StatusBusy
		default:
			healthy = append(healthy, ep)
		}

		prof, perr := profileFactory.GetProfile(fe.typ)
		require.NoError(t, perr)
		models, perr := prof.ParseModelsResponse([]byte(fe.modelsJSON))
		require.NoError(t, perr)
		wantModels += len(models)

		unified.RegisterEndpoint(ep)
		require.NoError(t, unified.RegisterModelsWithEndpoint(ctx, ep, models))
	}
	require.Eventually(t, func() bool {
		models, gerr := unified.GetUnifiedModels(ctx)
		return gerr == nil && len(models) >= wantModels
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	var selected []*domain.Endpoint
	app := &Application{
		Config:         &config.Config{Server: config.ServerConfig{RateLimits: config.ServerRateLimits{}}},
		logger:         styledLog,
		profileFactory: profileFactory,
		inspectorChain: chain,
		discoveryService: &mockDiscoveryServiceWithFunc{getHealthyEndpointsFunc: func(context.Context) ([]*domain.Endpoint, error) { return healthy, nil },
			getEndpointsFunc: func(context.Context) ([]*domain.Endpoint, error) { return all, nil },
		},
		modelRegistry: unified,
		proxyService: &mockProxyService{
			proxyFunc: func(_ context.Context, w http.ResponseWriter, _ *http.Request, eps []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
				selected = eps
				w.WriteHeader(http.StatusOK)
				return nil
			},
		},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if strings.HasPrefix(path, "/olla/proxy/") {
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
	}
	w := httptest.NewRecorder()

	if strings.HasPrefix(path, "/olla/tei/") {
		app.providerProxyHandler(w, req)
	} else {
		app.proxyHandler(w, req)
	}

	return w, selected
}

// mixedFleetDeps builds the shipped profiles and the path/body inspector chain the
// fleet tests route through.
func mixedFleetDeps(t *testing.T) (profile.ProfileFactory, *inspector.Chain, *mockStyledLogger) {
	t.Helper()
	styledLog := &mockStyledLogger{}

	profileFactory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	inspectorFactory := inspector.NewFactory(profileFactory, styledLog)
	bodyInspector, err := inspectorFactory.CreateBodyInspector()
	require.NoError(t, err)
	chain := inspectorFactory.CreateChain()
	chain.AddInspector(inspectorFactory.CreatePathInspector())
	chain.AddInspector(bodyInspector)
	return profileFactory, chain, styledLog
}

// When a capability exists in the fleet but every host with it is down, a model-less
// request is refused with a clear 503. Falling back to "everything" sprayed it at hosts
// that serve a different kind: with the only embedder stopped, /embed went to the
// reranker and classifier and TEI answered 424 for most of 1500 requests.
func TestProxyRouting_CapabilityUnavailable(t *testing.T) {
	t.Parallel()

	profileFactory, chain, styledLog := mixedFleetDeps(t)

	const (
		embeddingsPath = "/olla/proxy/v1/embeddings"
		chatPath       = "/olla/proxy/v1/chat/completions"
		inputsBody     = `{"inputs":"hello"}`
		embedBody      = `{"input":"hello"}`
		visionBody     = `{"messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`
		plainChatBody  = `{"messages":[{"role":"user","content":"hi"}]}`
	)
	tei := func(name, info string, offline bool) fleetEndpoint {
		return fleetEndpoint{name: name, typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: info, offline: offline}
	}
	ollama := func(models string, offline bool) fleetEndpoint {
		return fleetEndpoint{name: "ollama-1", typ: domain.ProfileOllama, modelsJSON: models, offline: offline}
	}
	teiBusy := fleetEndpoint{name: "tei-embed", typ: constants.ProviderTypeTextEmbeddingsInference, modelsJSON: teiInfoSnowflake, busy: true}
	vllmChat := fleetEndpoint{name: "vllm-chat", typ: domain.ProfileVLLM, modelsJSON: vllmChatModel}

	tests := []struct {
		name        string
		path        string
		body        string
		wantReason  string   // set when the request must be refused with a 503
		wantOffered []string // otherwise the hosts the proxy engine is handed
		fleet       []fleetEndpoint
	}{
		{"model-less embed with the only embedder down", "/olla/proxy/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		{"model-less embed on the tei provider route with the only embedder down", "/olla/tei/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		{"model-less embeddings with the only embedding host down", embeddingsPath, embedBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{ollama(ollamaTagsEmbedAndVision, true), vllmChat}},
		{"model-less vision with the only vision host down", chatPath, visionBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{ollama(ollamaTagsEmbedAndVision, true), vllmChat}},
		{"model-less embed on the tei provider route with every tei host offline", "/olla/tei/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, true), vllmChat}},
		// GetHealthyEndpoints excludes Busy hosts, so they never receive proxied traffic and count as down
		{"model-less embed with the only embedder busy", "/olla/proxy/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{teiBusy, tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		{"model-less embed on the tei provider route with the only embedder busy", "/olla/tei/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{teiBusy, tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		// a healthy embedder of a type the path can't use (Ollama for TEI's /embed) must not veto the refusal
		{"model-less embed with the tei embedder down and a healthy ollama embedder", "/olla/proxy/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false), ollama(ollamaTagsEmbedAndVision, false)}},
		{"model-less embed on the tei provider route with the tei embedder down and a healthy ollama embedder", "/olla/tei/embed", inputsBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false), ollama(ollamaTagsEmbedAndVision, false)}},
		// /v1/embeddings is shared with Ollama, but the tei provider route confines candidates to TEI
		{"model-less v1 embeddings on the tei provider route with a healthy ollama embedder", "/olla/tei/v1/embeddings", embedBody, constants.RoutingReasonCapabilityUnavailable, nil,
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false), ollama(ollamaTagsEmbedAndVision, false)}},
		{"model-less v1 embeddings on the unified route reaches the ollama embedder", embeddingsPath, embedBody, "", []string{"ollama-1"},
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false), ollama(ollamaTagsEmbedAndVision, false)}},
		{"model-less embed on the tei provider route whose only embedder is a healthy ollama keeps the fallback", "/olla/tei/embed", inputsBody, "", []string{"tei-classify", "tei-rerank"},
			[]fleetEndpoint{tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false), ollama(ollamaTagsEmbedAndVision, false)}},
		{"model-less embed routes to a second embedder that is up", "/olla/proxy/embed", inputsBody, "", []string{"tei-embed-2"},
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), tei("tei-embed-2", teiInfoBGESmall, false), tei("tei-rerank", teiInfoReranker, false)}},
		{"model-less embed with no embedder model anywhere keeps the fallback", "/olla/proxy/embed", inputsBody, "", []string{"tei-classify", "tei-rerank"},
			[]fleetEndpoint{tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		{"model-less embed whose embedder is up but not a candidate keeps the fallback", "/olla/proxy/embed", inputsBody, "", []string{"tei-classify", "tei-rerank"},
			[]fleetEndpoint{ollama(ollamaTagsEmbedAndVision, false), tei("tei-rerank", teiInfoReranker, false), tei("tei-classify", teiInfoClassify, false)}},
		{"model-less chat on an untagged fleet is untouched", chatPath, plainChatBody, "", []string{"ollama-1", "vllm-chat"},
			[]fleetEndpoint{ollama(ollamaTagsPlainChat, false), vllmChat}},
		{"a down host without the capability changes nothing", chatPath, plainChatBody, "", []string{"vllm-chat"},
			[]fleetEndpoint{ollama(ollamaTagsEmbedAndVision, true), vllmChat}},
		{"a named request skips the capability check when the embedder is down", embeddingsPath, `{"model":"Qwen/Qwen3-32B","input":"hello"}`, "", []string{"vllm-chat"},
			[]fleetEndpoint{tei("tei-embed", teiInfoSnowflake, true), vllmChat}},
	}

	strategies := []struct {
		name string
		cfg  config.ModelRoutingStrategy
	}{
		{"strict", config.ModelRoutingStrategy{Type: "strict"}},
		{"optimistic-compatible-only", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorCompatibleOnly}}},
		{"optimistic-all", config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorAll}}},
	}

	for _, tt := range tests {
		for _, strategy := range strategies {
			t.Run(tt.name+"/"+strategy.name, func(t *testing.T) {
				t.Parallel()

				w, selected := runMixedFleet(t, profileFactory, chain, styledLog, strategy.cfg, tt.fleet, tt.path, tt.body)

				if tt.wantReason != "" {
					require.Equal(t, http.StatusServiceUnavailable, w.Code, "body: %s", w.Body.String())
					assert.Empty(t, selected, "a refused request never reaches the proxy engine")
					assert.Equal(t, tt.wantReason, w.Header().Get(constants.HeaderXOllaRoutingReason))
					assert.Equal(t, constants.RoutingStrategyCapability, w.Header().Get(constants.HeaderXOllaRoutingStrategy))
					assert.Equal(t, ports.RoutingActionRejected, w.Header().Get(constants.HeaderXOllaRoutingDecision))
					assert.Contains(t, w.Body.String(), tt.wantReason)
					return
				}

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				got := make([]string, 0, len(selected))
				for _, ep := range selected {
					got = append(got, ep.Name)
				}
				assert.ElementsMatch(t, tt.wantOffered, got)
			})
		}
	}
}
