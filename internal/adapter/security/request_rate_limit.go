package security

/*
				Olla Security Adapter - Rate Limit Validator
	RateLimitValidator enforces global and per-IP rate limits using token buckets.
	It supports custom limits for health check endpoints and includes automatic
	cleanup of stale IP limiters. We're trying to keep it simple and efficient.

	It's thread-safe and designed for high-throughput environments.

	References:
	- https://pkg.go.dev/golang.org/x/time/rate
	- https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/
*/

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"golang.org/x/time/rate"
)

// reasonGlobalRateLimit distinguishes a global-bucket rejection from a per-IP one.
const reasonGlobalRateLimit = "Global rate limit exceeded"

type RateLimitValidator struct {
	metrics ports.SecurityMetricsService
	logger  logger.StyledLogger

	globalLimiter           *rate.Limiter
	cleanupTicker           *time.Ticker
	stopCleanup             chan struct{}
	rejectLog               *logger.Throttle
	ipLimiters              *xsync.Map[string, *ipLimiterInfo]
	trustedCIDRs            []*net.IPNet
	globalRequestsPerMinute int
	perIPRequestsPerMinute  int
	burstSize               int
	healthRequestsPerMinute int
	stopOnce                sync.Once
	trustProxyHeaders       bool
}

// ipCharge is the per-IP token a request took. It is a value (not a closure)
// so the admit path, which is every successful request, stays allocation-free.
type ipCharge struct {
	at          time.Time
	reservation *rate.Reservation
	info        *ipLimiterInfo
}

// refund hands the token back; a no-op on the zero value.
func (c ipCharge) refund() {
	if c.reservation == nil {
		return
	}
	c.reservation.CancelAt(c.at)
	c.info.mu.Lock()
	if c.info.tokensUsed > 0 {
		c.info.tokensUsed--
	}
	c.info.mu.Unlock()
}

type ipLimiterInfo struct {
	lastAccess   time.Time
	windowStart  time.Time
	limiter      *rate.Limiter
	tokensUsed   int
	requestLimit int
	mu           sync.RWMutex
	// flow serialises one client's per-IP reserve, global decision and refund.
	// x/time's CancelAt only restores a token exactly when no other reservation
	// on that limiter has landed since, so interleaved requests from one client
	// could lose a token on refund. Other clients are unaffected.
	flow sync.Mutex
}

func NewRateLimitValidator(limits config.ServerRateLimits, metrics ports.SecurityMetricsService, log logger.StyledLogger) *RateLimitValidator {
	rl := &RateLimitValidator{
		globalRequestsPerMinute: limits.GlobalRequestsPerMinute,
		perIPRequestsPerMinute:  limits.PerIPRequestsPerMinute,
		burstSize:               limits.BurstSize,
		healthRequestsPerMinute: limits.HealthRequestsPerMinute,
		trustProxyHeaders:       limits.TrustProxyHeaders,
		trustedCIDRs:            limits.TrustedProxyCIDRsParsed,
		metrics:                 metrics,
		logger:                  log,
		stopCleanup:             make(chan struct{}),
		ipLimiters:              xsync.NewMap[string, *ipLimiterInfo](),
	}

	// A client over its limit can be rejected at wire speed, so the WARN is
	// sampled per client IP. The violation counters still see every rejection.
	rl.rejectLog = logger.NewRejectThrottle(log, "Rate limit exceeded", "rate_limit", "client_ip", nil)

	if limits.GlobalRequestsPerMinute > 0 {
		globalRate := rate.Limit(float64(limits.GlobalRequestsPerMinute) / 60.0)
		rl.globalLimiter = rate.NewLimiter(globalRate, limits.BurstSize)
	}

	if limits.CleanupInterval > 0 {
		rl.cleanupTicker = time.NewTicker(limits.CleanupInterval)
		go rl.cleanupRoutine()
	}

	return rl
}

func (rl *RateLimitValidator) Name() string {
	return "rate_limit"
}

