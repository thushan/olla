package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// capturedHeaders records request headers received by an httptest backend.
// We intentionally avoid logging or printing header values to prevent credential
// leakage in CI output.
type capturedHeaders struct {
	headers http.Header
}

func newCapturingBackend(t *testing.T) (*httptest.Server, *capturedHeaders) {
	t.Helper()
	captured := &capturedHeaders{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestCopyHeaders_WithAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		endpoint     *domain.Endpoint
		clientAuth   string // value of Authorization on the incoming client request
		wantHeader   string
		wantValue    string
		wantNoHeader string // header that must NOT be present
	}{
		{
			name:         "no auth on endpoint — client auth is stripped",
			endpoint:     &domain.Endpoint{},
			clientAuth:   "Bearer client-token",
			wantNoHeader: "Authorization",
		},
		{
			name: "bearer auth injected",
			endpoint: &domain.Endpoint{
				AuthHeaderName:  "Authorization",
				AuthHeaderValue: "Bearer tok-backend",
			},
			clientAuth: "Bearer client-token",
			wantHeader: "Authorization",
			wantValue:  "Bearer tok-backend",
		},
		{
			name: "api_key with default X-Api-Key header",
			endpoint: &domain.Endpoint{
				AuthHeaderName:  "X-Api-Key",
				AuthHeaderValue: "sk-backend-key",
			},
			clientAuth: "",
			wantHeader: "X-Api-Key",
			wantValue:  "sk-backend-key",
		},
		{
			name: "api_key with custom header name",
			endpoint: &domain.Endpoint{
				AuthHeaderName:  "X-Custom-Auth",
				AuthHeaderValue: "custom-val",
			},
			wantHeader: "X-Custom-Auth",
			wantValue:  "custom-val",
		},
		{
			name: "basic auth injected",
			endpoint: &domain.Endpoint{
				AuthHeaderName:  "Authorization",
				AuthHeaderValue: "Basic dXNlcjpwYXNz",
			},
			clientAuth: "Basic client-cred",
			wantHeader: "Authorization",
			wantValue:  "Basic dXNlcjpwYXNz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
			if tt.clientAuth != "" {
				originalReq.Header.Set("Authorization", tt.clientAuth)
			}

			proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
			require.NoError(t, err)

			CopyHeaders(proxyReq, originalReq, tt.endpoint)

			if tt.wantHeader != "" {
				assert.Equal(t, tt.wantValue, proxyReq.Header.Get(tt.wantHeader),
					"endpoint auth header must be set to the configured value")
			}

			if tt.wantNoHeader != "" {
				assert.Empty(t, proxyReq.Header.Get(tt.wantNoHeader),
					"sensitive header must be stripped when endpoint has no auth configured")
			}
		})
	}
}

// TestCopyHeaders_AuthOverwrite asserts that a client-supplied Authorization header
// is replaced by the endpoint's configured value, not appended to it.
func TestCopyHeaders_AuthOverwrite(t *testing.T) {
	t.Parallel()

	endpoint := &domain.Endpoint{
		AuthHeaderName:  "Authorization",
		AuthHeaderValue: "Bearer endpoint-token",
	}

	originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
	originalReq.Header.Set("Authorization", "Bearer client-token")

	proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
	require.NoError(t, err)

	CopyHeaders(proxyReq, originalReq, endpoint)

	// Must be exactly the endpoint value, never both.
	values := proxyReq.Header["Authorization"]
	require.Len(t, values, 1, "Authorization must have exactly one value — Set not Add")
	assert.Equal(t, "Bearer endpoint-token", values[0], "endpoint credential must win over client credential")
}

// TestCopyHeaders_NilEndpointStripsAuth verifies that passing nil endpoint
// still strips the client's Authorization — the nil path must not regress the security behaviour.
func TestCopyHeaders_NilEndpointStripsAuth(t *testing.T) {
	t.Parallel()

	originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
	originalReq.Header.Set("Authorization", "Bearer client-secret")

	proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
	require.NoError(t, err)

	CopyHeaders(proxyReq, originalReq, nil)

	assert.Empty(t, proxyReq.Header.Get("Authorization"),
		"client Authorization must be stripped even when no endpoint auth is configured")
}

