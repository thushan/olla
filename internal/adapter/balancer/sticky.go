package balancer

import (
	"context"
	"hash/fnv"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/tidwall/gjson"
)

// StickyOutcome is the domain type; alias it here for callers that import
// this package directly (e.g. handler layer). The canonical definition lives in
// core/domain so that adapter/proxy/core can read it without a cycle.
type StickyOutcome = domain.StickyOutcome

// StickySessionWrapper is a decorator around any EndpointSelector that adds
// KV-cache affinity routing. It remembers which backend last handled a
// conversation (identified by a computed key) and steers subsequent turns
// to the same backend while it remains routable.
//
// TOCTOU on the session store is intentionally acceptable — both racing
// goroutines will select valid backends and the last writer wins, which
// converges quickly in practice.
type StickySessionWrapper struct {
	inner domain.EndpointSelector
	store *ttlcache.Cache[string, string]
	cfg   config.StickySessionConfig
}

// NewStickySessionWrapper wraps inner with sticky session affinity using cfg.
// Call Start() after construction and Stop() on shutdown.
func NewStickySessionWrapper(inner domain.EndpointSelector, cfg config.StickySessionConfig) *StickySessionWrapper {
	idleTTL := time.Duration(cfg.IdleTTLSeconds) * time.Second

	if cfg.IdleTTLSeconds <= 0 {
		// ttlcache treats a zero TTL as no expiration — sessions accumulate until
		// capacity pressure forces eviction. Warn so operators notice the config.
		slog.Warn("sticky sessions TTL is zero — sessions will never expire by TTL")
	}

	store := ttlcache.New[string, string](
		ttlcache.WithTTL[string, string](idleTTL),
		ttlcache.WithCapacity[string, string](cfg.MaxSessions),
	)

	return &StickySessionWrapper{
		inner: inner,
		store: store,
		cfg:   cfg,
	}
}

// Start launches the ttlcache background goroutine that handles TTL expiry and
// capacity-based eviction. Must be called before the wrapper is used for routing.
func (s *StickySessionWrapper) Start() {
	go s.store.Start()
}

// Stop shuts down the ttlcache background goroutine. Safe to call multiple times.
func (s *StickySessionWrapper) Stop() {
	s.store.Stop()
}

// Name returns a descriptive name that composes the inner balancer name.
func (s *StickySessionWrapper) Name() string {
	return "sticky(" + s.inner.Name() + ")"
}

// IncrementConnections delegates to the inner selector.
func (s *StickySessionWrapper) IncrementConnections(endpoint *domain.Endpoint) {
	s.inner.IncrementConnections(endpoint)
}

// DecrementConnections delegates to the inner selector.
func (s *StickySessionWrapper) DecrementConnections(endpoint *domain.Endpoint) {
	s.inner.DecrementConnections(endpoint)
}

// Select routes the request to the pinned backend when the affinity key is present
// and the backend is still routable. On a miss or dead-backend, it delegates to
// the inner selector and pins the newly chosen backend for next time.
func (s *StickySessionWrapper) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	outcome, _ := ctx.Value(constants.ContextStickyOutcomeKey).(*StickyOutcome)

	key, _ := ctx.Value(constants.ContextStickyKeyKey).(string)
	source, _ := ctx.Value(constants.ContextStickyKeySourceKey).(string)

	if key == "" {
		// No affinity key — pass through transparently.
		if outcome != nil {
			outcome.Result = "disabled"
			outcome.Source = "none"
		}
		return s.inner.Select(ctx, endpoints)
	}

	// ttlcache.Get refreshes the sliding TTL automatically.
	item := s.store.Get(key)
	if item != nil {
		pinnedURL := item.Value()
		for _, ep := range endpoints {
			if ep.Status.IsRoutable() && ep.URLString == pinnedURL {
				// Sticky hit — backend is still alive and serving this model.
				if outcome != nil {
					outcome.Result = "hit"
					outcome.Source = source
				}
				return ep, nil
			}
		}
		// Pinned backend is gone or unhealthy — fall through to repin.
	}

	chosen, err := s.inner.Select(ctx, endpoints)
	if err != nil {
		return nil, err
	}

	// Record the affinity mapping for future turns.
	s.store.Set(key, chosen.URLString, ttlcache.DefaultTTL)

	result := "miss"
	if item != nil {
		// We had a pin but the backend was no longer routable.
		result = "repin"
	}

	if outcome != nil {
		outcome.Result = result
		outcome.Source = source
	}

	return chosen, nil
}

