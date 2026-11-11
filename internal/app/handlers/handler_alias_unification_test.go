package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/balancer"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func TestConfiguredAliasNameUnification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		aliases     map[string][]string
		requested   string
		unified     bool
		target      string
		stickyModel string
	}{
		{"capitalised alias lower request", map[string][]string{"My-Model": {"Native-Model"}}, "my-model", true, "Native-Model", "My-Model"},
		{"lower alias capitalised request", map[string][]string{"my-model": {"Native-Model"}}, "MY-MODEL", true, "Native-Model", "my-model"},
		{"capitalised exact", map[string][]string{"My-Model": {"Native-Model"}}, "My-Model", true, "Native-Model", "My-Model"},
		{"disabled capitalised alias", map[string][]string{"My-Model": {"Native-Model"}}, "My-Model", false, "", ""},
		{"disabled lower alias", map[string][]string{"my-model": {"Native-Model"}}, "MY-MODEL", false, "Native-Model", "my-model"},
		{"disabled case mismatch", map[string][]string{"My-Model": {"Native-Model"}}, "my-model", false, "", ""},
		{"collision first exact", map[string][]string{"My-Model": {"Native-Model"}, "my-model": {"Other-Model"}}, "My-Model", true, "Native-Model", "My-Model"},
		{"collision second exact", map[string][]string{"My-Model": {"Native-Model"}, "my-model": {"Other-Model"}}, "my-model", true, "Other-Model", "my-model"},
		{"collision ambiguous", map[string][]string{"My-Model": {"Native-Model"}, "my-model": {"Other-Model"}}, "MY-MODEL", true, "", ""},
		{"no punctuation normalisation", map[string][]string{"My-Model": {"Native-Model"}}, "my_model", true, "", ""},
		{"native target overlaps alias", map[string][]string{"Alias": {"MY-ALIAS"}, "my-alias": {"Other-Model"}}, "ALIAS", true, "MY-ALIAS", "Alias"},
		{"unknown alias", map[string][]string{"My-Model": {"Native-Model"}}, "unknown", true, "", ""},
	} {
		for _, route := range []string{"proxy", "provider", "anthropic"} {
			t.Run(tc.name+"/"+route, func(t *testing.T) {
				log := &mockStyledLogger{}
				endpointURL, err := url.Parse("http://backend:8000")
				require.NoError(t, err)
				endpoint := &domain.Endpoint{Name: "backend", URL: endpointURL, URLString: endpointURL.String(), Type: domain.ProfileOllama, Status: domain.StatusHealthy}
				discovery := &mockDiscoveryServiceWithHealthy{endpoints: []*domain.Endpoint{endpoint}}
				modelRegistry := &mockSimpleModelRegistry{strict: true, endpointsForModel: map[string][]string{
					"Native-Model": {endpointURL.String()}, "Other-Model": {endpointURL.String()}, "MY-ALIAS": {endpointURL.String()},
				}}
				stickyConfig := config.StickySessionConfig{Enabled: true, KeySources: []string{"session_header"}}
				var forwarded bool
				proxy := &mockProxyService{proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
					forwarded = true
					require.Equal(t, []*domain.Endpoint{endpoint}, endpoints)
					aliasMap, ok := ctx.Value(constants.ContextModelAliasMapKey).(map[string]string)
					require.True(t, ok, "the proxy needs the native target rewrite map")
					assert.Equal(t, tc.target, aliasMap[endpointURL.String()])
					requested := tc.requested
					if !tc.unified {
						requested = strings.ToLower(requested)
					}
					assert.Equal(t, requested, ctx.Value(constants.ContextModelKey))
					expectedKey, _ := balancer.ComputeStickyKey(r, tc.stickyModel, stickyConfig, nil)
					require.NotEmpty(t, expectedKey)
					assert.Equal(t, expectedKey, ctx.Value(constants.ContextStickyKeyKey), "case variants of one alias must keep the same session affinity")
					assert.Equal(t, tc.target, core.ResolvedModelName(ctx, endpoint, requested), "stats must retain the native target even when it resembles another alias")
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					return json.NewEncoder(w).Encode(map[string]interface{}{
						"id": "chatcmpl-test", "object": "chat.completion", "model": tc.target,
						"choices": []interface{}{map[string]interface{}{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "hello"}, "finish_reason": "stop"}},
						"usage":   map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
					})
				}}
				cfg := config.DefaultConfig()
				cfg.ModelAliases = tc.aliases
				cfg.Proxy.StickySessions = stickyConfig
				cfg.ModelRegistry.EnableUnifier = tc.unified
				cfg.Translators.Anthropic.PassthroughEnabled = false
				app, err := NewApplication(t.Context(), cfg, proxy, &mockStatsCollector{}, modelRegistry, discovery, &mockEndpointRepository{}, nil, log)
				require.NoError(t, err)
				path := "/olla/proxy/v1/chat/completions"
				if route == "provider" {
					path = "/olla/ollama/v1/chat/completions"
				}
				if route == "anthropic" {
					path = "/olla/anthropic/v1/messages"
				}
				body := fmt.Sprintf(`{"model":%q,"max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`, tc.requested)
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
				req.Header.Set(constants.HeaderXOllaSessionID, "alias-session")
				response := httptest.NewRecorder()
				switch route {
				case "proxy":
					req = req.WithContext(context.WithValue(req.Context(), constants.ContextRoutePrefixKey, "/olla/proxy/"))
					app.proxyHandler(response, req)
				case "provider":
					app.providerProxyHandler(response, req)
				case "anthropic":
					trans, err := anthropic.NewTranslator(log, cfg.Translators.Anthropic)
					require.NoError(t, err)
					app.translationHandler(trans).ServeHTTP(response, req)
				}
				if tc.target == "" {
					assert.False(t, forwarded)
					assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
				} else {
					assert.True(t, forwarded)
					assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
				}
			})
		}
	}
}
