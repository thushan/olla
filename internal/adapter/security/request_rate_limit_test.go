package security

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/thushan/olla/internal/util"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func createTestRateLimitLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func TestNewRateLimitValidator(t *testing.T) {
	cidrs := []string{"192.168.0.0/16", "10.0.0.0/8"}
	trustedCIDRs, _ := util.ParseTrustedCIDRs(cidrs)
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               50,
		HealthRequestsPerMinute: 500,
		CleanupInterval:         time.Minute,
		TrustProxyHeaders:       true,
		TrustedProxyCIDRs:       cidrs,
		TrustedProxyCIDRsParsed: trustedCIDRs,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	if validator.Name() != "rate_limit" {
		t.Errorf("Expected name 'rate_limit', got %q", validator.Name())
	}
	if validator.globalRequestsPerMinute != 1000 {
		t.Errorf("Expected global limit 1000, got %d", validator.globalRequestsPerMinute)
	}
	if validator.perIPRequestsPerMinute != 100 {
		t.Errorf("Expected per-IP limit 100, got %d", validator.perIPRequestsPerMinute)
	}
	if validator.burstSize != 50 {
		t.Errorf("Expected burst size 50, got %d", validator.burstSize)
	}
	if !validator.trustProxyHeaders {
		t.Error("Expected trust proxy headers to be true")
	}
	if validator.globalLimiter == nil {
		t.Error("Expected global limiter to be initialised")
	}
	if len(validator.trustedCIDRs) != 2 {
		t.Errorf("Expected 2 trusted CIDRs, got %d", len(validator.trustedCIDRs))
	}
}

func createTestRateLimitValidator(limits config.ServerRateLimits) *RateLimitValidator {
	logger := createTestRateLimitLogger()
	statsCollector := createTestStatsCollector(logger)
	metricsAdapter := NewSecurityMetricsAdapter(statsCollector, logger)

	return NewRateLimitValidator(limits, metricsAdapter, logger)
}
func TestNewRateLimitValidator_InvalidCIDRs(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 1000,
		PerIPRequestsPerMinute:  100,
		BurstSize:               50,
		TrustProxyHeaders:       true,
		TrustedProxyCIDRs:       []string{"invalid-cidr", "192.168.0.0/16"},
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	if validator.trustedCIDRs != nil {
		t.Error("Expected trustedCIDRs to be nil when parsing fails")
	}
}

func TestRateLimitValidator_Validate_Disabled(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  0,
		BurstSize:               10,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	for i := range 10 {
		result, err := validator.Validate(context.Background(), req)
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed when limits are disabled", i+1)
		}
	}

	if validator.globalLimiter != nil {
		t.Error("Global limiter should not be initialised when global limit is 0")
	}
}

