package handlers

// Regression tests for the security-middleware status-code bug:
// Before the fix, proxy routes received 403 "Security validation failed" for
// both rate-limit rejections and oversized-body rejections, because
// SecurityAdapters.CreateChainMiddleware called the abstract securityChain.Validate
// and hard-coded http.StatusForbidden for any rejection.
//
// The correct behaviour (enforced here):
//   - Rate-limited request  → 429 + Retry-After + X-RateLimit-* headers
//   - Oversized body        → 413

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/router"
)

// buildTightSecurityAdapters returns a *security.Adapters whose rate limiter
// immediately rejects any request (1 req/min, burst 0) and whose size
// validator rejects bodies > 10 bytes.
func buildTightSecurityAdapters(t *testing.T) *security.Adapters {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			RateLimits: config.ServerRateLimits{
				GlobalRequestsPerMinute: 1,
				PerIPRequestsPerMinute:  1,
				BurstSize:               0,
			},
			RequestLimits: config.ServerRequestLimits{
				MaxBodySize:   10,
				MaxHeaderSize: 0,
			},
		},
	}
	_, adapters := security.NewSecurityServices(cfg, &mockStatsCollector{}, &mockStyledLogger{})
	t.Cleanup(adapters.Stop)
	return adapters
}

// buildSecurityAdaptersWithSizeOnly returns adapters with no rate limiting but a
// strict 10-byte body cap, to isolate the 413 path.
func buildSecurityAdaptersWithSizeOnly(t *testing.T) *security.Adapters {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			RateLimits: config.ServerRateLimits{
				// Zero values mean the rate limiter allows everything.
				GlobalRequestsPerMinute: 0,
				PerIPRequestsPerMinute:  0,
				BurstSize:               0,
			},
			RequestLimits: config.ServerRequestLimits{
				MaxBodySize:   10,
				MaxHeaderSize: 0,
			},
		},
	}
	_, adapters := security.NewSecurityServices(cfg, &mockStatsCollector{}, &mockStyledLogger{})
	t.Cleanup(adapters.Stop)
	return adapters
}

// okHandler is a trivial handler that records whether it was reached.
func okHandler(reached *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	}
}

// alwaysRejectValidator is a SecurityValidator that always denies requests.
type alwaysRejectValidator struct{}

func (alwaysRejectValidator) Name() string { return "always-reject" }

func (alwaysRejectValidator) Validate(_ context.Context, _ ports.SecurityRequest) (ports.SecurityResult, error) {
	return ports.SecurityResult{Allowed: false, Reason: "test rejection"}, nil
}

