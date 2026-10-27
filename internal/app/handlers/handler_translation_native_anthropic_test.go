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

// SGLang and LMDeploy ship with anthropic_support enabled. These tests run the
// shipped profiles through the real inspector chain so the mode header shows
// whether the profile took effect, rather than the request being quietly translated.

const (
	nativeAnthropicPlainBody = `{"model":"native-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	nativeAnthropicToolsBody = `{"model":"native-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"get_weather","description":"w","input_schema":{"type":"object"}}]}`
)

// postNativeAnthropic sends body to a fleet of an Ollama host plus one host of
// typ, serving native-model only on the latter, and returns the recorder, the
// endpoint names offered to the proxy and the path it was asked to forward.
func postNativeAnthropic(t *testing.T, typ, body string, passthrough bool) (*httptest.ResponseRecorder, []string, string) {
	t.Helper()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)
	log := &mockStyledLogger{}
	pathInspector := inspector.NewPathInspector(factory, log)
	chain := inspector.NewChain(log)
	chain.AddInspector(pathInspector)
	bodyInspector, err := inspector.NewBodyInspector(log)
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)
	trans, err := anthropic.NewTranslator(log, config.AnthropicTranslatorConfig{PassthroughEnabled: passthrough})
	require.NoError(t, err)

	mk := func(name, typ string) *domain.Endpoint {
		raw := "http://" + name + ".test:8080"
		u, _ := url.Parse(raw)
		return &domain.Endpoint{Name: name, Type: typ, URL: u, URLString: raw, Status: domain.StatusHealthy}
	}
	fleet := []*domain.Endpoint{mk("ollama-1", domain.ProfileOllama), mk("target-1", typ)}

	var offered []string
	var forwardedPath string
	app := &Application{
		Config:         &config.Config{},
		logger:         log,
		inspectorChain: chain,
		profileFactory: factory,
		profileLookup:  factory,
		discoveryService: &mockDiscoveryServiceWithFunc{getHealthyEndpointsFunc: func(context.Context) ([]*domain.Endpoint, error) {
			return fleet, nil
		}},
		modelRegistry:  &mockSimpleModelRegistry{strict: true, endpointsForModel: map[string][]string{"native-model": {fleet[1].URLString}}},
		statsCollector: &mockStatsCollector{},
		proxyService: &mockProxyService{proxyFunc: func(_ context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
			offered = offeredNames(eps)
			forwardedPath = r.URL.Path
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
			return nil
		}},
	}

	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()
	app.translationHandler(trans).ServeHTTP(w, req)
	return w, offered, forwardedPath
}

func TestTranslationHandler_NativeAnthropic_SGLangAndLMDeployPassThrough(t *testing.T) {
	t.Parallel()

	for _, typ := range []string{domain.ProfileSGLang, domain.ProfileLMDeploy} {
		t.Run(typ+"/passthrough", func(t *testing.T) {
			t.Parallel()
			w, offered, path := postNativeAnthropic(t, typ, nativeAnthropicPlainBody, true)
			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, []string{"target-1"}, offered)
			assert.Equal(t, string(constants.TranslatorModePassthrough), w.Header().Get(constants.HeaderXOllaMode))
			assert.Equal(t, "/v1/messages", path)
		})
		t.Run(typ+"/translation when passthrough is globally off", func(t *testing.T) {
			t.Parallel()
			w, offered, path := postNativeAnthropic(t, typ, nativeAnthropicPlainBody, false)
			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, []string{"target-1"}, offered)
			assert.Empty(t, w.Header().Get(constants.HeaderXOllaMode))
			assert.Equal(t, "/v1/chat/completions", path)
		})
	}
}

// SGLang takes tools natively; LMDeploy before 0.18 answers them with a
// NotImplementedError, so its profile declares no_tool_use.
func TestTranslationHandler_NativeAnthropic_ToolRequests(t *testing.T) {
	t.Parallel()

	t.Run("sglang tools pass through", func(t *testing.T) {
		t.Parallel()
		w, _, path := postNativeAnthropic(t, domain.ProfileSGLang, nativeAnthropicToolsBody, true)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, string(constants.TranslatorModePassthrough), w.Header().Get(constants.HeaderXOllaMode))
		assert.Equal(t, "/v1/messages", path)
	})
	t.Run("lmdeploy tools translate", func(t *testing.T) {
		t.Parallel()
		w, offered, path := postNativeAnthropic(t, domain.ProfileLMDeploy, nativeAnthropicToolsBody, true)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, []string{"target-1"}, offered)
		assert.Empty(t, w.Header().Get(constants.HeaderXOllaMode))
		assert.Equal(t, "/v1/chat/completions", path)
	})
}
