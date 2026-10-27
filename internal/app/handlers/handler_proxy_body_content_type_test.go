package handlers

import (
	"context"
	"io"
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

// A JSON body must route by its model whatever Content-Type the client sent. curl -d sends
// form-urlencoded, fetch() with a string body sends text/plain and many scripts send nothing.
// With optimistic/all fallback a model-less request lands on any compatible host, so the
// assertion that only the hosting endpoint is selected catches the header gate.
func TestProxyRouting_NamedModelIgnoresContentType(t *testing.T) {
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

	const body = `{"model":"nomic-embed-text:latest","input":"hi"}`
	contentTypes := map[string]string{
		"json":            constants.ContentTypeJSON,
		"form-urlencoded": "application/x-www-form-urlencoded",
		"text/plain":      "text/plain;charset=UTF-8",
		"vnd.api+json":    "application/vnd.api+json",
		"x-ndjson":        "application/x-ndjson",
		"missing":         "",
	}

	for name, ct := range contentTypes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			cfg := config.ModelRoutingStrategy{Type: "optimistic", Options: config.ModelRoutingStrategyOptions{FallbackBehavior: constants.FallbackBehaviorAll}}
			unified := registry.NewUnifiedMemoryModelRegistry(styledLog, nil, &cfg, nil)

			fleet := []fleetEndpoint{
				{name: "ollama-has", typ: domain.ProfileOllama, modelsJSON: `{"models":[{"name":"nomic-embed-text:latest"}]}`},
				{name: "ollama-other", typ: domain.ProfileOllama, modelsJSON: `{"models":[{"name":"llama3:latest"}]}`},
				{name: "vllm-other", typ: domain.ProfileVLLM, modelsJSON: vllmQwenVL},
			}
			endpoints := make([]*domain.Endpoint, 0, len(fleet))
			wantModels := 0
			for _, fe := range fleet {
				u, perr := url.Parse("http://" + fe.name + ":8080")
				require.NoError(t, perr)
				ep := &domain.Endpoint{Name: fe.name, URL: u, URLString: u.String(), Type: fe.typ, Status: domain.StatusHealthy}
				endpoints = append(endpoints, ep)

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
			}, 5*time.Second, 10*time.Millisecond)

			var selected []*domain.Endpoint
			var forwarded []byte
			app := &Application{
				Config:           &config.Config{Server: config.ServerConfig{RateLimits: config.ServerRateLimits{}}},
				logger:           styledLog,
				profileFactory:   profileFactory,
				inspectorChain:   chain,
				discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: endpoints},
				modelRegistry:    unified,
				proxyService: &mockProxyService{
					proxyFunc: func(_ context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
						selected = eps
						forwarded, _ = io.ReadAll(r.Body)
						w.WriteHeader(http.StatusOK)
						return nil
					},
				},
				StartTime: time.Now(),
			}

			req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/embeddings", strings.NewReader(body))
			if ct != "" {
				req.Header.Set(constants.HeaderContentType, ct)
			}
			req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
			w := httptest.NewRecorder()
			app.proxyHandler(w, req)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			require.NotEmpty(t, selected)
			assert.Equal(t, body, string(forwarded), "forwarded body must be byte-identical")
			assert.Equal(t, ct, req.Header.Get(constants.HeaderContentType), "Content-Type must not be rewritten")
			for _, ep := range selected {
				assert.Equal(t, "ollama-has", ep.Name)
			}
		})
	}
}
