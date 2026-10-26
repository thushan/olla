package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/router"
)

// Provider routes come from the profiles alone. This walks every shipped
// profile and every routing prefix, so a provider whose routes need a special
// case in registerProviderPrefixRoutes shows up here rather than in production:
// each prefix must serve its model listing(s), and the catch-all proxy must be
// there for everything else.
func TestRegisterProviderRoutes_AllShippedProfiles(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err, "shipped profiles must load")

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	app := &Application{
		Config:           &config.Config{},
		logger:           &mockStyledLogger{},
		routeRegistry:    reg,
		profileFactory:   factory,
		modelRegistry:    registry.NewUnifiedMemoryModelRegistry(&mockStyledLogger{}, nil, nil, nil),
		repository:       &mockRepository{},
		converterFactory: converter.NewConverterFactory(),
	}
	app.registerProviderRoutes()

	mux := http.NewServeMux()
	reg.WireUp(mux)
	routes := reg.GetRoutes()

	// Profiles whose discovery path returns model metadata rather than a model
	// listing. The listing route must not shadow it: it is forwarded verbatim.
	metadataDiscovery := map[string]bool{constants.ProviderTypeTextEmbeddingsInference: true}

	names := append(factory.GetAvailableProfiles(), constants.ProviderTypeOpenAICompat)
	checked := 0
	for _, name := range names {
		p, perr := factory.GetProfile(name)
		if perr != nil {
			continue
		}
		cfg := p.GetConfig()
		for _, prefix := range cfg.Routing.Prefixes {
			t.Run(name+"/"+prefix, func(t *testing.T) {
				base := constants.DefaultOllaProxyPathPrefix + prefix + constants.DefaultPathPrefix
				assert.Contains(t, routes, base, "catch-all proxy route")

				listings := []string{}
				if cfg.API.OpenAICompatible {
					listings = append(listings, base+"v1/models")
				}
				if cfg.API.ModelDiscoveryPath != "" {
					discovery := base + strings.TrimPrefix(cfg.API.ModelDiscoveryPath, constants.DefaultPathPrefix)
					if metadataDiscovery[name] {
						assert.NotContains(t, routes, discovery, "discovery path carries metadata and must reach the backend")
					} else {
						listings = append(listings, discovery)
					}
				}
				require.NotEmpty(t, listings, "every profile serves at least one listing")

				for _, path := range listings {
					assert.Contains(t, routes, path)
					rec := httptest.NewRecorder()
					mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
					assert.Equal(t, http.StatusOK, rec.Code, "GET %s: %s", path, rec.Body.String())
					assert.Contains(t, rec.Header().Get(constants.HeaderContentType), "json", "GET %s", path)
				}
			})
			checked++
		}
	}
	assert.Greater(t, checked, 10, "walked the shipped profiles")
}
