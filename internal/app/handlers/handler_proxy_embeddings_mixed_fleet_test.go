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
//
// models bypasses the parser for fleets that need a tag the parser doesn't
// currently surface (see lmStudioEmbed).
type fleetEndpoint struct {
	name       string
	typ        string
	modelsJSON string
	models     []*domain.ModelInfo
}

const (
	ollamaTagsEmbedAndVision = `{"models":[
		{"name":"all-minilm:latest","details":{"family":"bert","parameter_size":"23M"},"capabilities":["embedding"]},
		{"name":"llava:13b","details":{"family":"llava"},"capabilities":["completion","vision"]}]}`
	vllmBGE          = `{"data":[{"id":"BAAI/bge-m3","object":"model"}]}`
	vllmQwenVL       = `{"data":[{"id":"Qwen/Qwen2.5-VL-7B-Instruct","object":"model"}]}`
	teiInfoSnowflake = `{"model_id":"Snowflake/snowflake-arctic-embed-l-v2.0","model_type":{"embedding":{"pooling":"cls"}}}`
)

// lmStudioEmbed is registered as a prebuilt ModelInfo with the top-level
// embeddings type. The real LM Studio parser puts "embeddings" in Details.Type
// but sets the top-level Type to "model", and the unifier lets the latter mask
// the former, so a parsed LM Studio embedding model is currently untagged. This
// reproduces the tagged shape the routing bug needs without relying on that.
var lmStudioEmbed = []*domain.ModelInfo{{Name: "text-embedding-nomic-embed-text-v1.5", Type: "embeddings"}}

// TestProxyRouting_MixedFleetMatrix guards the regression where the capability
// stage filtered endpoints to those hosting ANY model carrying the required tag,
// regardless of which model the caller named. A tag on one backend's model then
// evicted every other backend, so strict routing 503ed (or optimistic fallback
// misrouted) a perfectly hosted, untagged model. A named model must reach a host
// of that model; model-less requests are still steered by capability.
func TestProxyRouting_MixedFleetMatrix(t *testing.T) {
	t.Parallel()

	styledLog := &mockStyledLogger{}

	profileFactory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	inspectorFactory := inspector.NewFactory(profileFactory, styledLog)
	bodyInspector, err := inspectorFactory.CreateBodyInspector()
	require.NoError(t, err)
	chain := inspectorFactory.CreateChain()
	chain.AddInspector(inspectorFactory.CreatePathInspector())
	chain.AddInspector(bodyInspector)

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
	lmStudio := fleetEndpoint{name: "lmstudio-1", typ: constants.ProviderTypeLMStudio, models: lmStudioEmbed}

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
		for _, strat := range strategies {
			t.Run(tt.name+"/"+strat.name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				cfg := strat.cfg
				unified := registry.NewUnifiedMemoryModelRegistry(styledLog, nil, &cfg, nil)

				endpoints := make([]*domain.Endpoint, 0, len(tt.fleet))
				wantModels := 0
				for _, fe := range tt.fleet {
					u, perr := url.Parse("http://" + fe.name + ":8080")
					require.NoError(t, perr)
					ep := &domain.Endpoint{Name: fe.name, URL: u, URLString: u.String(), Type: fe.typ, Status: domain.StatusHealthy}
					endpoints = append(endpoints, ep)

					models := fe.models
					if models == nil {
						prof, perr := profileFactory.GetProfile(fe.typ)
						require.NoError(t, perr)
						models, perr = prof.ParseModelsResponse([]byte(fe.modelsJSON))
						require.NoError(t, perr)
					}
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
					Config:           &config.Config{Server: config.ServerConfig{RateLimits: config.ServerRateLimits{}}},
					logger:           styledLog,
					profileFactory:   profileFactory,
					inspectorChain:   chain,
					discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: endpoints},
					modelRegistry:    unified,
					proxyService: &mockProxyService{
						proxyFunc: func(_ context.Context, w http.ResponseWriter, _ *http.Request, eps []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
							selected = eps
							w.WriteHeader(http.StatusOK)
							return nil
						},
					},
					StartTime: time.Now(),
				}

				req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
				req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
				w := httptest.NewRecorder()

				app.proxyHandler(w, req)

				require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
				require.NotEmpty(t, selected, "proxy engine must receive candidate endpoints")
				for _, ep := range selected {
					assert.Equal(t, tt.wantEndpoint, ep.Name)
				}
			})
		}
	}
}
