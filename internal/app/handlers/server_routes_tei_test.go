package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// TestRegisterProviderRoutes_TextEmbeddingsInference exercises the real
// profile-driven route registration for TEI. It guards two things the static
// fallback cannot: both routing prefixes (text-embeddings-inference and tei)
// produce routes, and /info - TEI's discovery path, which carries model
// metadata rather than a listing - is NOT intercepted: it falls through to
// the catch-all proxy and is forwarded verbatim. The OpenAI model listing is
// served at /v1/models instead.
func TestRegisterProviderRoutes_TextEmbeddingsInference(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err, "shipped profiles must load")

	teiURL, _ := url.Parse("http://tei:8080")
	endpoint := &domain.Endpoint{
		Name:      "tei-1",
		URL:       teiURL,
		URLString: teiURL.String(),
		Status:    domain.StatusHealthy,
		Type:      constants.ProviderTypeTextEmbeddingsInference,
	}

	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(&mockStyledLogger{}, nil, nil, nil)
	ctx := context.Background()
	unifiedRegistry.RegisterEndpoint(endpoint)

	const teiModel = "Snowflake/snowflake-arctic-embed-l-v2.0"
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoint, []*domain.ModelInfo{{Name: teiModel}}))

	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= 1
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:           &config.Config{},
		logger:           &mockStyledLogger{},
		routeRegistry:    reg,
		profileFactory:   factory,
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: []*domain.Endpoint{endpoint}},
		converterFactory: converter.NewConverterFactory(),
	}

	app.registerProviderRoutes()

	// Both prefixes must expose the OpenAI listing, and /info must not be
	// intercepted (it is forwarded verbatim by the catch-all proxy).
	for _, prefix := range []string{constants.ProviderPrefixTextEmbeddingsInference1, constants.ProviderPrefixTextEmbeddingsInference2} {
		routes := reg.GetRoutes()
		assert.NotContains(t, routes, "/olla/"+prefix+"/info", "prefix %s must not intercept /info", prefix)
		assert.Contains(t, routes, "/olla/"+prefix+"/v1/models", "prefix %s must register /v1/models", prefix)
	}

	mux := http.NewServeMux()
	reg.WireUp(mux)

	for _, path := range []string{"/olla/text-embeddings-inference/v1/models", "/olla/tei/v1/models"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			var response struct {
				Object string `json:"object"`
				Data   []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			assert.Equal(t, "list", response.Object)
			require.Len(t, response.Data, 1)
			assert.Equal(t, teiModel, response.Data[0].ID)
		})
	}
}

// teiCaptureProxyService records the request the handler hands to the proxy
// engine so tests can assert on the forwarded path, method and body without a
// live backend.
type teiCaptureProxyService struct {
	invoked   bool
	path      string
	method    string
	body      []byte
	endpoints []*domain.Endpoint
}

func (m *teiCaptureProxyService) reset() {
	m.invoked = false
	m.path = ""
	m.method = ""
	m.body = nil
	m.endpoints = nil
}

func (m *teiCaptureProxyService) ProxyRequestToEndpoints(_ context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
	m.invoked = true
	m.path = r.URL.Path
	m.method = r.Method
	m.endpoints = endpoints
	if r.Body != nil {
		m.body, _ = io.ReadAll(r.Body)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (m *teiCaptureProxyService) ProxyRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ *ports.RequestStats, _ logger.StyledLogger) error {
	return nil
}

func (m *teiCaptureProxyService) GetStats(_ context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}

func (m *teiCaptureProxyService) UpdateConfig(_ ports.ProxyConfiguration) {}

// TestRegisterProviderRoutes_TextEmbeddingsInference_ForwardsNativePaths proves
// the fix for the upstream review finding: every native TEI endpoint is
// forwarded verbatim by the provider catch-all route, not just the
// OpenAI-compatible /v1/embeddings. For each documented path - including /info,
// TEI's discovery path, which carries model metadata rather than a listing - and
// both routing prefixes, the request must reach the proxy engine with the
// /olla/<prefix> prefix stripped and the body byte-for-byte unchanged - no
// per-endpoint handlers, no translation.
func TestRegisterProviderRoutes_TextEmbeddingsInference_ForwardsNativePaths(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err, "shipped profiles must load")

	teiURL, _ := url.Parse("http://tei:8080")
	endpoint := &domain.Endpoint{
		Name:      "tei-1",
		URL:       teiURL,
		URLString: teiURL.String(),
		Status:    domain.StatusHealthy,
		Type:      constants.ProviderTypeTextEmbeddingsInference,
	}

	capture := &teiCaptureProxyService{}
	app := &Application{
		Config:           &config.Config{},
		logger:           &mockStyledLogger{},
		routeRegistry:    router.NewRouteRegistry(&mockStyledLogger{}),
		profileFactory:   factory,
		discoveryService: &mockDiscoveryServiceWithHealthy{endpoints: []*domain.Endpoint{endpoint}},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		proxyService:     capture,
		converterFactory: converter.NewConverterFactory(),
	}

	app.registerProviderRoutes()

	mux := http.NewServeMux()
	app.routeRegistry.WireUp(mux)

	cases := []struct {
		path   string
		method string
		body   string
	}{
		{"/v1/embeddings", http.MethodPost, `{"model":"tei-model","input":"hello"}`},
		{"/embed", http.MethodPost, `{"inputs":"hello"}`},
		{"/embed_all", http.MethodPost, `{"inputs":["a","b"]}`},
		{"/embed_sparse", http.MethodPost, `{"inputs":"hello"}`},
		{"/tokenize", http.MethodPost, `{"inputs":"hello"}`},
		{"/decode", http.MethodPost, `{"ids":[1,2]}`},
		{"/predict", http.MethodPost, `{"inputs":"hello"}`},
		{"/rerank", http.MethodPost, `{"query":"q","texts":["a","b"]}`},
		{"/similarity", http.MethodPost, `{"inputs":{"source_sentence":"s","sentences":["a"]}}`},
		{"/info", http.MethodGet, ""},
		{"/health", http.MethodGet, ""},
		{"/metrics", http.MethodGet, ""},
	}

	for _, prefix := range []string{constants.ProviderPrefixTextEmbeddingsInference1, constants.ProviderPrefixTextEmbeddingsInference2} {
		for _, tc := range cases {
			t.Run(prefix+tc.path, func(t *testing.T) {
				capture.reset()

				var bodyReader io.Reader
				if tc.body != "" {
					bodyReader = strings.NewReader(tc.body)
				}
				req := httptest.NewRequest(tc.method, "/olla/"+prefix+tc.path, bodyReader)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)

				require.True(t, capture.invoked,
					"request to /olla/%s%s must reach the proxy engine (got HTTP %d: %s)",
					prefix, tc.path, rec.Code, rec.Body.String())
				assert.Equal(t, tc.path, capture.path,
					"prefix must be stripped before forwarding")
				assert.Equal(t, tc.method, capture.method)
				require.NotEmpty(t, capture.endpoints, "a TEI endpoint must be selected")
				assert.Equal(t, "tei-1", capture.endpoints[0].Name)

				if tc.body != "" {
					assert.Equal(t, tc.body, string(capture.body),
						"native body must be forwarded byte-for-byte, unmodified")
				}
			})
		}
	}
}