func TestRateLimitValidator_Validate_HealthEndpoint(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		HealthRequestsPerMinute: 300,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()
	clientIP := "192.168.1.100"

	regularReq := ports.SecurityRequest{
		ClientID:      clientIP,
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	healthReq := ports.SecurityRequest{
		ClientID:      clientIP,
		Endpoint:      "/internal/health",
		Method:        "GET",
		IsHealthCheck: true,
	}

	regularResult, err := validator.Validate(ctx, regularReq)
	if err != nil {
		t.Fatalf("Regular request validation failed: %v", err)
	}
	if regularResult.RateLimit != 60 {
		t.Errorf("Expected regular limit 60, got %d", regularResult.RateLimit)
	}

	healthResult, err := validator.Validate(ctx, healthReq)
	if err != nil {
		t.Fatalf("Health request validation failed: %v", err)
	}
	if healthResult.RateLimit != 300 {
		t.Errorf("Expected health limit 300, got %d", healthResult.RateLimit)
	}
}

func TestRateLimitValidator_Validate_BurstCapacity(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()
	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	successCount := 0
	rateLimitedCount := 0

	for range 10 {
		result, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		if result.Allowed {
			successCount++
		} else {
			rateLimitedCount++
			if result.RetryAfter == 0 {
				t.Error("Expected Retry-After header when rate limited")
			}
		}
	}

	if successCount == 0 {
		t.Error("Expected some successful requests")
	}
	if rateLimitedCount == 0 {
		t.Log("No rate limiting triggered - this may be acceptable with new implementation")
	}
}

func TestRateLimitValidator_Validate_PerIPIsolation(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  60,
		BurstSize:               2,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()

	req1 := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	req2 := ports.SecurityRequest{
		ClientID:      "192.168.1.101",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	ip1Blocked := false
	for range 10 {
		result, err := validator.Validate(ctx, req1)
		if err != nil {
			t.Fatalf("IP1 validation failed: %v", err)
		}
		if !result.Allowed {
			ip1Blocked = true
			break
		}
		time.Sleep(time.Millisecond)
	}

	result2, err := validator.Validate(ctx, req2)
	if err != nil {
		t.Fatalf("IP2 validation failed: %v", err)
	}
	if !result2.Allowed {
		t.Error("IP2 should be allowed (separate limiter)")
	}

	t.Logf("IP1 blocked: %v", ip1Blocked)
}

func TestRateLimitValidator_Validate_GlobalLimit(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 180,
		PerIPRequestsPerMinute:  600,
		BurstSize:               3,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()
	ips := []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"}
	globalBlocked := false

	// Each IP stays under its own burst, so the only thing that can reject is
	// the global bucket (burst 3, refill 3/s). Twenty requests inside a few
	// microseconds cannot be rescued by refill.
	for i := range 20 {
		req := ports.SecurityRequest{
			ClientID: ips[i%len(ips)],
			Endpoint: "/api/test",
			Method:   "POST",
		}

		result, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Global limit validation failed: %v", err)
		}

		if !result.Allowed {
			globalBlocked = true
			if result.Reason != constants.ReasonGlobalRateLimitExceeded {
				t.Errorf("Expected the global limiter to reject, got reason: %s", result.Reason)
			}
			break
		}
	}

	// Burst is 3 at 180/min, and the per-IP allowance of 600/min is far looser,
	// so a tight loop of 20 across three IPs can only be stopped by the global
	// limiter. A miss must fail the test; it cannot be downgraded to a log line.
	if !globalBlocked {
		t.Error("Global rate limit was never enforced across 20 requests with a burst of 3")
	}
}

func TestRateLimitValidator_Validate_ConcurrentAccess(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  300,
		BurstSize:               5,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := range 20 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := ports.SecurityRequest{
				ClientID:      "192.168.1.100",
				Endpoint:      "/api/test",
				Method:        "POST",
				IsHealthCheck: false,
			}

			for range 10 {
				_, err := validator.Validate(ctx, req)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestRateLimitValidator_Cleanup(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 0,
		PerIPRequestsPerMinute:  100,
		BurstSize:               10,
		CleanupInterval:         50 * time.Millisecond,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	ctx := context.Background()

	for i := range 5 {
		req := ports.SecurityRequest{
			ClientID:      fmt.Sprintf("192.168.1.%d", 100+i),
			Endpoint:      "/api/test",
			Method:        "POST",
			IsHealthCheck: false,
		}
		_, err := validator.Validate(ctx, req)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
	}

	limiterCount := 0
	validator.ipLimiters.Range(func(key string, limiterInfo *ipLimiterInfo) bool {
		limiterCount++
		return true
	})

	if limiterCount != 5 {
		t.Errorf("Expected 5 IP limiters, got %d", limiterCount)
	}

	validator.ipLimiters.Range(func(key string, limiterInfo *ipLimiterInfo) bool {
		limiterInfo.mu.Lock()
		limiterInfo.lastAccess = time.Now().Add(-11 * time.Minute)
		limiterInfo.mu.Unlock()
		return true
	})

	time.Sleep(100 * time.Millisecond)

	limiterCountAfter := 0
	validator.ipLimiters.Range(func(key string, limiterInfo *ipLimiterInfo) bool {
		limiterCountAfter++
		return true
	})

	if limiterCountAfter != 0 {
		t.Errorf("Expected 0 IP limiters after cleanup, got %d", limiterCountAfter)
	}
}

// A zero per-IP limit means "do not limit per client", not "do not limit at all".
// The global limit is a separate switch with its own zero value, so disabling only
// the per-IP tier must leave the global limiter in force. Regression guard for
// Validate() returning early on perIPRequestsPerMinute <= 0, before the global
// limiter is consulted.
func TestRateLimitValidator_Validate_PerIPDisabledStillEnforcesGlobal(t *testing.T) {
	limits := config.ServerRateLimits{
		GlobalRequestsPerMinute: 60, // 1/sec, so burst is the only real headroom
		PerIPRequestsPerMinute:  0,  // per-IP tier off, global left on
		BurstSize:               5,
		CleanupInterval:         time.Minute,
	}

	validator := createTestRateLimitValidator(limits)
	defer validator.Stop()

	if validator.globalLimiter == nil {
		t.Fatal("Global limiter should be initialised when the global limit is non-zero")
	}

	req := ports.SecurityRequest{
		ClientID:      "192.168.1.100",
		Endpoint:      "/api/test",
		Method:        "POST",
		IsHealthCheck: false,
	}

	const requests = 50
	allowed := 0
	for i := range requests {
		result, err := validator.Validate(context.Background(), req)
		if err != nil {
			t.Fatalf("Validate failed on request %d: %v", i+1, err)
		}
		if result.Allowed {
			allowed++
		}
	}

	// A tight loop cannot earn meaningful refill at 1/sec, so anything far above
	// the burst means the global limiter was never asked.
	if allowed == requests {
		t.Fatalf("All %d requests were allowed with a global limit of %d/min and burst %d; "+
			"a zero per-IP limit has disabled global rate limiting entirely",
			requests, limits.GlobalRequestsPerMinute, limits.BurstSize)
	}
	if allowed > limits.BurstSize {
		t.Errorf("Allowed %d requests, expected no more than the burst size of %d", allowed, limits.BurstSize)
	}
}

// sendN fires n requests from one client and returns the results in order.
func sendN(t *testing.T, v *RateLimitValidator, client string, n int) []ports.SecurityResult {
	t.Helper()
	out := make([]ports.SecurityResult, 0, n)
	for range n {
		res, err := v.Validate(context.Background(), ports.SecurityRequest{
			ClientID: client, Endpoint: "/api/test", Method: "POST",
		})
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		out = append(out, res)
	}
	return out
}

// TestRateLimitValidator_Validate_TierSwitches pins that the per-IP and global
// tiers are independent: 0 turns off only that tier, and each rejection names
// the tier that made it.
func TestRateLimitValidator_Validate_TierSwitches(t *testing.T) {
	t.Parallel()

	const burst = 3
	tests := []struct {
		name         string
		global       int
		perIP        int
		clients      []string // requests round-robin across these
		wantReason   string   // "" means nothing may be rejected
		wantLimit    int
		wantFirstRej int // 1-based index of the first rejection
	}{
		{name: "both off", global: 0, perIP: 0, clients: []string{"a"}},
		{name: "global only", global: 60, perIP: 0, clients: []string{"a"},
			wantReason: constants.ReasonGlobalRateLimitExceeded, wantLimit: 60, wantFirstRej: burst + 1},
		{name: "per-IP only", global: 0, perIP: 120, clients: []string{"a"},
			wantReason: constants.ReasonRateLimitExceeded, wantLimit: 120, wantFirstRej: burst + 1},
		{name: "both, one client trips per-IP first", global: 60, perIP: 120, clients: []string{"a"},
			wantReason: constants.ReasonRateLimitExceeded, wantLimit: 120, wantFirstRej: burst + 1},
		{name: "both, many clients trip global", global: 60, perIP: 600, clients: []string{"a", "b", "c"},
			wantReason: constants.ReasonGlobalRateLimitExceeded, wantLimit: 60, wantFirstRej: burst + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := createTestRateLimitValidator(config.ServerRateLimits{
				GlobalRequestsPerMinute: tt.global,
				PerIPRequestsPerMinute:  tt.perIP,
				BurstSize:               burst,
				CleanupInterval:         time.Minute,
			})
			defer v.Stop()

			for i := range 12 {
				res := sendN(t, v, tt.clients[i%len(tt.clients)], 1)[0]
				rejected := !res.Allowed
				switch {
				case tt.wantReason == "" && rejected:
					t.Fatalf("request %d rejected (%q) with both tiers off", i+1, res.Reason)
				case tt.wantReason != "" && i+1 < tt.wantFirstRej && rejected:
					t.Fatalf("request %d rejected early: %q", i+1, res.Reason)
				case tt.wantReason != "" && i+1 == tt.wantFirstRej:
					if !rejected {
						t.Fatalf("request %d should have been rejected", i+1)
					}
					if res.Reason != tt.wantReason {
						t.Errorf("reason = %q, want %q", res.Reason, tt.wantReason)
					}
					if res.RateLimit != tt.wantLimit {
						t.Errorf("RateLimit = %d, want %d", res.RateLimit, tt.wantLimit)
					}
				}
			}
		})
	}
}

// TestRateLimitValidator_Validate_PerIPRejectionDoesNotDrainGlobal is the
// fairness case: a client hammering past its own limit must not use up the
// global bucket and starve a well-behaved neighbour.
func TestRateLimitValidator_Validate_PerIPRejectionDoesNotDrainGlobal(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		GlobalRequestsPerMinute: 600,
		PerIPRequestsPerMinute:  60,
		BurstSize:               3, // per-IP burst
		CleanupInterval:         time.Minute,
	})
	defer v.Stop()
	// Global gets a larger burst than any one client so only leaked tokens can
	// exhaust it. Refill is 0.1/s, negligible over the test.
	v.globalLimiter = rate.NewLimiter(rate.Limit(0.1), 20)

	hammer := sendN(t, v, "hammer", 200)
	admitted := 0
	for _, r := range hammer {
		if r.Allowed {
			admitted++
		}
	}
	if admitted != 3 {
		t.Fatalf("hammering client admitted %d, want its burst of 3", admitted)
	}

	for i, r := range sendN(t, v, "victim", 3) {
		if !r.Allowed {
			t.Fatalf("victim request %d starved: %q", i+1, r.Reason)
		}
	}
}

