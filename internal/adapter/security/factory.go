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
func NewSecurityServices(cfg *config.Config, statsCollector ports.StatsCollector, logger *logger.StyledLogger) (*Services, *Adapters) {
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

func (sa *Adapters) CreateChainMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rateLimitMiddleware := sa.RateLimit.CreateMiddleware()
			sizeMiddleware := sa.SizeValidation.CreateMiddleware()

			handler := rateLimitMiddleware(sizeMiddleware(next))
			handler.ServeHTTP(w, r)
		})
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