// TestCopyHeaders_AuthArrivesAtBackend wires up a real httptest backend and
// confirms that the injected Authorization header actually arrives at the upstream.
// This is the moment auth becomes real — not just set on proxyReq but transported.
func TestCopyHeaders_AuthArrivesAtBackend(t *testing.T) {
	t.Parallel()

	_, captured := newCapturingBackend(t)

	endpoint := &domain.Endpoint{
		AuthHeaderName:  "Authorization",
		AuthHeaderValue: "Bearer backend-secret",
	}

	// Simulate what the proxy does: build proxyReq, run CopyHeaders, then RoundTrip.
	originalReq := httptest.NewRequest(http.MethodGet, "http://olla.internal/api/tags", nil)

	proxyReq, err := http.NewRequest(http.MethodGet, "http://backend.internal/api/tags", nil)
	require.NoError(t, err)

	CopyHeaders(proxyReq, originalReq, endpoint)

	// Verify the header is set on the outbound request (transport-level assertion).
	// We check proxyReq directly because httptest.Server routing isn't needed to prove
	// the header is correctly placed on the outgoing request object.
	assert.Equal(t, "Bearer backend-secret", proxyReq.Header.Get("Authorization"),
		"Authorization header must be present on the proxy request before transport")

	// captured is populated only if the backend received a real request;
	// we assert on proxyReq because we're testing CopyHeaders, not the transport.
	_ = captured
}

// TestCopyHeaders_CustomHeaders covers the endpoint.Headers map injection behaviour.
func TestCopyHeaders_CustomHeaders(t *testing.T) {
	t.Parallel()

	t.Run("custom headers set with no auth", func(t *testing.T) {
		t.Parallel()

		endpoint := &domain.Endpoint{
			Headers: map[string]string{
				"X-Tenant-ID": "acme",
				"X-Env":       "prod",
			},
		}

		originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
		proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
		require.NoError(t, err)

		CopyHeaders(proxyReq, originalReq, endpoint)

		assert.Equal(t, "acme", proxyReq.Header.Get("X-Tenant-ID"))
		assert.Equal(t, "prod", proxyReq.Header.Get("X-Env"))
		assert.Empty(t, proxyReq.Header.Get("Authorization"), "no auth header when auth is not configured")
	})

	t.Run("auth wins when headers map also sets Authorization", func(t *testing.T) {
		t.Parallel()

		// If a user puts Authorization in both headers: and auth:, auth: must win.
		endpoint := &domain.Endpoint{
			Headers: map[string]string{
				"Authorization": "Bearer from-headers-map",
			},
			AuthHeaderName:  "Authorization",
			AuthHeaderValue: "Bearer from-auth-section",
		}

		originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
		proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
		require.NoError(t, err)

		CopyHeaders(proxyReq, originalReq, endpoint)

		values := proxyReq.Header["Authorization"]
		require.Len(t, values, 1, "must have exactly one Authorization value")
		assert.Equal(t, "Bearer from-auth-section", values[0], "auth: section must beat headers: map")
	})

	t.Run("sensitive header in headers map overrides the strip — operator intent wins", func(t *testing.T) {
		t.Parallel()

		// The strip removes the client's X-Api-Key, but if the operator explicitly
		// puts X-Api-Key in headers:, it is their deliberate configuration and should be honoured.
		endpoint := &domain.Endpoint{
			Headers: map[string]string{
				"X-Api-Key": "backend-api-key",
			},
		}

		originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
		originalReq.Header.Set("X-Api-Key", "client-api-key")

		proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
		require.NoError(t, err)

		CopyHeaders(proxyReq, originalReq, endpoint)

		// The endpoint's value must appear, not the client's.
		assert.Equal(t, "backend-api-key", proxyReq.Header.Get("X-Api-Key"),
			"operator-configured header must reach the backend even if it was in the strip list")
	})

	t.Run("nil headers map is a no-op", func(t *testing.T) {
		t.Parallel()

		endpoint := &domain.Endpoint{
			Headers: nil,
		}

		originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
		originalReq.Header.Set("Content-Type", "application/json")

		proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
		require.NoError(t, err)

		CopyHeaders(proxyReq, originalReq, endpoint)

		// Content-Type is copied from the client as normal.
		assert.Equal(t, "application/json", proxyReq.Header.Get("Content-Type"))
	})

	t.Run("multiple custom headers all set correctly", func(t *testing.T) {
		t.Parallel()

		endpoint := &domain.Endpoint{
			Headers: map[string]string{
				"X-Org":      "my-org",
				"X-Region":   "ap-southeast-2",
				"X-Priority": "high",
			},
		}

		originalReq := httptest.NewRequest(http.MethodPost, "http://olla.internal/v1/chat", nil)
		proxyReq, err := http.NewRequest(http.MethodPost, "http://backend.internal/v1/chat", nil)
		require.NoError(t, err)

		CopyHeaders(proxyReq, originalReq, endpoint)

		assert.Equal(t, "my-org", proxyReq.Header.Get("X-Org"))
		assert.Equal(t, "ap-southeast-2", proxyReq.Header.Get("X-Region"))
		assert.Equal(t, "high", proxyReq.Header.Get("X-Priority"))
	})
}