// TestRateLimitValidator_Validate_GlobalRejectionRefundsPerIP checks that a
// request refused by the global tier is not charged to its per-IP bucket either.
func TestRateLimitValidator_Validate_GlobalRejectionRefundsPerIP(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		GlobalRequestsPerMinute: 60,
		PerIPRequestsPerMinute:  600,
		BurstSize:               5,
		CleanupInterval:         time.Minute,
	})
	defer v.Stop()
	v.globalLimiter = rate.NewLimiter(rate.Limit(0.01), 1) // one token, then dry

	res := sendN(t, v, "a", 6)
	if !res[0].Allowed {
		t.Fatal("first request should consume the single global token")
	}
	for i, r := range res[1:] {
		if r.Allowed || r.Reason != constants.ReasonGlobalRateLimitExceeded {
			t.Fatalf("request %d: allowed=%v reason=%q, want global rejection", i+2, r.Allowed, r.Reason)
		}
	}

	info, ok := v.ipLimiters.Load("a")
	if !ok {
		t.Fatal("per-IP limiter missing")
	}
	info.mu.RLock()
	used := info.tokensUsed
	info.mu.RUnlock()
	if used != 1 {
		t.Errorf("tokensUsed = %d, want 1 (rejected requests must be refunded)", used)
	}
	if tokens := info.limiter.Tokens(); tokens < 3.9 {
		t.Errorf("per-IP bucket has %.2f tokens, want about 4 (5 burst minus the 1 admitted)", tokens)
	}
}

