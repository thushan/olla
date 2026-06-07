package middleware

import (
	"github.com/rs/cors"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

// DefaultCORSExposedHeaders is the set of X-Olla-* response headers exposed to
// browser clients when the caller has not configured explicit ExposedHeaders.
//
// By default browsers block non-simple response headers from cross-origin reads.
// Olla's routing, model, and sticky-session metadata all live in X-Olla-* headers,
// so browser clients (OpenWebUI, custom dashboards) cannot inspect them without
// this exposure list. We expose the full set rather than a subset to avoid
// surprising omissions when new headers are added to the proxy output.
var DefaultCORSExposedHeaders = []string{
	constants.HeaderXOllaRequestID,
	constants.HeaderXOllaEndpoint,
	constants.HeaderXOllaBackendType,
	constants.HeaderXOllaModel,
	constants.HeaderXOllaResponseTime,
	constants.HeaderXOllaRoutingStrategy,
	constants.HeaderXOllaRoutingDecision,
	constants.HeaderXOllaRoutingReason,
	constants.HeaderXOllaMode,
	constants.HeaderXOllaStickySession,
	constants.HeaderXOllaStickyKeySource,
	constants.HeaderXOllaSessionID,
}

// NewCORS builds an rs/cors handler from Olla's CORS config. It is only constructed
// when CORS is enabled (the caller gates on cfg.Enabled). When ExposedHeaders is empty
// we expose the full X-Olla-* response header set so browser clients can read Olla's
// routing/model metadata, which they otherwise cannot access cross-origin.
func NewCORS(cfg config.CorsConfig) *cors.Cors {
	exposed := cfg.ExposedHeaders
	if len(exposed) == 0 {
		exposed = DefaultCORSExposedHeaders
	}

	return cors.New(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   cfg.AllowedMethods,
		AllowedHeaders:   cfg.AllowedHeaders,
		ExposedHeaders:   exposed,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	})
}
