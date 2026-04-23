package balancer

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// --- test helpers ---

func defaultStickyConfig() config.StickySessionConfig {
	return config.StickySessionConfig{
		Enabled:         true,
		IdleTTLSeconds:  60,
		MaxSessions:     100,
		KeySources:      []string{"session_header", "prefix_hash", "auth_header", "ip"},
		PrefixHashBytes: 512,
	}
}

func makeEndpoint(name, rawURL string) *domain.Endpoint {
	u, _ := url.Parse(rawURL)
	return &domain.Endpoint{
		Name:      name,
		URL:       u,
		URLString: rawURL,
		Status:    domain.StatusHealthy,
	}
}

func makeWrapper(t *testing.T, cfg config.StickySessionConfig) *StickySessionWrapper {
	t.Helper()
	inner := NewRoundRobinSelector(nil)
	// patch IncrementConnections/DecrementConnections to accept nil statsCollector
	w := NewStickySessionWrapper(inner, cfg)
	w.Start()
	t.Cleanup(w.Stop)
	return w
}

// injectKey builds a context carrying the sticky key and an outcome pointer.
func injectKey(parent context.Context, key, source string) (context.Context, *StickyOutcome) {
	outcome := &StickyOutcome{}
	ctx := context.WithValue(parent, constants.ContextStickyKeyKey, key)
	ctx = context.WithValue(ctx, constants.ContextStickyKeySourceKey, source)
	ctx = context.WithValue(ctx, constants.ContextStickyOutcomeKey, outcome)
	return ctx, outcome
}

// --- RoundRobinSelector with nil stats shim ---
// The existing RoundRobinSelector panics on nil statsCollector only inside
// IncrementConnections/DecrementConnections (which call RecordConnection).
// Select itself works fine, so we can use it directly in unit tests where we
// never call those methods.

// --- tests ---

func TestStickySessionWrapper_Miss(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	ctx, outcome := injectKey(context.Background(), "sess-abc:llama3", "session_header")

	chosen, err := w.Select(ctx, endpoints)
	require.NoError(t, err)
	assert.NotNil(t, chosen)
	assert.Equal(t, "miss", outcome.Result)
	assert.Equal(t, "session_header", outcome.Source)
}

func TestStickySessionWrapper_Hit(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	const stickyKey = "sess-hit:llama3"

	// First call — should be a miss and pin a backend.
	ctx1, _ := injectKey(context.Background(), stickyKey, "session_header")
	first, err := w.Select(ctx1, endpoints)
	require.NoError(t, err)

	// Second call with same key — should return the same backend (hit).
	ctx2, outcome2 := injectKey(context.Background(), stickyKey, "session_header")
	second, err := w.Select(ctx2, endpoints)
	require.NoError(t, err)

	assert.Equal(t, first.URLString, second.URLString, "second request should be pinned to the same backend")
	assert.Equal(t, "hit", outcome2.Result)
}

func TestStickySessionWrapper_Repin(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")

	const stickyKey = "sess-repin:llama3"

	// Pin to ep1.
	ctx1, _ := injectKey(context.Background(), stickyKey, "session_header")
	first, err := w.Select(ctx1, []*domain.Endpoint{ep1, ep2})
	require.NoError(t, err)

	// Remove pinned backend from routable set — simulate it going offline.
	remaining := []*domain.Endpoint{ep1, ep2}
	for i, ep := range remaining {
		if ep.URLString == first.URLString {
			remaining[i] = remaining[len(remaining)-1]
			remaining = remaining[:len(remaining)-1]
			break
		}
	}

	ctx2, outcome2 := injectKey(context.Background(), stickyKey, "session_header")
	second, err := w.Select(ctx2, remaining)
	require.NoError(t, err)

	assert.NotEqual(t, first.URLString, second.URLString, "repin should select a different backend")
	assert.Equal(t, "repin", outcome2.Result)
}

func TestStickySessionWrapper_NoKey(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")

	// Empty key — wrapper should pass through transparently.
	ctx := context.Background()
	outcome := &StickyOutcome{}
	ctx = context.WithValue(ctx, constants.ContextStickyOutcomeKey, outcome)

	chosen, err := w.Select(ctx, []*domain.Endpoint{ep1})
	require.NoError(t, err)
	assert.Equal(t, ep1.URLString, chosen.URLString)
	assert.Equal(t, "disabled", outcome.Result)
	assert.Equal(t, "none", outcome.Source)
}