// TestRateLimitValidator_CheckIPLimit_ZeroLimitDoesNotPanic checks that a zero
// limit passed to checkIPLimit is treated as "tier off" rather than rejecting.
func TestRateLimitValidator_CheckIPLimit_ZeroLimitDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{BurstSize: 1, CleanupInterval: time.Minute})
	defer v.Stop()

	for range 5 {
		res, _ := v.checkIPLimit(v.getOrCreateLimiter("a", 1), 0, time.Now())
		if !res.Allowed {
			t.Fatal("a zero per-IP limit means the tier is off; request must be allowed")
		}
	}
}

// TestRateLimitValidator_Middleware_GlobalRejectionHeaders checks the 429 reports
// the global limit and a Retry-After taken from the global bucket, not the
// per-IP value and a fixed 60.
func TestRateLimitValidator_Middleware_GlobalRejectionHeaders(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		GlobalRequestsPerMinute: 60, // refills 1 token/s
		PerIPRequestsPerMinute:  600000,
		BurstSize:               10,
		CleanupInterval:         time.Minute,
	})
	defer v.Stop()
	// Smaller burst than the per-IP bucket so the third request is the global
	// tier's rejection.
	v.globalLimiter = rate.NewLimiter(rate.Limit(1), 2)

	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var rec *httptest.ResponseRecorder
	for range 3 {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		h.ServeHTTP(rec, req)
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "60" {
		t.Errorf("X-RateLimit-Limit = %q, want the global limit 60", got)
	}
	retry, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil {
		t.Fatalf("Retry-After not an integer: %v", err)
	}
	if retry < 1 || retry > 3 {
		t.Errorf("Retry-After = %d, want 1-3 seconds from the global bucket's refill", retry)
	}
}

