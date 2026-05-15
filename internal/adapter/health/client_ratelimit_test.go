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

// rateLimitBackend returns 429 with an optional Retry-After header.
func rateLimitBackend(t *testing.T, retryAfterHeader string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryAfterHeader != "" {
			w.Header().Set("Retry-After", retryAfterHeader)
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func rlEndpoint(t *testing.T, rawURL string) *domain.Endpoint {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &domain.Endpoint{
		Name:                 "rl-test",
		URL:                  u,
		HealthCheckURL:       u,
		URLString:            u.String(),
		HealthCheckURLString: u.String(),
		CheckTimeout:         2 * time.Second,
	}
}

// TestRateLimit_RetryAfterSeconds verifies that a numeric Retry-After is parsed
// into a RateLimitedUntil approximately 60s in the future.
func TestRateLimit_RetryAfterSeconds(t *testing.T) {
	t.Parallel()

	srv := rateLimitBackend(t, "60")
	ep := rlEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	before := time.Now()
	result, err := hc.Check(context.Background(), ep)
	after := time.Now()

	require.NoError(t, err)
	assert.Equal(t, domain.StatusRateLimited, result.Status)
	assert.False(t, result.RateLimitedUntil.IsZero(), "RateLimitedUntil must be set")

	// Allow a generous window for test execution jitter.
	lo := before.Add(59 * time.Second)
	hi := after.Add(61 * time.Second)
	assert.True(t, result.RateLimitedUntil.After(lo) && result.RateLimitedUntil.Before(hi),
		"RateLimitedUntil (%v) should be ~60s from probe time [%v, %v]",
		result.RateLimitedUntil, lo, hi)
}

// TestRateLimit_RetryAfterHTTPDate verifies that an HTTP-date Retry-After is parsed.
func TestRateLimit_RetryAfterHTTPDate(t *testing.T) {
	t.Parallel()

	future := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	httpDate := future.Format(http.TimeFormat)

	srv := rateLimitBackend(t, httpDate)
	ep := rlEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	result, err := hc.Check(context.Background(), ep)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusRateLimited, result.Status)
	assert.False(t, result.RateLimitedUntil.IsZero())
	// Should be within a second of our expected time.
	assert.WithinDuration(t, future, result.RateLimitedUntil, time.Second)
}

// TestRateLimit_NoRetryAfterUsesDefault verifies the 30s fallback when the header
// is absent.
func TestRateLimit_NoRetryAfterUsesDefault(t *testing.T) {
	t.Parallel()

	srv := rateLimitBackend(t, "")
	ep := rlEndpoint(t, srv.URL)

	hc := NewHealthClient(http.DefaultClient, NewCircuitBreaker())
	before := time.Now()
	result, err := hc.Check(context.Background(), ep)
	after := time.Now()

	require.NoError(t, err)
	assert.Equal(t, domain.StatusRateLimited, result.Status)

	lo := before.Add(DefaultRateLimitBackoff - time.Second)
	hi := after.Add(DefaultRateLimitBackoff + time.Second)
	assert.True(t, result.RateLimitedUntil.After(lo) && result.RateLimitedUntil.Before(hi),
		"default backoff should be ~%v", DefaultRateLimitBackoff)
}

// TestRateLimit_DoesNotTripCircuitBreaker checks that 429 responses never open the CB.
func TestRateLimit_DoesNotTripCircuitBreaker(t *testing.T) {
	t.Parallel()

	srv := rateLimitBackend(t, "1")
	ep := rlEndpoint(t, srv.URL)

	cb := NewCircuitBreaker()
	hc := NewHealthClient(http.DefaultClient, cb)

	for range DefaultCircuitBreakerThreshold * 3 {
		_, _ = hc.Check(context.Background(), ep)
	}

	assert.False(t, cb.IsOpen(ep.HealthCheckURLString),
		"circuit breaker must not open on rate-limit responses")
}

// TestScheduler_SkipsRateLimitedEndpoints verifies the comparison logic that the
// scheduler uses to skip endpoints still inside their Retry-After window.
// We test the predicate directly rather than wiring up a full scheduler tick
// to keep this fast and deterministic.
func TestScheduler_SkipsRateLimitedEndpoints(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name             string
		rateLimitedUntil time.Time
		wantSkipped      bool
	}{
		{
			name:             "window in future — skip",
			rateLimitedUntil: now.Add(30 * time.Second),
			wantSkipped:      true,
		},
		{
			name:             "window just expired — probe",
			rateLimitedUntil: now.Add(-time.Millisecond),
			wantSkipped:      false,
		},
		{
			name:             "zero time — probe (never rate-limited)",
			rateLimitedUntil: time.Time{},
			wantSkipped:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ep := &domain.Endpoint{RateLimitedUntil: tt.rateLimitedUntil}
			// Mirror the scheduler predicate from performHealthChecks.
			skipped := !ep.RateLimitedUntil.IsZero() && now.Before(ep.RateLimitedUntil)
			assert.Equal(t, tt.wantSkipped, skipped)
		})
	}
}
