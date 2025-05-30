package app

import (
	"github.com/thushan/olla/internal/util"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
)

type RateLimiter struct {
	globalRequestsPerMinute int
	perIPRequestsPerMinute  int
	burstSize               int
	healthRequestsPerMinute int
	trustProxyHeaders       bool
	logger                  *logger.StyledLogger

	globalTokens     int64
	lastGlobalRefill int64
	ipBuckets        sync.Map
	cleanupTicker    *time.Ticker
	stopCleanup      chan struct{}
}

type ipBucket struct {
	tokens     int64
	lastRefill int64
	lastAccess int64
}

type RateLimitResult struct {
	Allowed    bool
	RetryAfter int
	Limit      int
	Remaining  int
	ResetTime  time.Time
}

func NewRateLimiter(limits config.ServerRateLimits, logger *logger.StyledLogger) *RateLimiter {

	/*
		if limits.AutoScale {
			limits.BurstSize = determineBurstSize(limits.PerIPRequestsPerMinute, limits.GlobalRequestsPerMinute)
		}
	*/
	// configure global tokens to burst size if global limit is enabled
	initialGlobalTokens := int64(0)
	if limits.GlobalRequestsPerMinute > 0 {
		initialGlobalTokens = int64(limits.BurstSize)
	}

	rl := &RateLimiter{
		globalRequestsPerMinute: limits.GlobalRequestsPerMinute,
		perIPRequestsPerMinute:  limits.PerIPRequestsPerMinute,
		burstSize:               limits.BurstSize,
		healthRequestsPerMinute: limits.HealthRequestsPerMinute,
		trustProxyHeaders:       limits.IPExtractionTrustProxy,
		logger:                  logger,
		globalTokens:            initialGlobalTokens,
		lastGlobalRefill:        time.Now().UnixNano(),
		stopCleanup:             make(chan struct{}),
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
			if isHealthEndpoint {
				limit = rl.healthRequestsPerMinute
			} else {
				limit = rl.perIPRequestsPerMinute
			}

			result := rl.checkRateLimit(clientIP, limit)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

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

func (rl *RateLimiter) checkRateLimit(clientIP string, limit int) RateLimitResult {
	now := time.Now()
	nowNano := now.UnixNano()

	// Check global limit first if enabled
	if rl.globalRequestsPerMinute > 0 {
		if !rl.checkGlobalLimit(nowNano) {
			return RateLimitResult{
				Allowed:    false,
				RetryAfter: 60,
				Limit:      rl.globalRequestsPerMinute,
				Remaining:  0,
				ResetTime:  now.Add(time.Minute),
			}
		}
	}

	return rl.checkIPLimit(clientIP, limit, nowNano, now)
}

func (rl *RateLimiter) checkGlobalLimit(nowNano int64) bool {
	if rl.globalRequestsPerMinute <= 0 {
		return true
	}

	rl.refillGlobalTokens(nowNano)

	for {
		tokens := atomic.LoadInt64(&rl.globalTokens)
		if tokens <= 0 {
			return false
		}

		if atomic.CompareAndSwapInt64(&rl.globalTokens, tokens, tokens-1) {
			return true
		}
		// CAS failed, retry
	}
}

func (rl *RateLimiter) refillGlobalTokens(nowNano int64) {
	lastRefill := atomic.LoadInt64(&rl.lastGlobalRefill)
	elapsed := nowNano - lastRefill

	if elapsed < 1e9 { // Less than 1 second
		return
	}

	if !atomic.CompareAndSwapInt64(&rl.lastGlobalRefill, lastRefill, nowNano) {
		return
	}

	tokensToAdd := elapsed * int64(rl.globalRequestsPerMinute) / (60 * 1e9)
	if tokensToAdd > 0 {
		for {
			currentTokens := atomic.LoadInt64(&rl.globalTokens)
			newTokens := currentTokens + tokensToAdd
			maxTokens := int64(rl.burstSize)

			if newTokens > maxTokens {
				newTokens = maxTokens
			}

			if atomic.CompareAndSwapInt64(&rl.globalTokens, currentTokens, newTokens) {
				break
			}
		}
	}
}

func (rl *RateLimiter) checkIPLimit(clientIP string, limit int, nowNano int64, now time.Time) RateLimitResult {
	if limit <= 0 {
		return RateLimitResult{
			Allowed:   true,
			Limit:     limit,
			Remaining: limit,
			ResetTime: now.Add(time.Minute),
		}
	}

	// Create unique key for IP + endpoint type combination
	bucketKey := clientIP
	if limit == rl.healthRequestsPerMinute {
		bucketKey = clientIP + ":health"
	}

	// Use the smaller of limit or burstSize for initial tokens
	initialTokens := int64(min(limit, rl.burstSize))

	value, _ := rl.ipBuckets.LoadOrStore(bucketKey, &ipBucket{
		tokens:     initialTokens,
		lastRefill: nowNano,
		lastAccess: nowNano,
	})

	bucket := value.(*ipBucket)

	// Always refill before checking
	rl.refillIPTokens(bucket, limit, nowNano)

	// Try to consume a token
	for {
		tokens := atomic.LoadInt64(&bucket.tokens)
		if tokens <= 0 {
			// Calculate retry after based on token refill rate
			tokensPerSecond := float64(limit) / 60.0
			retryAfter := int(1.0 / tokensPerSecond)
			if retryAfter < 1 {
				retryAfter = 1
			}

			return RateLimitResult{
				Allowed:    false,
				RetryAfter: retryAfter,
				Limit:      limit,
				Remaining:  0,
				ResetTime:  now.Add(time.Minute),
			}
		}

		if atomic.CompareAndSwapInt64(&bucket.tokens, tokens, tokens-1) {
			atomic.StoreInt64(&bucket.lastAccess, nowNano)

			remaining := int(tokens - 1)
			if remaining < 0 {
				remaining = 0
			}

			return RateLimitResult{
				Allowed:   true,
				Limit:     limit,
				Remaining: remaining,
				ResetTime: now.Add(time.Minute),
			}
		}
	}
}

func (rl *RateLimiter) refillIPTokens(bucket *ipBucket, limit int, nowNano int64) {
	lastRefill := atomic.LoadInt64(&bucket.lastRefill)
	elapsed := nowNano - lastRefill

	// Only refill if at least 1 second has passed
	if elapsed < 1e9 {
		return
	}

	if !atomic.CompareAndSwapInt64(&bucket.lastRefill, lastRefill, nowNano) {
		return
	}

	// Calculate tokens to add based on rate (tokens per second)
	tokensToAdd := elapsed * int64(limit) / (60 * 1e9)
	if tokensToAdd > 0 {
		for {
			currentTokens := atomic.LoadInt64(&bucket.tokens)
			newTokens := currentTokens + tokensToAdd
			maxTokens := int64(rl.burstSize)

			if newTokens > maxTokens {
				newTokens = maxTokens
			}

			if atomic.CompareAndSwapInt64(&bucket.tokens, currentTokens, newTokens) {
				break
			}
		}
	}
}

func (rl *RateLimiter) cleanupRoutine() {
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-rl.cleanupTicker.C:
			rl.cleanupOldBuckets()
		}
	}
}

func (rl *RateLimiter) cleanupOldBuckets() {
	cutoff := time.Now().Add(-10 * time.Minute).UnixNano()

	rl.ipBuckets.Range(func(key, value interface{}) bool {
		bucket := value.(*ipBucket)
		lastAccess := atomic.LoadInt64(&bucket.lastAccess)

		if lastAccess < cutoff {
			rl.ipBuckets.Delete(key)
		}
		return true
	})
}