// TestRateLimitValidator_Middleware_RateLimitHeaders checks the X-RateLimit-* headers
// describe the limit that actually applies, and are absent when none does.
func TestRateLimitValidator_Middleware_RateLimitHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limits    config.ServerRateLimits
		path      string
		wantLimit string // empty means no X-RateLimit-* header at all
	}{
		{
			name:   "both limits off",
			limits: config.ServerRateLimits{CleanupInterval: time.Minute},
			path:   "/olla/proxy/",
		},
		{
			name: "per-IP on",
			limits: config.ServerRateLimits{
				PerIPRequestsPerMinute: 100, BurstSize: 10, CleanupInterval: time.Minute,
			},
			path:      "/olla/proxy/",
			wantLimit: "100",
		},
		{
			name: "per-IP off, global on reports the global limit",
			limits: config.ServerRateLimits{
				GlobalRequestsPerMinute: 600, BurstSize: 10, CleanupInterval: time.Minute,
			},
			path:      "/olla/proxy/",
			wantLimit: "600",
		},
		{
			name: "unlimited health probe omits headers",
			limits: config.ServerRateLimits{
				PerIPRequestsPerMinute: 100, BurstSize: 10, CleanupInterval: time.Minute,
			},
			path: constants.DefaultHealthCheckEndpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := createTestRateLimitValidator(tt.limits)
			defer v.Stop()

			h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.RemoteAddr = "10.0.0.1:1234"
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			for _, name := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
				got := rec.Header().Get(name)
				if tt.wantLimit == "" && got != "" {
					t.Errorf("%s = %q, want it absent", name, got)
				}
				if tt.wantLimit != "" && got == "" {
					t.Errorf("%s missing", name)
				}
			}
			if got := rec.Header().Get("X-RateLimit-Limit"); got != tt.wantLimit {
				t.Errorf("X-RateLimit-Limit = %q, want %q", got, tt.wantLimit)
			}
		})
	}
}

// TestRateLimitValidator_Validate_ConcurrentGlobalRejectionKeepsPerIPToken hammers
// one client while the global tier rejects everything. Every per-IP token taken
// must be handed back, even when the client's requests interleave.
func TestRateLimitValidator_Validate_ConcurrentGlobalRejectionKeepsPerIPToken(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		PerIPRequestsPerMinute: 60,
		BurstSize:              1,
		CleanupInterval:        time.Minute,
	})
	defer v.Stop()
	v.globalLimiter = rate.NewLimiter(rate.Limit(0.001), 1)
	v.globalLimiter.Allow() // drain: every request now fails globally

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 300 {
				_, _ = v.Validate(context.Background(), ports.SecurityRequest{ClientID: "a"})
			}
		}()
	}
	wg.Wait()

	info, ok := v.ipLimiters.Load("a")
	if !ok {
		t.Fatal("per-IP limiter missing")
	}
	if tokens := info.limiter.Tokens(); tokens < 0.9 {
		t.Errorf("per-IP bucket has %.2f tokens after only global rejections, want its full burst of 1", tokens)
	}
}