func TestStickySessionWrapper_TTLExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL test in short mode")
	}
	t.Parallel()

	cfg := defaultStickyConfig()
	cfg.IdleTTLSeconds = 1 // 1 second for a fast test

	w := makeWrapper(t, cfg)
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	const stickyKey = "sess-ttl:llama3"

	ctx1, _ := injectKey(context.Background(), stickyKey, "session_header")
	first, err := w.Select(ctx1, endpoints)
	require.NoError(t, err)

	// Confirm pinned.
	ctx2, outcome2 := injectKey(context.Background(), stickyKey, "session_header")
	second, _ := w.Select(ctx2, endpoints)
	assert.Equal(t, first.URLString, second.URLString)
	assert.Equal(t, "hit", outcome2.Result)

	// ttlcache uses sliding TTL (touch-on-hit) — every Get refreshes the
	// entry. Polling with Select would keep the entry alive forever, so we
	// must wait the full TTL without touching the store. Add a small buffer
	// to allow the background janitor goroutine to run the eviction sweep.
	time.Sleep(time.Duration(cfg.IdleTTLSeconds)*time.Second + 500*time.Millisecond)

	ctx3, outcome3 := injectKey(context.Background(), stickyKey, "session_header")
	_, err = w.Select(ctx3, endpoints)
	require.NoError(t, err)
	// After TTL the entry is gone, so it's a fresh miss not a repin.
	assert.Equal(t, "miss", outcome3.Result)
}

func TestStickySessionWrapper_ModelScoping(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	// Same session but different models — keys must be distinct.
	keyModel1 := "sess-scope:modelA"
	keyModel2 := "sess-scope:modelB"

	ctx1, _ := injectKey(context.Background(), keyModel1, "session_header")
	chosenA, err := w.Select(ctx1, endpoints)
	require.NoError(t, err)

	// Force the second model to a specific backend so we can assert they differ.
	// Simply select a few times — the round-robin inner will distribute.
	var chosenB *domain.Endpoint
	for range 10 {
		ctx2, _ := injectKey(context.Background(), keyModel2, "session_header")
		chosenB, _ = w.Select(ctx2, endpoints)
		if chosenB.URLString != chosenA.URLString {
			break
		}
	}
	// The important assertion: each model key is tracked independently.
	ctx3, out3 := injectKey(context.Background(), keyModel1, "session_header")
	third, err := w.Select(ctx3, endpoints)
	require.NoError(t, err)
	assert.Equal(t, chosenA.URLString, third.URLString, "model-scoped key should return same backend")
	assert.Equal(t, "hit", out3.Result)
}

func TestStickySessionWrapper_KeySources_SessionHeader(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	endpoints := []*domain.Endpoint{ep1}

	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(constants.HeaderXOllaSessionID, "my-session-id")

	key, source := ComputeStickyKey(req, "llama3", defaultStickyConfig(), nil)
	// Fix 1: session_header now hashes the raw value with FNV-64a, producing a
	// 16-hex-char prefix. The raw string "my-session-id" must not appear in the key.
	assert.Equal(t, "bd95cc3ab55faccc:llama3", key)
	assert.NotContains(t, key, "my-session-id")
	assert.Equal(t, "session_header", source)

	// Verify it routes
	ctx, out := injectKey(context.Background(), key, source)
	_, err := w.Select(ctx, endpoints)
	require.NoError(t, err)
	assert.Equal(t, "miss", out.Result)
}