// TestSecurityChainMiddleware_RateLimit_Returns429 verifies that a proxy route
// gets 429 (not 403) when the rate limiter rejects the request, and that the
// Retry-After and X-RateLimit-* response headers are present.
//
// burst=0 means every reserve call returns !OK, so the very first request is rejected.
func TestSecurityChainMiddleware_RateLimit_Returns429(t *testing.T) {
	t.Parallel()

	adapters := buildTightSecurityAdapters(t)

	sa := &SecurityAdapters{
		securityAdapters: adapters,
		logger:           &mockStyledLogger{},
	}

	reached := false
	chain := sa.CreateChainMiddleware()(okHandler(&reached))

	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions",
		strings.NewReader(`{"model":"llama3"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:9999"

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatal("rate limit returned 403 Forbidden — status code is being flattened by the abstract chain path")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for rate-limited request, got %d", w.Code)
	}

	// The concrete rate-limit middleware sets these headers unconditionally.
	if w.Header().Get("Retry-After") == "" {
		t.Error("rate-limited response missing Retry-After header")
	}
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("rate-limited response missing X-RateLimit-Limit header")
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("rate-limited response missing X-RateLimit-Remaining header")
	}
	if reached {
		t.Error("handler was reached; rate-limited request should have been rejected")
	}
}

// TestSecurityChainMiddleware_OversizedBody_Returns413 verifies that a proxy route
// gets 413 (not 403) when the request body exceeds the configured limit.
func TestSecurityChainMiddleware_OversizedBody_Returns413(t *testing.T) {
	t.Parallel()

	adapters := buildSecurityAdaptersWithSizeOnly(t)

	sa := &SecurityAdapters{
		securityAdapters: adapters,
		logger:           &mockStyledLogger{},
	}

	reached := false
	chain := sa.CreateChainMiddleware()(okHandler(&reached))

	// Body is 50 bytes, well above the 10-byte cap.
	body := strings.Repeat("x", 50)
	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions",
		strings.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:9999"

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatal("oversized body returned 403 Forbidden — status code is being flattened by the abstract chain path")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized body, got %d", w.Code)
	}
	if reached {
		t.Error("handler was reached; oversized request should have been rejected")
	}
}

// TestSecurityChainMiddleware_FallbackPath_Returns403 verifies that the abstract
// chain fallback (no securityAdapters wired) still returns 403 for rejected
// requests. This covers test contexts where only securityChain is available.
func TestSecurityChainMiddleware_FallbackPath_Returns403(t *testing.T) {
	t.Parallel()

	// Wire only the abstract chain, no concrete securityAdapters.
	rejectingChain := ports.NewSecurityChain(alwaysRejectValidator{})
	sa := &SecurityAdapters{
		securityChain: rejectingChain,
		logger:        &mockStyledLogger{},
		// securityAdapters intentionally nil — tests the fallback path.
	}

	reached := false
	chain := sa.CreateChainMiddleware()(okHandler(&reached))

	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/", strings.NewReader("hi"))
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("abstract-chain fallback expected 403, got %d", w.Code)
	}
	if reached {
		t.Error("handler should not be reached when abstract chain rejects")
	}
}

// rateLimitExcludedPrefixes lists proxy-style route prefixes the rate-limit
// route-table test skips. /olla/anthropic/* are registered as non-proxy routes
// today, so they carry no limiter and another change is bringing them in.
// Once that lands, delete the entry here and the loop covers them.
var rateLimitExcludedPrefixes = []string{"/olla/anthropic/"}

// TestRateLimit_EveryProxyRouteIsLimited loops the real route table (not a
// hand-written list) so a new inference route that skips the security chain
// fails here. Each tier is exercised on its own: a global-only config is the
// case that used to be silently unlimited, because per-IP 0 bypassed the
// whole validator.
func TestRateLimit_EveryProxyRouteIsLimited(t *testing.T) {
	t.Parallel()

	factory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err, "shipped profiles must load")

	tests := []struct {
		name      string
		limits    config.ServerRateLimits
		wantLimit string
	}{
		// Burst 0 rejects every request, so no real handler is ever reached.
		{"global only", config.ServerRateLimits{GlobalRequestsPerMinute: 7, PerIPRequestsPerMinute: 0}, "7"},
		{"per-IP only", config.ServerRateLimits{GlobalRequestsPerMinute: 0, PerIPRequestsPerMinute: 9}, "9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{Server: config.ServerConfig{RateLimits: tt.limits}}
			_, adapters := security.NewSecurityServices(cfg, &mockStatsCollector{}, &mockStyledLogger{})
			t.Cleanup(adapters.Stop)

			reg := router.NewRouteRegistry(&mockStyledLogger{})
			app := &Application{
				Config:         cfg,
				logger:         &mockStyledLogger{},
				routeRegistry:  reg,
				profileFactory: factory,
			}
			app.registerRoutes()

			mux := http.NewServeMux()
			reg.WireUpWithSecurityChain(mux, adapters)

			checked := 0
			for path, info := range reg.GetRoutes() {
				if !info.IsProxy || slices.ContainsFunc(rateLimitExcludedPrefixes, func(p string) bool {
					return strings.HasPrefix(path, p)
				}) {
					continue
				}
				checked++

				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				req.RemoteAddr = "192.0.2.1:4000"
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)

				assert.Equal(t, http.StatusTooManyRequests, rec.Code, "route %s must be rate limited", path)
				assert.Equal(t, tt.wantLimit, rec.Header().Get("X-RateLimit-Limit"), "route %s reported the wrong tier", path)
			}
			assert.Greater(t, checked, 10, "route table looks truncated; the loop is not covering the providers")
		})
	}
}
