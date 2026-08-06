package security

import (
	"net/http"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

type Services struct {
	Chain   *ports.SecurityChain
	Metrics ports.SecurityMetricsService
}

type Adapters struct {
	RateLimit      *RateLimitValidator
	SizeValidation *SizeValidator
	Metrics        *MetricsAdapter
	Chain          *ports.SecurityChain
}

// NewSecurityServices Creates and wires security validators so they're easy to chain and c onsume
func NewSecurityServices(cfg *config.Config, statsCollector ports.StatsCollector, logger logger.StyledLogger) (*Services, *Adapters) {
	metricsAdapter := NewSecurityMetricsAdapter(statsCollector, logger)
	rateLimitValidator := NewRateLimitValidator(cfg.Server.RateLimits, metricsAdapter, logger)
	sizeValidator := NewSizeValidator(cfg.Server.RequestLimits, metricsAdapter, logger)

	chain := ports.NewSecurityChain(
		rateLimitValidator, /* We start with rate limiting */
		sizeValidator,      /* if we pass that, we can check size */
	)

	services := &Services{
		Chain:   chain,
		Metrics: metricsAdapter,
	}

	adapters := &Adapters{
		RateLimit:      rateLimitValidator,
		SizeValidation: sizeValidator,
		Metrics:        metricsAdapter,
		Chain:          chain,
	}

	return services, adapters
}

func (sa *Adapters) Stop() {
	if sa.RateLimit != nil {
		sa.RateLimit.Stop()
	}
}

// CreateChainMiddleware builds the rate-limit -> size-validation chain once,
// at wiring time. Building it fresh on every request (as this used to do)
// allocated four closures per request for no reason - the underlying
// validators don't change after startup.
func (sa *Adapters) CreateChainMiddleware() func(http.Handler) http.Handler {
	rateLimitMiddleware := sa.RateLimit.CreateMiddleware()
	sizeMiddleware := sa.SizeValidation.CreateMiddleware()

	return func(next http.Handler) http.Handler {
		return rateLimitMiddleware(sizeMiddleware(next))
	}
}

func (sa *Adapters) CreateRateLimitMiddleware() func(http.Handler) http.Handler {
	if sa.RateLimit != nil {
		return sa.RateLimit.CreateMiddleware()
	}
	return func(next http.Handler) http.Handler {
		return next
	}
}

// CreateSizeMiddleware returns a middleware that enforces request-size limits
// for non-proxy routes (status, health, stats). Unlike CreateMiddleware, this
// variant actively drains chunked bodies (Content-Length == -1) up to the cap
// so that oversized requests are rejected even when the handler never reads the
// body. Proxy routes must not use this path — they stream large bodies and must
// not be buffered.
func (sa *Adapters) CreateSizeMiddleware() func(http.Handler) http.Handler {
	if sa.SizeValidation != nil {
		return sa.SizeValidation.CreateNonProxyMiddleware()
	}
	return func(next http.Handler) http.Handler {
		return next
	}
}