func TestStickySessionWrapper_KeySources_PrefixHash(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"llama3","messages":[{"role":"user","content":"hello"}]}`)
	req, _ := http.NewRequest(http.MethodPost, "/", nil)

	key, source := ComputeStickyKey(req, "llama3", defaultStickyConfig(), body)
	assert.Equal(t, "prefix_hash", source)
	assert.NotEmpty(t, key)
}

func TestStickySessionWrapper_KeySources_AuthHash(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"auth_header"},
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer token-xyz")

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)
	assert.Equal(t, "auth_header", source)
	assert.NotEmpty(t, key)
}

func TestStickySessionWrapper_KeySources_IP(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"ip"},
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "192.168.1.42:12345"

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)
	assert.Equal(t, "ip", source)
	assert.Contains(t, key, "192.168.1.42")
}

func TestStickySessionWrapper_KeySources_NoMatch(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"}, // header not present
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)
	assert.Empty(t, key)
	assert.Empty(t, source)
}

func TestComputeStickyKey_ModelScope(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(constants.HeaderXOllaSessionID, "same-session")

	keyA, _ := ComputeStickyKey(req, "llama3", defaultStickyConfig(), nil)
	keyB, _ := ComputeStickyKey(req, "mistral", defaultStickyConfig(), nil)

	assert.NotEqual(t, keyA, keyB, "same session ID with different models must produce different keys")
}

func TestComputeStickyKey_PrefixHashBytes_Truncation(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":"` + string(make([]byte, 1000)) + `"}]}`)
	req, _ := http.NewRequest(http.MethodPost, "/", nil)

	cfg := config.StickySessionConfig{
		KeySources:      []string{"prefix_hash"},
		PrefixHashBytes: 16, // very small limit
	}
	key1, _ := ComputeStickyKey(req, "llama3", cfg, body)

	cfg2 := config.StickySessionConfig{
		KeySources:      []string{"prefix_hash"},
		PrefixHashBytes: 32,
	}
	key2, _ := ComputeStickyKey(req, "llama3", cfg2, body)

	// Different prefix lengths should produce different hashes.
	assert.NotEqual(t, key1, key2)
}

