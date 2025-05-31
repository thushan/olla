package app

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	globalRequestsPerMinute int
	perIPRequestsPerMinute  int
	burstSize               int
	healthRequestsPerMinute int
	trustProxyHeaders       bool
	logger                  *logger.StyledLogger

	globalLimiter *rate.Limiter
	ipLimiters    sync.Map // map[string]*ipLimiterInfo
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

type ipLimiterInfo struct {
	limiter      *rate.Limiter
	lastAccess   time.Time
	tokensUsed   int
	windowStart  time.Time
	requestLimit int
	mu           sync.RWMutex
}

type RateLimitResult struct {
	Allowed    bool
	RetryAfter int
	Limit      int
	Remaining  int
	ResetTime  time.Time
}

func NewRateLimiter(limits config.ServerRateLimits, logger *logger.StyledLogger) *RateLimiter {
	rl := &RateLimiter{
		globalRequestsPerMinute: limits.GlobalRequestsPerMinute,
		perIPRequestsPerMinute:  limits.PerIPRequestsPerMinute,
		burstSize:               limits.BurstSize,
		healthRequestsPerMinute: limits.HealthRequestsPerMinute,
		trustProxyHeaders:       limits.IPExtractionTrustProxy,
		logger:                  logger,
		stopCleanup:             make(chan struct{}),
	}

	// set global limiter if global limiting is enabled
	if limits.GlobalRequestsPerMinute > 0 {
		globalRate := rate.Limit(float64(limits.GlobalRequestsPerMinute) / 60.0) // per second
		rl.globalLimiter = rate.NewLimiter(globalRate, limits.BurstSize)
	}

	if limits.CleanupInterval > 0 {
		rl.cleanupTicker = time.NewTicker(limits.CleanupInterval)
		go rl.cleanupRoutine()
	}

	return rl
}

func (rl *RateLimiter) Stop() {
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
	close(rl.stopCleanup)
}

func (rl *RateLimiter) Middleware(isHealthEndpoint bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := util.GetClientIP(r, rl.trustProxyHeaders)

			var limit int
			// TODO: make this more of ignroed_routes later
			// we're doing individual ones for now
			// let's look at a bucket of ignored routes later
			if isHealthEndpoint {
				limit = rl.healthRequestsPerMinute
			} else {
				limit = rl.perIPRequestsPerMinute
			}

			result := rl.checkRateLimit(clientIP, limit, isHealthEndpoint)

			// ALWAYS set headers, even if rate limiting is disabled
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

			rl.logger.Debug("Request Rate request", "limit", result.Limit, "remaining", result.Remaining)

			if !result.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(result.RetryAfter))

				rl.logger.Warn("Rate limit exceeded",
					"client_ip", clientIP,
					"method", r.Method,
					"path", r.URL.Path,
					"limit", result.Limit,
					"retry_after", result.RetryAfter)

				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) checkRateLimit(clientIP string, limit int, isHealthEndpoint bool) RateLimitResult {
	now := time.Now()

	// Handle disabled rate limiting
	if limit <= 0 {
		return RateLimitResult{
			Allowed:   true,
			Limit:     0,
			Remaining: 0,
			ResetTime: now.Add(time.Minute),
		}
	}

	// Check glibal limit first if enabled
	if rl.globalLimiter != nil {
		reservation := rl.globalLimiter.Reserve()
		if !reservation.OK() || reservation.Delay() > 0 {
			if reservation.Delay() > 0 {
				reservation.Cancel()
			}
			return RateLimitResult{
				Allowed:    false,
				RetryAfter: 60,
				Limit:      limit,
				Remaining:  0,
				ResetTime:  now.Add(time.Minute),
			}
		}
	}

	return rl.checkIPLimit(clientIP, limit, now, isHealthEndpoint)
}

func (rl *RateLimiter) checkIPLimit(clientIP string, limit int, now time.Time, isHealthEndpoint bool) RateLimitResult {
	// we'll use this as the key for the limiter
	bucketKey := clientIP
	if isHealthEndpoint {
		bucketKey = clientIP + ":health"
	}

	limiterInfo := rl.getOrCreateLimiter(bucketKey, limit)
	limiterInfo.mu.Lock()
	limiterInfo.lastAccess = now

	// Check if we need to reset the window (every minute)
	if now.Sub(limiterInfo.windowStart) >= time.Minute {
		limiterInfo.windowStart = now
		limiterInfo.tokensUsed = 0
	}

	limiter := limiterInfo.limiter
	limiterInfo.mu.Unlock()

	// Try to get a token
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return RateLimitResult{
			Allowed:    false,
			RetryAfter: 60 / limit,
			Limit:      limit,
			Remaining:  0,
			ResetTime:  now.Add(time.Minute),
		}
	}

	delay := reservation.Delay()
	if delay > 0 {
		// Cancel the reservation since we can't wait
		reservation.Cancel()

		limiterInfo.mu.RLock()
		remaining := rl.calculateRemaining(limiterInfo, limit, now)
		limiterInfo.mu.RUnlock()

		return RateLimitResult{
			Allowed:    false,
			RetryAfter: int(delay.Seconds()) + 1,
			Limit:      limit,
			Remaining:  remaining,
			ResetTime:  now.Add(time.Minute),
		}
	}

	// Request allowed - update token usage tracking
	limiterInfo.mu.Lock()
	limiterInfo.tokensUsed++
	remaining := rl.calculateRemaining(limiterInfo, limit, now)
	limiterInfo.mu.Unlock()

	return RateLimitResult{
		Allowed:   true,
		Limit:     limit,
		Remaining: remaining,
		ResetTime: now.Add(time.Minute),
	}
}

func (rl *RateLimiter) calculateRemaining(limiterInfo *ipLimiterInfo, limit int, now time.Time) int {
	remaining := limit - limiterInfo.tokensUsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

func (rl *RateLimiter) getOrCreateLimiter(key string, limit int) *ipLimiterInfo {
	value, _ := rl.ipLimiters.LoadOrStore(key, &ipLimiterInfo{
		limiter:      rate.NewLimiter(rate.Limit(float64(limit)/60.0), rl.burstSize), // per second
		lastAccess:   time.Now(),
		tokensUsed:   0,
		windowStart:  time.Now(),
		requestLimit: limit,
	})

	return value.(*ipLimiterInfo)
}

func (rl *RateLimiter) cleanupRoutine() {
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-rl.cleanupTicker.C:
			rl.cleanupOldLimiters()
		}
	}
}

func (rl *RateLimiter) cleanupOldLimiters() {
	cutoff := time.Now().Add(-10 * time.Minute)

	rl.ipLimiters.Range(func(key, value interface{}) bool {
		limiterInfo := value.(*ipLimiterInfo)

		limiterInfo.mu.RLock()
		lastAccess := limiterInfo.lastAccess
		limiterInfo.mu.RUnlock()

		if lastAccess.Before(cutoff) {
			rl.ipLimiters.Delete(key)
		}
		return true
	})
}
