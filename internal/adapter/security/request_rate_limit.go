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

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"golang.org/x/time/rate"
)

type RateLimitValidator struct {
	metrics ports.SecurityMetricsService
	logger  logger.StyledLogger

	globalLimiter           *rate.Limiter
	cleanupTicker           *time.Ticker
	stopCleanup             chan struct{}
	ipLimiters              sync.Map
	trustedCIDRs            []*net.IPNet
	globalRequestsPerMinute int
	perIPRequestsPerMinute  int
	burstSize               int
	healthRequestsPerMinute int
	stopOnce                sync.Once
	trustProxyHeaders       bool
}

type ipLimiterInfo struct {
	lastAccess   time.Time
	windowStart  time.Time
	limiter      *rate.Limiter
	tokensUsed   int
	requestLimit int
	mu           sync.RWMutex
}

func NewRateLimitValidator(limits config.ServerRateLimits, metrics ports.SecurityMetricsService, logger logger.StyledLogger) *RateLimitValidator {
	rl := &RateLimitValidator{
		globalRequestsPerMinute: limits.GlobalRequestsPerMinute,
		perIPRequestsPerMinute:  limits.PerIPRequestsPerMinute,
		burstSize:               limits.BurstSize,
		healthRequestsPerMinute: limits.HealthRequestsPerMinute,
		trustProxyHeaders:       limits.TrustProxyHeaders,
		trustedCIDRs:            limits.TrustedProxyCIDRsParsed,
		metrics:                 metrics,
		logger:                  logger,
		stopCleanup:             make(chan struct{}),
	}

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
// It applies global and per-IP rules and returns a detailed SecurityResult.
// Thread safe.
func (rl *RateLimitValidator) Validate(ctx context.Context, req ports.SecurityRequest) (ports.SecurityResult, error) {
	now := time.Now()

	var limit int
	if req.IsHealthCheck {
		limit = rl.healthRequestsPerMinute
	} else {
		limit = rl.perIPRequestsPerMinute
	}

	if limit <= 0 {
		return ports.SecurityResult{
			Allowed:   true,
			RateLimit: 0,
			Remaining: 0,
			ResetTime: now.Add(time.Minute),
		}, nil
	}

	if rl.globalLimiter != nil {
		reservation := rl.globalLimiter.Reserve()
		if !reservation.OK() || reservation.Delay() > 0 {
			if reservation.Delay() > 0 {
				reservation.Cancel()
			}
			return ports.SecurityResult{
				Allowed:    false,
				RetryAfter: 60,
				RateLimit:  limit,
				Remaining:  0,
				ResetTime:  now.Add(time.Minute),
				Reason:     "Rate limit exceeded",
			}, nil
		}
	}

	return rl.checkIPLimit(req.ClientID, limit, now, req.IsHealthCheck), nil
}

// checkIPLimit handles per-IP rate enfrecment, including health check separation.
// Returns a SecurityResult with rate metadata and retry after guidance.
func (rl *RateLimitValidator) checkIPLimit(clientIP string, limit int, now time.Time, isHealthEndpoint bool) ports.SecurityResult {
	bucketKey := clientIP
	if isHealthEndpoint {
		bucketKey = clientIP + ":health"
	}

	limiterInfo := rl.getOrCreateLimiter(bucketKey, limit)
	limiterInfo.mu.Lock()
	limiterInfo.lastAccess = now

	if now.Sub(limiterInfo.windowStart) >= time.Minute {
		limiterInfo.windowStart = now
		limiterInfo.tokensUsed = 0
	}

	limiter := limiterInfo.limiter
	limiterInfo.mu.Unlock()

	reservation := limiter.Reserve()
	if !reservation.OK() {
		return ports.SecurityResult{
			Allowed:    false,
			RetryAfter: 60 / limit,
			RateLimit:  limit,
			Remaining:  0,
			ResetTime:  now.Add(time.Minute),
			Reason:     "Rate limit exceeded",
		}
	}

	delay := reservation.Delay()
	if delay > 0 {
		reservation.Cancel()

		limiterInfo.mu.RLock()
		remaining := rl.calculateRemaining(limiterInfo, limit)
		limiterInfo.mu.RUnlock()

		return ports.SecurityResult{
			Allowed:    false,
			RetryAfter: int(delay.Seconds()) + 1,
			RateLimit:  limit,
			Remaining:  remaining,
			ResetTime:  now.Add(time.Minute),
			Reason:     "Rate limit exceeded",
		}
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
	}
}

func (rl *RateLimitValidator) calculateRemaining(limiterInfo *ipLimiterInfo, limit int) int {
	remaining := limit - limiterInfo.tokensUsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

func (rl *RateLimitValidator) getOrCreateLimiter(key string, limit int) *ipLimiterInfo {
	newLimiter := &ipLimiterInfo{
		limiter:      rate.NewLimiter(rate.Limit(float64(limit)/60.0), rl.burstSize),
		lastAccess:   time.Now(),
		tokensUsed:   0,
		windowStart:  time.Now(),
		requestLimit: limit,
	}

	actual, _ := rl.ipLimiters.LoadOrStore(key, newLimiter)

	if limiterInfo, ok := actual.(*ipLimiterInfo); ok {
		return limiterInfo
	}
	// shouldn't happen but keeps golangci-lint happy
	return newLimiter
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

	rl.ipLimiters.Range(func(key, value interface{}) bool {
		limiterInfo, ok := value.(*ipLimiterInfo)
		if !ok {
			return true // skip corrropt entry
		}
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
	})
}

func (rl *RateLimitValidator) CreateMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := util.GetClientIP(r, rl.trustProxyHeaders, rl.trustedCIDRs)

			//  Thisis n't so nice to do this but we want to ignore the Heatcheck endpoint
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

				rl.logger.Warn("Rate limit exceeded",
					"client_ip", clientIP,
					"method", r.Method,
					"path", r.URL.Path,
					"limit", result.RateLimit,
					"retry_after", result.RetryAfter)

				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