// PurgeDeadEndpoints removes session entries that point to backends no longer
// present in the provided routable set. Callers can invoke this periodically
// (e.g. on health-check updates) to reclaim store capacity proactively.
func (s *StickySessionWrapper) PurgeDeadEndpoints(routable []*domain.Endpoint) {
	alive := make(map[string]struct{}, len(routable))
	for _, ep := range routable {
		alive[ep.URLString] = struct{}{}
	}

	s.store.Range(func(item *ttlcache.Item[string, string]) bool {
		if _, ok := alive[item.Value()]; !ok {
			s.store.Delete(item.Key())
		}
		return true
	})
}

// ComputeStickyKey derives an affinity key for this request using the configured
// key_sources cascade. The key is model-scoped so the same client talking to
// different models does not cross-contaminate their session state.
//
// Returns ("", "") when no source produces a usable key.
// Exported so handlers can compute the key before invoking the balancer.
func ComputeStickyKey(r *http.Request, modelName string, cfg config.StickySessionConfig, body []byte) (key, source string) {
	for _, src := range cfg.KeySources {
		var k, s string
		switch src {
		case "session_header":
			k, s = stickyKeyFromSessionHeader(r, modelName)
		case "prefix_hash":
			k, s = stickyKeyFromPrefixHash(body, modelName, cfg.PrefixHashBytes)
		case "auth_header":
			k, s = stickyKeyFromAuthHeader(r, modelName)
		case "ip":
			k, s = stickyKeyFromIP(r, modelName)
		}
		if k != "" {
			return k, s
		}
	}

	return "", ""
}

// stickyKeyFromSessionHeader hashes the session ID header with FNV-64a so that
// unbounded client-supplied strings do not inflate cache key memory.
func stickyKeyFromSessionHeader(r *http.Request, modelName string) (string, string) {
	v := r.Header.Get(constants.HeaderXOllaSessionID)
	if v == "" {
		return "", ""
	}
	h := fnv.New64a()
	h.Write([]byte(v))
	return uint64ToHex(h.Sum64()) + ":" + modelName, "session_header"
}

// stickyKeyFromPrefixHash hashes the first prefixBytes bytes of the messages
// JSON array so requests with identical conversation prefixes are routed together.
func stickyKeyFromPrefixHash(body []byte, modelName string, prefixBytes int) (string, string) {
	if len(body) == 0 {
		return "", ""
	}
	raw := gjson.GetBytes(body, "messages").Raw
	if raw == "" {
		return "", ""
	}
	limit := prefixBytes
	if limit <= 0 || limit > len(raw) {
		limit = len(raw)
	}
	h := fnv.New64a()
	h.Write([]byte(raw[:limit]))
	return strings.ReplaceAll(modelName, ":", "_") + ":" + uint64ToHex(h.Sum64()), "prefix_hash"
}

// stickyKeyFromAuthHeader hashes the Authorization header value so that tokens
// are never stored in plaintext inside the session store.
func stickyKeyFromAuthHeader(r *http.Request, modelName string) (string, string) {
	v := r.Header.Get("Authorization")
	if v == "" {
		return "", ""
	}
	h := fnv.New64a()
	h.Write([]byte(v))
	return "auth:" + uint64ToHex(h.Sum64()) + ":" + modelName, "auth_header"
}

// stickyKeyFromIP extracts the remote host using net.SplitHostPort, which
// handles bracketed IPv6 addresses correctly (strings.LastIndex cannot).
func stickyKeyFromIP(r *http.Request, modelName string) (string, string) {
	addr := r.RemoteAddr
	if addr == "" {
		return "", ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Bare address with no port — use as-is.
		host = addr
	}
	if host == "" {
		return "", ""
	}
	return "ip:" + host + ":" + modelName, "ip"
}

// StickyStats holds a point-in-time snapshot of sticky session activity.
type StickyStats struct {
	Enabled        bool   `json:"enabled"`
	ActiveSessions int    `json:"active_sessions"`
	Insertions     uint64 `json:"insertions"`
	Hits           uint64 `json:"hits"`
	Misses         uint64 `json:"misses"`
	Evictions      uint64 `json:"evictions"`
	MaxSessions    uint64 `json:"max_sessions"`
	IdleTTLSeconds int    `json:"idle_ttl_seconds"`
}

// Stats returns a point-in-time snapshot of the session store metrics.
func (s *StickySessionWrapper) Stats() StickyStats {
	m := s.store.Metrics()
	return StickyStats{
		Enabled:        true,
		ActiveSessions: s.store.Len(),
		Insertions:     m.Insertions,
		Hits:           m.Hits,
		Misses:         m.Misses,
		Evictions:      m.Evictions,
		MaxSessions:    s.cfg.MaxSessions,
		IdleTTLSeconds: s.cfg.IdleTTLSeconds,
	}
}

// uint64ToHex converts a uint64 to a hex string without importing fmt (avoids allocation).
func uint64ToHex(v uint64) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf)
}
