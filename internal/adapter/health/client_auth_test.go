package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// authEnforcingBackend returns 401 when the expected header is absent or wrong,
// and 200 when it matches. This proves auth is actually transported, not just set.
func authEnforcingBackend(t *testing.T, headerName, wantValue string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerName) != wantValue {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func makeEndpoint(t *testing.T, rawURL string) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{
		Name:                 "test",
		URL:                  u,
		HealthCheckURL:       u,
		URLString:            u.String(),
		HealthCheckURLString: u.String(),
		CheckTimeout:         2 * time.Second,
	}
}

// TestHealthProbe_BearerAuth proves that a probe on an endpoint with bearer auth
// reaches the backend with the correct Authorization header and is classified healthy.
func TestHealthProbe_BearerAuth(t *testing.T) {
	t.Parallel()

	const token = "Bearer secret-token"
	srv := authEnforcingBackend(t, "Authorization", token)

	ep := makeEndpoint(t, srv.URL)
	ep.AuthHeaderName = "Authorization"
	ep.AuthHeaderValue = token

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusHealthy, result.Status, "probe with correct bearer token must be healthy")
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

// TestHealthProbe_MissingAuth demonstrates that probing an auth-protected backend
// without credentials configured on the endpoint returns an unhealthy classification.
// A 401 response maps to StatusConfigError via the health client's status mapping.
func TestHealthProbe_MissingAuth(t *testing.T) {
	t.Parallel()

	srv := authEnforcingBackend(t, "Authorization", "Bearer required")

	// Endpoint has no auth configured — backend will reject with 401.
	ep := makeEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	// No transport error — just an HTTP 401.
	require.NoError(t, err, "401 is an HTTP response, not a transport error")
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
	assert.NotEqual(t, domain.StatusHealthy, result.Status, "unauthenticated probe must not be healthy")
}

// TestHealthProbe_CustomHeaders proves that the endpoint.Headers map entries
// are forwarded on health probes, not just on proxy requests.
func TestHealthProbe_CustomHeaders(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Backend-Key"
		headerValue = "backend-secret"
	)

	srv := authEnforcingBackend(t, headerName, headerValue)

	ep := makeEndpoint(t, srv.URL)
	ep.Headers = map[string]string{
		headerName: headerValue,
	}

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusHealthy, result.Status, "probe with custom header must be healthy")
}

// TestInjectEndpointAuth_Precedence verifies that auth wins over the headers map
// when both configure the same field, matching CopyHeaders precedence.
func TestInjectEndpointAuth_Precedence(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://localhost/health", nil)
	require.NoError(t, err)

	ep := &domain.Endpoint{
		Headers: map[string]string{
			"Authorization": "Bearer from-headers-map",
		},
		AuthHeaderName:  "Authorization",
		AuthHeaderValue: "Bearer from-auth-section",
	}

	injectEndpointAuth(req, ep)

	values := req.Header["Authorization"]
	require.Len(t, values, 1, "must have exactly one Authorization value")
	assert.Equal(t, "Bearer from-auth-section", values[0], "auth section must beat headers map")
}

// TestInjectEndpointAuth_Nil ensures nil endpoint is a no-op and does not panic.
func TestInjectEndpointAuth_Nil(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://localhost/health", nil)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		injectEndpointAuth(req, nil)
	})

	assert.Empty(t, req.Header.Get("Authorization"))
}
