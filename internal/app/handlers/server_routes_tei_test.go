package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/router"
)

// TestRegisterProviderRoutes_TextEmbeddingInference exercises the real
// profile-driven route registration for TEI. It guards two things the static
// fallback cannot: both routing prefixes (text-embedding-inference and tei)
// produce routes, and /info - TEI's discovery path, which has no native
// listing shape of its own - is served in OpenAI format rather than the
// unregistered "native" converter that would 500.
func TestRegisterProviderRoutes_TextEmbeddingInference(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err, "shipped profiles must load")

	teiURL, _ := url.Parse("http://tei:8080")
	endpoint := &domain.Endpoint{
		Name:      "tei-1",
		URL:       teiURL,
		URLString: teiURL.String(),
		Status:    domain.StatusHealthy,
		Type:      constants.ProviderTypeTextEmbeddingInference,
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

	// Both prefixes must expose the discovery path and the OpenAI listing.
	for _, prefix := range []string{constants.ProviderPrefixTextEmbeddingInference1, constants.ProviderPrefixTextEmbeddingInference2} {
		routes := reg.GetRoutes()
		assert.Contains(t, routes, "/olla/"+prefix+"/info", "prefix %s must register /info", prefix)
		assert.Contains(t, routes, "/olla/"+prefix+"/v1/models", "prefix %s must register /v1/models", prefix)
	}

	mux := http.NewServeMux()
	reg.WireUp(mux)

	for _, path := range []string{"/olla/text-embedding-inference/info", "/olla/tei/info"} {
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