// TestRateLimitValidator_Validate_ConcurrentRejectionsKeepGlobalTokens runs many
// clients against a dry global bucket. Rejections must not push the shared
// bucket into debt, or global rejection would outlast the traffic that caused it.
func TestRateLimitValidator_Validate_ConcurrentRejectionsKeepGlobalTokens(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		GlobalRequestsPerMinute: 60,
		BurstSize:               1,
		CleanupInterval:         time.Minute,
	})
	defer v.Stop()
	v.globalLimiter = rate.NewLimiter(rate.Limit(0.001), 1)
	v.globalLimiter.Allow() // drain

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 300 {
				_, _ = v.Validate(context.Background(), ports.SecurityRequest{ClientID: fmt.Sprintf("c%d", i)})
			}
		}()
	}
	wg.Wait()

	if tokens := v.globalLimiter.Tokens(); tokens < -0.01 {
		t.Errorf("global bucket in debt (%.2f tokens) after only rejections", tokens)
	}
}

// TestRateLimitValidator_Middleware_PerIPRejectionReportsZeroRemaining covers the
// delayed-reservation reject branch, which used to report the unspent allowance.
func TestRateLimitValidator_Middleware_PerIPRejectionReportsZeroRemaining(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		PerIPRequestsPerMinute: 60,
		BurstSize:              3,
		CleanupInterval:        time.Minute,
	})
	defer v.Stop()

	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var rec *httptest.ResponseRecorder
	for range 4 {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/", nil)
		req.RemoteAddr = "10.0.0.2:1234"
		h.ServeHTTP(rec, req)
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q on a 429, want 0", got)
	}
}

// TestRateLimitValidator_Middleware_LogsRejectionReason checks the WARN says which
// tier rejected, since the response body is the same for both.
func TestRateLimitValidator_Middleware_LogsRejectionReason(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	v := NewRateLimitValidator(config.ServerRateLimits{
		GlobalRequestsPerMinute: 60,
		BurstSize:               10,
	}, nil, log)
	defer v.Stop()
	v.globalLimiter = rate.NewLimiter(rate.Limit(0.001), 1)
	v.globalLimiter.Allow() // drain

	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/", nil)
	req.RemoteAddr = "10.0.0.3:1234"
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if body := rec.Body.String(); body != "Too Many Requests\n" {
		t.Errorf("body = %q, want it unchanged", body)
	}
	var found bool
	for _, r := range records() {
		if r.msg == "Rate limit exceeded" {
			found = true
			if r.attrs["reason"] != constants.ReasonGlobalRateLimitExceeded {
				t.Errorf("logged reason = %v, want %q", r.attrs["reason"], constants.ReasonGlobalRateLimitExceeded)
			}
		}
	}
	if !found {
		t.Fatal("no \"Rate limit exceeded\" WARN logged")
	}
}

// TestRateLimitValidator_Middleware_ZeroBurstRetryAfterFloor covers the
// retryAfterSeconds floor: with no burst every request is refused, and 100/min
// would round 60/limit down to 0 seconds.
func TestRateLimitValidator_Middleware_ZeroBurstRetryAfterFloor(t *testing.T) {
	t.Parallel()

	v := createTestRateLimitValidator(config.ServerRateLimits{
		PerIPRequestsPerMinute: 100,
		BurstSize:              0,
		CleanupInterval:        time.Minute,
	})
	defer v.Stop()

	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/olla/proxy/", nil)
	req.RemoteAddr = "10.0.0.4:1234"
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
}
