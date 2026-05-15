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

// statusBackend returns a fixed HTTP status code for every request.
func statusBackend(t *testing.T, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func probeEndpoint(t *testing.T, rawURL string) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{
		Name:                 "classify-test",
		URL:                  u,
		HealthCheckURL:       u,
		URLString:            u.String(),
		HealthCheckURLString: u.String(),
		CheckTimeout:         2 * time.Second,
	}
}

func TestDetermineStatus_401_IsConfigError(t *testing.T) {
	t.Parallel()

	srv := statusBackend(t, http.StatusUnauthorized)
	ep := probeEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	require.NoError(t, err, "401 is a valid HTTP response, not a transport error")
	assert.Equal(t, domain.StatusConfigError, result.Status)
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
}

func TestDetermineStatus_403_IsConfigError(t *testing.T) {
	t.Parallel()

	srv := statusBackend(t, http.StatusForbidden)
	ep := probeEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusConfigError, result.Status)
	assert.Equal(t, http.StatusForbidden, result.StatusCode)
}

// TestConfigError_DoesNotTripCircuitBreaker verifies that many 401 responses never
// open the circuit breaker. Auth failures are a config problem; the CB is for
// service availability problems.
func TestConfigError_DoesNotTripCircuitBreaker(t *testing.T) {
	t.Parallel()

	srv := statusBackend(t, http.StatusUnauthorized)
	ep := probeEndpoint(t, srv.URL)

	cb := NewCircuitBreaker()
	hc := NewHealthClient(http.DefaultClient, cb)

	// Fire well past the CB threshold.
	for range DefaultCircuitBreakerThreshold * 3 {
		_, _ = hc.Check(context.Background(), ep)
	}

	assert.False(t, cb.IsOpen(ep.HealthCheckURLString),
		"circuit breaker must not open on repeated auth failures")
}

// TestConfigError_IsNotRoutable ensures the new status doesn't accidentally
// get included in the routable set.
func TestConfigError_IsNotRoutable(t *testing.T) {
	t.Parallel()

	assert.False(t, domain.StatusConfigError.IsRoutable())
	assert.False(t, domain.StatusRateLimited.IsRoutable())
}

// TestDetermineStatus_ExistingCases guards regressions on the pre-existing
// status classification table now that the switch has new cases.
func TestDetermineStatus_ExistingCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		latency    time.Duration
		want       domain.EndpointStatus
	}{
		{"200 fast → healthy", http.StatusOK, 0, domain.StatusHealthy},
		{"200 slow → busy", http.StatusOK, SlowResponseThreshold + time.Millisecond, domain.StatusBusy},
		{"404 → unhealthy", http.StatusNotFound, 0, domain.StatusUnhealthy},
		{"500 → unhealthy", http.StatusInternalServerError, 0, domain.StatusUnhealthy},
		{"401 → config_error", http.StatusUnauthorized, 0, domain.StatusConfigError},
		{"403 → config_error", http.StatusForbidden, 0, domain.StatusConfigError},
		{"429 → rate_limited", http.StatusTooManyRequests, 0, domain.StatusRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := determineStatus(tt.statusCode, tt.latency, nil, domain.ErrorTypeNone)
			assert.Equal(t, tt.want, got)
		})
	}
}