// Validate checks if the request should be allowed under current rate limits.
// The per-IP and global tiers are independent switches (0 disables only that
// tier). The per-IP tier is consulted first so a client already over its own
// limit cannot burn global tokens and starve everyone else. If the per-IP tier
// admits the request but the global tier rejects it, the per-IP token is handed
// back so a rejected request is not charged to either bucket.
// Thread safe.
func (rl *RateLimitValidator) Validate(_ context.Context, req ports.SecurityRequest) (ports.SecurityResult, error) {
	now := time.Now()

	var limit int
	if req.IsHealthCheck {
		limit = rl.healthRequestsPerMinute
	} else {
		limit = rl.perIPRequestsPerMinute
	}

	// A zero health limit has always meant "health probes are not limited",
	// including by the global tier, so keep that.
	if req.IsHealthCheck && limit <= 0 {
		return ports.SecurityResult{
			Allowed:   true,
			ResetTime: now.Add(time.Minute),
		}, nil
	}

	result := ports.SecurityResult{Allowed: true, ResetTime: now.Add(time.Minute)}
	var charge ipCharge
	if limit > 0 {
		bucketKey := req.ClientID
		if req.IsHealthCheck {
			bucketKey += ":health"
		}
		info := rl.getOrCreateLimiter(bucketKey, limit)
		info.flow.Lock()
		defer info.flow.Unlock()
		result, charge = rl.checkIPLimit(info, limit, now)
		if !result.Allowed {
			return result, nil
		}
	}

	if rl.globalLimiter != nil {
		// Allow charges the bucket only when it admits, so concurrent rejections
		// cannot leave the shared global bucket in debt (a Reserve/Cancel pair
		// is not atomic across clients, and global is shared by all of them).
		if !rl.globalLimiter.Allow() {
			charge.refund()
			retryAfter := rl.globalRetryAfter()
			return ports.SecurityResult{
				Allowed:    false,
				RetryAfter: retryAfter,
				RateLimit:  rl.globalRequestsPerMinute,
				Remaining:  0,
				ResetTime:  now.Add(time.Minute),
				Reason:     reasonGlobalRateLimit,
			}, nil
		}
		if limit <= 0 {
			result.RateLimit = rl.globalRequestsPerMinute
			result.Remaining = int(rl.globalLimiter.Tokens())
		}
	}

	return result, nil
}

// checkIPLimit handles per-IP rate enforcement, including health check separation.
// Returns a SecurityResult with rate metadata and retry after guidance, plus a
// charge that can return the consumed token when the request is later rejected
// by the global tier (zero value unless the request was admitted).
func (rl *RateLimitValidator) checkIPLimit(limiterInfo *ipLimiterInfo, limit int, now time.Time) (ports.SecurityResult, ipCharge) {
	// Callers gate on limit > 0, but 60/limit below would panic if that ever slipped.
	if limit <= 0 {
		return ports.SecurityResult{Allowed: true, ResetTime: now.Add(time.Minute)}, ipCharge{}
	}

	limiterInfo.mu.Lock()
	limiterInfo.lastAccess = now

	if now.Sub(limiterInfo.windowStart) >= time.Minute {
		limiterInfo.windowStart = now
		limiterInfo.tokensUsed = 0
	}

	limiter := limiterInfo.limiter
	limiterInfo.mu.Unlock()

	// Reserve at an explicit instant so the rollback can cancel at the same one:
	// CancelAt only refunds when the reservation is not already due, and a
	// zero-delay reservation is due the moment it is made.
	reservedAt := time.Now()
	reservation := limiter.ReserveN(reservedAt, 1)
	if !reservation.OK() {
		return ports.SecurityResult{
			Allowed:    false,
			RetryAfter: 60 / limit,
			RateLimit:  limit,
			Remaining:  0,
			ResetTime:  now.Add(time.Minute),
			Reason:     "Rate limit exceeded",
		}, ipCharge{}
	}

	delay := reservation.Delay()
	if delay > 0 {
		reservation.Cancel()

		return ports.SecurityResult{
			Allowed:    false,
			RetryAfter: int(delay.Seconds()) + 1,
			RateLimit:  limit,
			Remaining:  0,
			ResetTime:  now.Add(time.Minute),
			Reason:     "Rate limit exceeded",
		}, ipCharge{}
	}

	limiterInfo.mu.Lock()
	limiterInfo.tokensUsed++
	remaining := rl.calculateRemaining(limiterInfo, limit)
	limiterInfo.mu.Unlock()

	return ports.SecurityResult{
		Allowed:   true,
		RateLimit: limit,
		Remaining: remaining,
		ResetTime: now.Add(time.Minute),
	}, ipCharge{reservation: reservation, at: reservedAt, info: limiterInfo}
}

