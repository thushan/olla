package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// Verifies that proxyHandler strips the route prefix from r.URL.Path before
// forwarding to the backend. Without this, backends receive the full
// /olla/proxy/... path which they cannot route.
func TestProxyPathStripping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		requestPath  string
		routePrefix  string
		expectedPath string
	}{
		{
			name:         "strips_olla_proxy_prefix_from_openai_path",
			requestPath:  "/olla/proxy/v1/chat/completions",
			routePrefix:  "/olla/proxy/",
			expectedPath: "/v1/chat/completions",
		},
		{
			name:         "strips_olla_proxy_prefix_from_ollama_path",
			requestPath:  "/olla/proxy/api/generate",
			routePrefix:  "/olla/proxy/",
			expectedPath: "/api/generate",
		},
		{
			name:         "no_route_prefix_leaves_path_unchanged",
			requestPath:  "/v1/chat/completions",
			routePrefix:  "", // no prefix injected
			expectedPath: "/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockLogger := &mockStyledLogger{}

			// Capture the path that the proxy service actually receives
			var capturedPath string
			proxyService := &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
					capturedPath = r.URL.Path
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(`{"ok":true}`))
					return err
				},
			}

			app := &Application{
				logger:           mockLogger,
				proxyService:     proxyService,
				discoveryService: &mockDiscoveryServiceForTranslation{},
				inspectorChain:   inspector.NewChain(mockLogger),
				profileFactory:   &mockProfileFactory{},
				Config: &config.Config{
					Server: config.ServerConfig{
						RateLimits: config.ServerRateLimits{},
					},
				},
				StartTime: time.Now(),
			}

			body := strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"hi"}]}`)
			req := httptest.NewRequest(http.MethodPost, tt.requestPath, body)
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

			// Inject the route prefix into context, mirroring what the router does
			if tt.routePrefix != "" {
				ctx := context.WithValue(req.Context(), constants.ContextRoutePrefixKey, tt.routePrefix)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			app.proxyHandler(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, "handler should complete successfully")
			assert.Equal(t, tt.expectedPath, capturedPath,
				"proxy service should receive the path with the route prefix stripped")
		})
	}
}
