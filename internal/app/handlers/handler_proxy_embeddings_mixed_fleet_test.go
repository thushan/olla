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

// TestProxyEmbeddings_MixedTEIAndOllamaFleet guards the regression where an
// embeddings-tagged TEI model made the capability filter drop every endpoint
// without an embeddings-tagged model. Ollama's embedding models were untagged
// (its /api/tags capabilities field was discarded), so /v1/embeddings for
// all-minilm:latest was narrowed to TEI and failed with 503. The Ollama side is
// fed through the real Ollama parser so the test breaks if parsing regresses.
func TestProxyEmbeddings_MixedTEIAndOllamaFleet(t *testing.T) {
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

	mkEndpoint := func(name, rawURL, typ string) *domain.Endpoint {
		u, _ := url.Parse(rawURL)
		return &domain.Endpoint{Name: name, URL: u, URLString: u.String(), Type: typ, Status: domain.StatusHealthy}
	}
	teiEndpoint := mkEndpoint("tei-1", "http://tei:8080", constants.ProviderTypeTextEmbeddingsInference)
	ollamaEndpoint := mkEndpoint("ollama-1", "http://ollama:11434", domain.ProfileOllama)

	const teiModel = "Snowflake/snowflake-arctic-embed-l-v2.0"
	ollamaTags := `{"models":[
		{"name":"all-minilm:latest","details":{"family":"bert","parameter_size":"23M"},"capabilities":["embedding"]},
		{"name":"qwen3-vl:8b","details":{"family":"qwen3vl"},"capabilities":["completion","vision","tools","thinking"]}]}`
	ollamaProfile, err := profileFactory.GetProfile(domain.ProfileOllama)
	require.NoError(t, err)
	ollamaModels, err := ollamaProfile.ParseModelsResponse([]byte(ollamaTags))
	require.NoError(t, err)

	unified := registry.NewUnifiedMemoryModelRegistry(styledLog, nil, nil, nil)
	unified.RegisterEndpoint(teiEndpoint)
	unified.RegisterEndpoint(ollamaEndpoint)
	ctx := context.Background()
	require.NoError(t, unified.RegisterModelsWithEndpoint(ctx, teiEndpoint, []*domain.ModelInfo{{Name: teiModel, Type: "embeddings"}}))
	require.NoError(t, unified.RegisterModelsWithEndpoint(ctx, ollamaEndpoint, ollamaModels))
	require.Eventually(t, func() bool {
		models, gerr := unified.GetUnifiedModels(ctx)
		return gerr == nil && len(models) >= 3
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	newApp := func(selected *[]*domain.Endpoint) *Application {
		return &Application{
			Config:           &config.Config{Server: config.ServerConfig{RateLimits: config.ServerRateLimits{}}},
			logger:           styledLog,
			profileFactory:   profileFactory,
			inspectorChain:   chain,
			discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: []*domain.Endpoint{teiEndpoint, ollamaEndpoint}},
			modelRegistry:    unified,
			proxyService: &mockProxyService{
				proxyFunc: func(_ context.Context, w http.ResponseWriter, _ *http.Request, endpoints []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
					*selected = endpoints
					w.WriteHeader(http.StatusOK)
					return nil
				},
			},
			StartTime: time.Now(),
		}
	}

	tests := []struct {
		name         string
		model        string
		wantEndpoint string
	}{
		{"ollama embedding model reaches ollama", "all-minilm:latest", "ollama-1"},
		{"tei model reaches tei", teiModel, "tei-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var selected []*domain.Endpoint
			app := newApp(&selected)

			body := `{"model":"` + tt.model + `","input":"hello"}`
			req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/embeddings", strings.NewReader(body))
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
