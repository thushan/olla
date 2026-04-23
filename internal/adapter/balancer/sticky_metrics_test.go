package balancer

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// TestStickySessionWrapper_Stats_TracksMetrics verifies that Stats() reports
// non-zero counters after Select activity. Issue #139: the reporter saw
// /internal/stats/sticky returning all zero counters despite sticky being
// enabled and active. This guards against any future regression where the
// underlying ttlcache stops collecting metrics (e.g. an opt-in option being
// added in a future version).
func TestStickySessionWrapper_Stats_TracksMetrics(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	endpoints := []*domain.Endpoint{ep1}

	// First select: miss → insertion + miss-style get (Get returns nil → not a hit).
	ctx1, _ := injectKey(context.Background(), "stats-key:llama3", "session_header")
	_, err := w.Select(ctx1, endpoints)
	require.NoError(t, err)

	// Second select: hit on the same key → Hits++.
	ctx2, _ := injectKey(context.Background(), "stats-key:llama3", "session_header")
	_, err = w.Select(ctx2, endpoints)
	require.NoError(t, err)

	stats := w.Stats()
	assert.True(t, stats.Enabled, "Stats.Enabled must be true for an active wrapper")
	assert.Equal(t, 1, stats.ActiveSessions, "one unique key should produce one active session")
	assert.GreaterOrEqual(t, stats.Insertions, uint64(1), "insertion counter must be tracked by the underlying ttlcache")
	assert.GreaterOrEqual(t, stats.Hits, uint64(1), "hit counter must be tracked by the underlying ttlcache")
}

// TestComputeStickyKey_PrefixHash_PromptFallback verifies that legacy completions
// requests (which use "prompt" instead of "messages") still produce a sticky key
// via the prefix_hash source. Without the fallback, /v1/completions and
// llamaswap-style passthrough requests bypass sticky entirely and the metrics
// counters never advance.
func TestComputeStickyKey_PrefixHash_PromptFallback(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"llama3","prompt":"why is the sky blue?","max_tokens":50}`)
	req, _ := http.NewRequest(http.MethodPost, "/", nil)

	cfg := config.StickySessionConfig{
		KeySources:      []string{"prefix_hash"},
		PrefixHashBytes: 512,
	}
	key, source := ComputeStickyKey(req, "llama3", cfg, body)

	assert.Equal(t, "prefix_hash", source, "prompt-only body must still resolve via prefix_hash")
	assert.NotEmpty(t, key, "prompt fallback must produce a non-empty key")
}

// TestComputeStickyKey_PrefixHash_PreferMessagesOverPrompt ensures the prompt
// fallback does not accidentally override the canonical messages path when both
// are present.
func TestComputeStickyKey_PrefixHash_PreferMessagesOverPrompt(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"prefix_hash"},
		PrefixHashBytes: 512,
	}

	chatBody := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	mixedBody := []byte(`{"messages":[{"role":"user","content":"hi"}],"prompt":"different content"}`)

	keyChat, _ := ComputeStickyKey(&http.Request{}, "m", cfg, chatBody)
	keyMixed, _ := ComputeStickyKey(&http.Request{}, "m", cfg, mixedBody)

	assert.Equal(t, keyChat, keyMixed, "messages must take precedence over prompt when both exist")
}

// TestComputeStickyKey_SessionHeader_EmptyModel verifies that an unidentified
// model (e.g. llamaswap requests where no inspector populates pr.model) still
// produces a usable, distinct key per session ID. Reproduces a slice of
// issue #139 where requests without model identification appeared to bypass
// sticky entirely.
func TestComputeStickyKey_SessionHeader_EmptyModel(t *testing.T) {
	t.Parallel()

	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	req1, _ := http.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set(constants.HeaderXOllaSessionID, "session-A")

	req2, _ := http.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set(constants.HeaderXOllaSessionID, "session-B")

	keyA, sourceA := ComputeStickyKey(req1, "", cfg, nil)
	keyB, sourceB := ComputeStickyKey(req2, "", cfg, nil)

	assert.Equal(t, "session_header", sourceA)
	assert.Equal(t, "session_header", sourceB)
	assert.NotEmpty(t, keyA, "empty model must still produce a key when session header is present")
	assert.NotEmpty(t, keyB)
	assert.NotEqual(t, keyA, keyB, "distinct session IDs must produce distinct keys even with empty model")
}

// TestStickySessionWrapper_EmptyModel_RoutesAndPins verifies the end-to-end path:
// a session header arrives with no identified model, the wrapper computes a key,
// pins a backend, and a second request with the same session ID hits the same
// backend. This is the scenario reported in issue #139 for llamaswap.
func TestStickySessionWrapper_EmptyModel_RoutesAndPins(t *testing.T) {
	t.Parallel()

	w := makeWrapper(t, defaultStickyConfig())
	ep1 := makeEndpoint("ep1", "http://backend1:8080")
	ep2 := makeEndpoint("ep2", "http://backend2:8080")
	endpoints := []*domain.Endpoint{ep1, ep2}

	cfg := config.StickySessionConfig{
		KeySources:      []string{"session_header"},
		PrefixHashBytes: 512,
	}

	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(constants.HeaderXOllaSessionID, "llamaswap-session")
	key, source := ComputeStickyKey(req, "", cfg, nil)
	require.NotEmpty(t, key, "issue #139: empty model + session header must still yield a key")

	ctx1, out1 := injectKey(context.Background(), key, source)
	first, err := w.Select(ctx1, endpoints)
	require.NoError(t, err)
	assert.Equal(t, "miss", out1.Result)

	ctx2, out2 := injectKey(context.Background(), key, source)
	second, err := w.Select(ctx2, endpoints)
	require.NoError(t, err)
	assert.Equal(t, "hit", out2.Result)
	assert.Equal(t, first.URLString, second.URLString, "same key must pin to same backend across calls")

	// Confirm the metrics counters reflect the activity — the symptom in #139.
	stats := w.Stats()
	assert.GreaterOrEqual(t, stats.Insertions, uint64(1))
	assert.GreaterOrEqual(t, stats.Hits, uint64(1))
}