// globalRetryAfter estimates whole seconds until the global bucket holds a token.
// It falls back to the full window when the bucket can never refill (zero burst).
func (rl *RateLimitValidator) globalRetryAfter() int {
	lim := rl.globalLimiter
	if lim.Burst() < 1 || lim.Limit() <= 0 {
		return 60
	}
	deficit := 1 - lim.Tokens()
	if deficit < 0 {
		deficit = 0
	}
	return int(deficit/float64(lim.Limit())) + 1
}

func (rl *RateLimitValidator) calculateRemaining(limiterInfo *ipLimiterInfo, limit int) int {
	remaining := limit - limiterInfo.tokensUsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

func (rl *RateLimitValidator) getOrCreateLimiter(key string, limit int) *ipLimiterInfo {
	limiterInfo, _ := rl.ipLimiters.LoadOrCompute(key, func() (newValue *ipLimiterInfo, cancel bool) {
		now := time.Now()
		return &ipLimiterInfo{
			limiter:      rate.NewLimiter(rate.Limit(float64(limit)/60.0), rl.burstSize),
			tokensUsed:   0,
			lastAccess:   now,
			windowStart:  now,
			requestLimit: limit,
		}, false
	})
	return limiterInfo
}

func (rl *RateLimitValidator) cleanupRoutine() {
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-rl.cleanupTicker.C:
			rl.cleanupOldLimiters()
		}
	}
}

// cleanupOldLimiters removes IP limiter entries that haven't been accessed recently.
// Called periodically from a background goroutine.
func (rl *RateLimitValidator) cleanupOldLimiters() {
	cutoff := time.Now().Add(-10 * time.Minute)

	rl.ipLimiters.Range(func(key string, limiterInfo *ipLimiterInfo) bool {
		limiterInfo.mu.RLock()
		lastAccess := limiterInfo.lastAccess
		limiterInfo.mu.RUnlock()

		if lastAccess.Before(cutoff) {
			rl.ipLimiters.Delete(key)
		}
		return true
	})
}

func (rl *RateLimitValidator) Stop() {
	// Ensure cleanup ticker is stopped and the channel
	// is closed only once, we test this in unit tests.
	rl.stopOnce.Do(func() {
		if rl.cleanupTicker != nil {
			rl.cleanupTicker.Stop()
		}
		close(rl.stopCleanup)
		rl.rejectLog.Flush()
	})
}

func (rl *RateLimitValidator) CreateMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := util.GetClientIP(r, rl.trustProxyHeaders, rl.trustedCIDRs)

			//  This isn't so nice to do this but we want to ignore the healthcheck endpoint
			isHealthEndpoint := r.URL.Path == constants.DefaultHealthCheckEndpoint

			req := ports.SecurityRequest{
				ClientID:      clientIP,
				Endpoint:      r.URL.Path,
				Method:        r.Method,
				IsHealthCheck: isHealthEndpoint,
			}

			result, err := rl.Validate(r.Context(), req)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// we need to make sure we _always_ send these headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.RateLimit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

			if !result.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(result.RetryAfter))

				if rl.metrics != nil {
					violation := ports.SecurityViolation{
						ClientID:      clientIP,
						ViolationType: constants.ViolationRateLimit,
						Endpoint:      r.URL.Path,
						Size:          0,
						Timestamp:     time.Now(),
					}
					_ = rl.metrics.RecordViolation(r.Context(), violation)
				}

				if rl.rejectLog.Allow(clientIP) {
					rl.logger.Warn("Rate limit exceeded",
						"client_ip", clientIP,
						"method", r.Method,
						"path", r.URL.Path,
						"limit", result.RateLimit,
						"retry_after", result.RetryAfter,
						"reason", result.Reason)
				}

				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
