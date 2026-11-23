package constants

import (
	"context"

	"github.com/thushan/olla/internal/core/domain"
)

// contextKey is a private type for context keys to prevent collisions with other packages.
// Storage role only; never used as a slog or metadata key. Where a key's spelling also
// serves such a role, a separate untyped string constant keeps it (see SlogRequestIdKey
// and InspectionMetaModelAliasMapKey).
type contextKey string

const (
	// ContextRoutePrefixKey carries the prefix a registered route strips from the
	// request path before forwarding. The "route_prefix" spelling also serves as
	// the proxy Configuration's path-prefix sentinel default (proxy package);
	// that role is a plain string on purpose and is never used as a context key.
	ContextRoutePrefixKey = contextKey("route_prefix") // injected into the context to allow stripping prefixes for proxy calls

	// ContextRequestIdKey is generated per proxy-handler request. The middleware
	// package keeps its own typed "request_id" key; the two are distinct context
	// slots and are deliberately not merged. The log-record role of the same
	// spelling is SlogRequestIdKey.
	ContextRequestIdKey    = contextKey("request_id")    // generated each proxy_handler request for the request ID
	ContextRequestTimeKey  = contextKey("request_time")  // generated each proxy_handler request to track the time taken for the request
	ContextOriginalPathKey = contextKey("original_path") // original path before any modifications, useful for logging/debugging
	ContextKeyStream       = contextKey("stream")        // indicates whether the response should be streamed or buffered
	ContextProviderTypeKey = contextKey("provider_type") // the provider type for the request, used for routing and load balancing

	// SlogRequestIdKey is the log-record role of "request_id": the key the styled
	// loggers and the logging middleware attach request IDs under. slog keys are
	// plain strings, so this stays untyped and is never used for context storage.
	SlogRequestIdKey = "request_id"

	// ContextModelKey carries the resolved model name through the proxy pipeline.
	// Using a typed key prevents accidental collisions with plain-string keys from
	// third-party middleware that might also use "model".
	ContextModelKey = contextKey("model")

	// Sticky session context keys — set by the handler before balancer selection
	// and read back after to surface affinity decisions in response headers.
	ContextStickyKeyKey       = contextKey("sticky-key")        // computed affinity key for this request
	ContextStickyKeySourceKey = contextKey("sticky-key-source") // which source produced the key
	ContextStickyOutcomeKey   = contextKey("sticky-outcome")    // *StickyOutcome written by the wrapper

	// ContextModelAliasMapKey stores a map[string]string of endpoint URL → actual model name
	// when a model alias is resolved, allowing the proxy to rewrite the model name in the
	// request body to match what the selected backend expects. The profile InspectionMeta
	// role of the same spelling is InspectionMetaModelAliasMapKey.
	ContextModelAliasMapKey = contextKey("model_alias_map")

	// InspectionMetaModelAliasMapKey is the profile InspectionMeta role of
	// "model_alias_map": the alias handler stores the endpoint→model rewrite map
	// under this string and the translation handler loads it before installing
	// the map in the context under ContextModelAliasMapKey. InspectionMeta is
	// string-keyed, so this stays untyped and is never used for context storage.
	InspectionMetaModelAliasMapKey = "model_alias_map"

	// ContextEndpointPathMapKey stores a map[string]string of endpoint URL to the request path
	// that endpoint serves, for requests a translator rewrote to a path that not every backend
	// mounts at the same place (DMR serves /engines/v1/chat/completions, not /v1/...). The
	// proxy builds each attempt's URL from it, so failover across mixed backends stays correct.
	ContextEndpointPathMapKey = contextKey("endpoint-path-map")

	// ContextCanonicalModelKey carries the native name a bare Ollama model name resolved to
	// (see core.CanonicalModel), so stats aggregate under one row. It is never used to
	// rewrite the forwarded request: the client's spelling is what the backend receives.
	ContextCanonicalModelKey = contextKey("canonical-model")

	// ContextInputTokensKey carries a prompt token estimate through the translation
	// pipeline so the streaming translator can populate input_tokens in message_start
	// before the upstream usage chunk arrives (which only comes at the end of the stream).
	ContextInputTokensKey = contextKey("input-tokens")
)

// WithRoutePrefix installs the prefix a registered route strips from the
// request path before forwarding. Writers are the route registry, the provider
// handler and tests; the reader is util.StripRoutePrefix.
func WithRoutePrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, ContextRoutePrefixKey, prefix)
}

// WithStickyOutcome installs the sticky-session outcome slot the handler
// allocates and the balancer wrapper fills before Select() returns. The value
// type is the domain type; balancer.StickyOutcome is an alias of it.
func WithStickyOutcome(ctx context.Context, outcome *domain.StickyOutcome) context.Context {
	return context.WithValue(ctx, ContextStickyOutcomeKey, outcome)
}

// GetStickyOutcome reads the sticky-session outcome slot. ok reports the
// slot's presence, not the pointer's non-nilness; callers that care check both.
func GetStickyOutcome(ctx context.Context) (*domain.StickyOutcome, bool) {
	outcome, ok := ctx.Value(ContextStickyOutcomeKey).(*domain.StickyOutcome)
	return outcome, ok
}

// GetStickyKey reads the computed affinity key for this request; an absent
// slot reads as the empty string, which callers already treat as "no key".
func GetStickyKey(ctx context.Context) string {
	key, _ := ctx.Value(ContextStickyKeyKey).(string)
	return key
}

// GetStickyKeySource reads which source produced the affinity key; an absent
// slot reads as the empty string.
func GetStickyKeySource(ctx context.Context) string {
	source, _ := ctx.Value(ContextStickyKeySourceKey).(string)
	return source
}

// WithModelAliasMap installs the endpoint→model rewrite map for the selected
// backend, loaded by the translation handler from the profile's InspectionMeta.
func WithModelAliasMap(ctx context.Context, aliasMap map[string]string) context.Context {
	return context.WithValue(ctx, ContextModelAliasMapKey, aliasMap)
}

// GetModelAliasMap reads the endpoint→model rewrite map.
func GetModelAliasMap(ctx context.Context) (map[string]string, bool) {
	aliasMap, ok := ctx.Value(ContextModelAliasMapKey).(map[string]string)
	return aliasMap, ok
}

// GetModel reads the resolved model name the proxy pipeline carries for
// header setting and stats.
func GetModel(ctx context.Context) (string, bool) {
	model, ok := ctx.Value(ContextModelKey).(string)
	return model, ok
}

// GetEndpointPathMap reads the endpoint URL → served request path map the
// proxy builds each failover attempt's URL from.
func GetEndpointPathMap(ctx context.Context) (map[string]string, bool) {
	paths, ok := ctx.Value(ContextEndpointPathMapKey).(map[string]string)
	return paths, ok
}

// GetInputTokens reads the prompt token estimate the streaming translator
// seeds input_tokens from.
func GetInputTokens(ctx context.Context) (int, bool) {
	estimate, ok := ctx.Value(ContextInputTokensKey).(int)
	return estimate, ok
}
