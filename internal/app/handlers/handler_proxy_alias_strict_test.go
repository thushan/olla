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

// TestAliasRejection_StrictRouting_FailsFastWithoutProxying is the full-handler
// regression test for #191: a model alias whose real target exists on no endpoint
// must be rejected before the request ever reaches the proxy engine, on both the
// provider-scoped route (providerProxyHandler) and the plain proxy route
// (proxyHandler). Before the fix, resolveAliasEndpoints fell back to returning
// every compatible endpoint, so the request silently proxied to a backend that
// couldn't serve the requested model and returned a false 200.
func TestAliasRejection_StrictRouting_FailsFastWithoutProxying(t *testing.T) {
	t.Parallel()

	styledLog := &mockStyledLogger{}

	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	inspectorFactory := inspector.NewFactory(profileFactory, styledLog)
	pathInspector := inspectorFactory.CreatePathInspector()
	bodyInspector, err := inspectorFactory.CreateBodyInspector()
	require.NoError(t, err)

	chain := inspectorFactory.CreateChain()
	chain.AddInspector(pathInspector)
	chain.AddInspector(bodyInspector)

	// The alias resolves to a real model name that exists on no endpoint, and the
	// registry is in strict mode so the standard-routing fallback also rejects.
	aliases := map[string][]string{
		"gpt-oss-120b": {"gpt-oss:120b"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, styledLog)
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{},
		strict:            true,
	}

	u, _ := url.Parse("http://ollama-1:11434")
	discoveryService := &mockDiscoveryServiceWithHealthy{
		endpoints: []*domain.Endpoint{{
			Name:      "ollama-1",
			URL:       u,
			URLString: u.String(),
			Type:      domain.ProfileOllama,
			Status:    domain.StatusHealthy,
		}},
	}

	body := `{"model":"gpt-oss-120b","messages":[{"role":"user","content":"hi"}]}`

	newApp := func(called *bool) *Application {
		return &Application{
			Config: &config.Config{
				Server: config.ServerConfig{RateLimits: config.ServerRateLimits{}},
			},
			logger:           styledLog,
			profileFactory:   profileFactory,
			inspectorChain:   chain,
			discoveryService: discoveryService,
			modelRegistry:    modelRegistry,
			aliasResolver:    aliasResolver,
			proxyService: &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
					*called = true
					w.WriteHeader(http.StatusOK)
					return nil
				},
			},
			StartTime: time.Now(),
		}
	}

	t.Run("provider route rejects without proxying", func(t *testing.T) {
		var called bool
		app := newApp(&called)

		req := httptest.NewRequest(http.MethodPost, "/olla/ollama/api/chat", strings.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w := httptest.NewRecorder()

		app.providerProxyHandler(w, req)

		assert.False(t, called, "request must not reach the proxy engine when the alias target is absent under strict routing")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "strict", w.Header().Get(constants.HeaderXOllaRoutingStrategy))
		assert.Equal(t, "rejected", w.Header().Get(constants.HeaderXOllaRoutingDecision))
	})

	t.Run("plain proxy route rejects without proxying", func(t *testing.T) {
		var called bool
		app := newApp(&called)

		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/api/chat", strings.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		ctx := context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		app.proxyHandler(w, req)

		assert.False(t, called, "request must not reach the proxy engine when the alias target is absent under strict routing")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "strict", w.Header().Get(constants.HeaderXOllaRoutingStrategy))
		assert.Equal(t, "rejected", w.Header().Get(constants.HeaderXOllaRoutingDecision))
	})
}