func TestStickySessionWrapper_Race(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	const goroutines = 50
	const iters = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {

		go func() {
			defer wg.Done()
			for range iters {
				key := fmt.Sprintf("sess-race-%d:llama3", g%5) // share some keys to exercise contention
				ctx, _ := injectKey(context.Background(), key, "session_header")
				_, err := w.Select(ctx, endpoints)
				if err != nil {
					t.Errorf("Select returned error: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

func TestStickySessionWrapper_PurgeDeadEndpoints(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")

	// Pin two different sessions to both backends.
	ctx1, _ := injectKey(context.Background(), "sess-purge1:m", "session_header")
	w.Select(ctx1, []*domain.Endpoint{ep1}) //nolint:errcheck

	ctx2, _ := injectKey(context.Background(), "sess-purge2:m", "session_header")
	w.Select(ctx2, []*domain.Endpoint{ep2}) //nolint:errcheck

	// Purge: only ep1 is alive.
	w.PurgeDeadEndpoints([]*domain.Endpoint{ep1})

	// sess-purge2 (pinned to ep2) should be gone → next select is a miss.
	ctx3, out3 := injectKey(context.Background(), "sess-purge2:m", "session_header")
	_, err := w.Select(ctx3, []*domain.Endpoint{ep1, ep2})
	require.NoError(t, err)
	assert.Equal(t, "miss", out3.Result, "session pinned to purged backend should be a fresh miss")

	// sess-purge1 (pinned to ep1) should still be a hit.
	ctx4, out4 := injectKey(context.Background(), "sess-purge1:m", "session_header")
	_, err = w.Select(ctx4, []*domain.Endpoint{ep1, ep2})
	require.NoError(t, err)
	assert.Equal(t, "hit", out4.Result, "session pinned to surviving backend should still hit")
}

// --- Fix 1: session_header hashing ---

// TestComputeStickyKey_SessionHeader_IsHashed verifies that the raw session ID
// value never appears in the computed key — only its FNV-64a hex digest does.
func TestComputeStickyKey_SessionHeader_IsHashed(t *testing.T) {
	t.Parallel()

	const modelName = "llama3"
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(constants.HeaderXOllaSessionID, "my-session-id")

	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	key, source := ComputeStickyKey(req, modelName, cfg, nil)

	assert.Equal(t, "session_header", source)
	assert.NotContains(t, key, "my-session-id", "raw session ID must not appear in the key")
	// 16 hex chars + ":" + modelName
	assert.Equal(t, 16+1+len(modelName), len(key), "key must be exactly 16 hex chars + colon + model name")
}

// TestComputeStickyKey_SessionHeader_LargeValue_IsHashed confirms that arbitrarily
// long session IDs are bounded to a fixed-length key after hashing.
func TestComputeStickyKey_SessionHeader_LargeValue_IsHashed(t *testing.T) {
	t.Parallel()

	const modelName = "llama3"
	largeValue := string(make([]byte, 10000)) // 10 000 zero bytes → valid UTF-8
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(constants.HeaderXOllaSessionID, largeValue)

	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	key, source := ComputeStickyKey(req, modelName, cfg, nil)

	assert.Equal(t, "session_header", source)
	// Regardless of input length, output is always 16 hex + ":" + model.
	assert.Equal(t, 16+1+len(modelName), len(key), "unbounded input must produce a bounded key")
}

// TestComputeStickyKey_SessionHeader_SameValueSameKey verifies that hashing is
// deterministic — two requests with identical session IDs produce identical keys.
func TestComputeStickyKey_SessionHeader_SameValueSameKey(t *testing.T) {
	t.Parallel()

	const modelName = "llama3"
	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	req1, _ := http.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set(constants.HeaderXOllaSessionID, "deterministic-session")

	req2, _ := http.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set(constants.HeaderXOllaSessionID, "deterministic-session")

	key1, _ := ComputeStickyKey(req1, modelName, cfg, nil)
	key2, _ := ComputeStickyKey(req2, modelName, cfg, nil)

	assert.Equal(t, key1, key2, "same session ID must always produce the same key")
}

// --- Fix 3: ip key source uses net.SplitHostPort ---

// TestComputeStickyKey_IP_IPv6Loopback verifies that IPv6 addresses are handled
// correctly — brackets and port are stripped, leaving only the clean host.
func TestComputeStickyKey_IP_IPv6Loopback(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"ip"},
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "[::1]:54321"

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)

	assert.Equal(t, "ip", source)
	assert.Contains(t, key, "::1", "clean IPv6 host must appear in key")
	assert.NotContains(t, key, "54321", "port must be stripped from key")
	assert.NotContains(t, key, "[", "opening bracket must be stripped by net.SplitHostPort")
	assert.NotContains(t, key, "]", "closing bracket must be stripped by net.SplitHostPort")
}

// TestComputeStickyKey_IP_IPv4 verifies that IPv4 address:port is correctly split
// and only the host is included in the key.
func TestComputeStickyKey_IP_IPv4(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"ip"},
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "192.168.1.42:12345"

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)

	assert.Equal(t, "ip", source)
	assert.Contains(t, key, "192.168.1.42", "IPv4 host must appear in key")
	assert.NotContains(t, key, "12345", "port must be stripped from key")
}

// TestComputeStickyKey_IP_BareAddress verifies the fallback path where RemoteAddr
// has no port (e.g. a custom listener that omits the port). The address must still
// be usable as a key rather than being discarded.
func TestComputeStickyKey_IP_BareAddress(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"ip"},
		PrefixHashBytes: 512,
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "10.0.0.1" // no port

	key, source := ComputeStickyKey(req, "llama3", cfg, nil)

	assert.Equal(t, "ip", source)
	assert.NotEmpty(t, key, "bare address without port must still produce a key")
	assert.Contains(t, key, "10.0.0.1", "bare host must appear in key")
}

// --- Fix 2: zero TTL warning ---

// TestNewStickySessionWrapper_ZeroTTL_NoPanic verifies that constructing a wrapper
// with IdleTTLSeconds == 0 does not panic. The warning is emitted to slog but
// capturing structured log output in tests requires non-trivial plumbing; this
// test focuses on the observable guarantee (no panic, wrapper is usable).
func TestNewStickySessionWrapper_ZeroTTL_NoPanic(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		Enabled:         true,
		IdleTTLSeconds:  0, // triggers the warning branch
		MaxSessions:     10,
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	// Must not panic.
	inner := NewRoundRobinSelector(nil)
	w := NewStickySessionWrapper(inner, cfg)
	w.Start()
	t.Cleanup(w.Stop)

	ep := makeEndpoint("ep1", "http://backend1:8080")
	ctx, out := injectKey(context.Background(), "zero-ttl-key:llama3", "session_header")
	_, err := w.Select(ctx, []*domain.Endpoint{ep})
	require.NoError(t, err)
	assert.Equal(t, "miss", out.Result, "wrapper with zero TTL must still route requests")
}
