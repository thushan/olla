package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
)

// Sanitisation half: the display URL surfaced in JSON must strip
// userinfo and the query/fragment wholesale. The config-validation half
// (rejecting userinfo at load time) lives in internal/adapter/discovery.
func TestSanitiseDisplayURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "userinfo and query stripped",
			in:   "https://alice:s3cr3t@ollama.example.local:11434/v1?token=abc&flag=1",
			want: "https://ollama.example.local:11434/v1",
		},
		{
			name: "fragment stripped",
			in:   "http://localhost:11434/api/tags#section",
			want: "http://localhost:11434/api/tags",
		},
		{
			name: "plain url unchanged",
			in:   "http://localhost:11434",
			want: "http://localhost:11434",
		},
		{
			name: "empty string stays empty",
			in:   "",
			want: "",
		},
		{
			name: "user with no password still stripped",
			in:   "https://token@gpu-host:443/v1",
			want: "https://gpu-host:443/v1",
		},
		{
			name: "query only, no userinfo",
			in:   "http://localhost:8080?x=1&y=2",
			want: "http://localhost:8080",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitiseDisplayURL(tc.in))
		})
	}
}

// Guard against a credential leaking into a sanitised URL even when the input
// is adversarial: the sanitised form must never contain the password.
func TestSanitiseDisplayURL_NeverLeaksCredentials(t *testing.T) {
	const password = "super-secret-token-9f8e7d"
	got := sanitiseDisplayURL("https://user:" + password + "@host:443/v1?k=v")
	assert.NotContains(t, got, password)
	assert.NotContains(t, got, "user")
	assert.NotContains(t, got, "k=v")
}

// stableEndpointID is the no-sibling BASE hash: it derives the ID from the
// SANITISED URL only, so two endpoints that differ purely by query string
// (e.g. one carrying ?api_key=A and another ?api_key=B) hash IDENTICALLY.
// Disambiguating such siblings is the job of buildEndpointIDs, not the base
// hash - the base path cannot know its siblings. This locks the new contract
// in: a query-only difference MUST NOT influence the base ID, so neither
// secrets nor rotation can move it.
func TestStableEndpointID_EqualAfterQueryRotation(t *testing.T) {
	a := "http://host:11434/api?v=a"
	b := "http://host:11434/api?v=b"

	require.Equal(t, sanitiseDisplayURL(a), sanitiseDisplayURL(b), "test setup: display URLs should collide after sanitisation")
	assert.Equal(t, stableEndpointID(a), stableEndpointID(b), "base ID must not change when only the query differs")
}

// Security regression: the raw URL's query string must NEVER influence the
// base ID, otherwise it becomes an offline verification oracle for a
// secret. The previous implementation hashed the raw URL including the
// query, so an attacker who could observe an ID could confirm a guessed
// api_key by re-deriving it locally. The new derivation hashes only the
// sanitised form, so a query-embedded secret has zero effect on the value.
func TestStableEndpointID_QuerySecretDoesNotInfluenceHash(t *testing.T) {
	const secret = "hunter2-secret-token"
	withSecret := "http://host:11434/v1?api_key=" + secret
	withoutSecret := "http://host:11434/v1"

	idWith := stableEndpointID(withSecret)
	idWithout := stableEndpointID(withoutSecret)

	assert.Equal(t, idWithout, idWith, "a secret-bearing query must not influence the base ID")
	// Belt and braces: the secret itself must not appear in the ID. The hash
	// is base36 of a 32-bit FNV-1a digest, so this is structurally impossible,
	// but the assertion documents the guarantee and would catch a future
	// regression that bypassed the hash (e.g. concatenating the raw URL).
	assert.NotContains(t, idWith, secret)
}

func TestStableEndpointID_DeterministicAcrossCalls(t *testing.T) {
	const raw = "http://localhost:11434/v1?token=abc"
	first := stableEndpointID(raw)
	for range 5 {
		assert.Equal(t, first, stableEndpointID(raw), "stableEndpointID must return the same value across repeated calls")
	}
}

// C1 regression: a parse error (here caused by a space in the password) must
// fail closed. The previous implementation returned the raw string on parse
// error, leaking credentials into the response JSON when url.Parse rejected
// the input. Config-load validation only catches URLs that parse, so an
// unparseable credentialed URL reaches this path. A non-empty sentinel must
// be returned so the operator sees a diagnosable placeholder, never the raw
// input.
func TestSanitiseDisplayURL_FailClosedOnParseError(t *testing.T) {
	const raw = "http://user:p a s s@host/v1"
	got := sanitiseDisplayURL(raw)

	assert.NotEqual(t, raw, got, "raw credentialed URL must not be returned verbatim on parse error")
	assert.NotContains(t, got, "user", "userinfo must not leak through parse-error path")
	assert.NotContains(t, got, "p a s s", "password must not leak through parse-error path")
	assert.NotEmpty(t, got, "sentinel must be non-empty so the operator sees a diagnosable placeholder")
}

// buildEndpointIDs is the precompute step that gives every endpoint in the
// set a stable, opaque ID derived from the SANITISED URL. The tests below
// pin its three core guarantees:
//   - collisions on sanitised form still produce DISTINCT IDs (positional
//     suffix);
//   - rotating a query-embedded secret does NOT change the ID (suffixes are
//     assigned by a secret-independent sort);
//   - the same endpoint set produces the same IDs regardless of input slice
//     order, so /internal/status, /internal/status/endpoints and
//     /internal/status/models emit identical IDs for the same endpoint.

// TestBuildEndpointIDs_DistinctForSanitisedCollisions: two endpoints that
// share a sanitised URL (differ only by query) MUST get distinct IDs.
// Previously the raw URL was hashed and they were naturally distinct but at
// the cost of leaking the secret into a public value. With sanitised
// hashing, distinctness comes from the positional disambiguator.
func TestBuildEndpointIDs_DistinctForSanitisedCollisions(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		{Name: "node-a", URLString: "http://host:11434/v1?api_key=A"},
		{Name: "node-b", URLString: "http://host:11434/v1?api_key=B"},
	}

	ids := buildEndpointIDs(endpoints)

	require.Len(t, ids, 2, "every endpoint must get an ID")
	idA := ids["http://host:11434/v1?api_key=A"]
	idB := ids["http://host:11434/v1?api_key=B"]
	assert.NotEqual(t, idA, idB, "siblings sharing a sanitised URL must get distinct IDs via the positional disambiguator")
	assert.NotEmpty(t, idA)
	assert.NotEmpty(t, idB)
}

// TestBuildEndpointIDs_SecretRotationKeepsIDStable: rotating api_key=A ->
// api_key=B must NOT change either endpoint's ID, because the disambiguator
// is derived from the secret-independent Name sort, not the query string.
// This test would FAIL against the old derivation: hashing raw URLs made the
// ID a function of the secret, so rotation silently broke every dashboard
// deep-link and row key.
func TestBuildEndpointIDs_SecretRotationKeepsIDStable(t *testing.T) {
	t.Parallel()

	before := []*domain.Endpoint{
		{Name: "node-a", URLString: "http://host:11434/v1?api_key=alpha"},
		{Name: "node-b", URLString: "http://host:11434/v1?api_key=beta"},
	}
	after := []*domain.Endpoint{
		{Name: "node-a", URLString: "http://host:11434/v1?api_key=rotated-xyz"},
		{Name: "node-b", URLString: "http://host:11434/v1?api_key=rotated-abc"},
	}

	// Sanity: the display URLs really do collide before and after, and the
	// secrets really did change - otherwise the test proves nothing.
	require.Equal(t, sanitiseDisplayURL(before[0].URLString), sanitiseDisplayURL(after[0].URLString))
	require.Equal(t, sanitiseDisplayURL(before[1].URLString), sanitiseDisplayURL(after[1].URLString))

	idsBefore := buildEndpointIDs(before)
	idsAfter := buildEndpointIDs(after)

	// Same Name -> same positional suffix -> same ID, despite the raw URL
	// (including the secret) being completely different.
	assert.Equal(t, idsBefore[before[0].URLString], idsAfter[after[0].URLString], "node-a ID must not change when its api_key rotates")
	assert.Equal(t, idsBefore[before[1].URLString], idsAfter[after[1].URLString], "node-b ID must not change when its api_key rotates")
}

// TestBuildEndpointIDs_StableAcrossSliceOrder: the same endpoint set must
// produce the same URL->ID mapping regardless of the order the repository
// yields them in (map iteration order is randomised per call). Without this,
// suffix assignment would churn between polls and break the dashboard's row
// keying. Feeds both orderings and asserts identical mappings.
func TestBuildEndpointIDs_StableAcrossSliceOrder(t *testing.T) {
	t.Parallel()

	a := &domain.Endpoint{Name: "node-a", URLString: "http://host:11434/v1?api_key=A"}
	b := &domain.Endpoint{Name: "node-b", URLString: "http://host:11434/v1?api_key=B"}

	first := buildEndpointIDs([]*domain.Endpoint{a, b})
	second := buildEndpointIDs([]*domain.Endpoint{b, a})

	assert.Equal(t, first, second, "ID assignment must be a pure function of the endpoint SET, not slice order")
}

// TestBuildEndpointIDs_SingletonHasNoSuffix: when a sanitised form is unique
// in the set, the ID is the bare base hash with no positional suffix. This
// keeps the common-case ID short and matches what stableEndpointID (the
// no-sibling fallback) produces, so a singleton endpoint has the same ID
// via either path.
func TestBuildEndpointIDs_SingletonHasNoSuffix(t *testing.T) {
	t.Parallel()

	const url = "http://host:11434/v1"
	endpoints := []*domain.Endpoint{{Name: "solo", URLString: url}}

	ids := buildEndpointIDs(endpoints)

	require.Contains(t, ids, url)
	assert.Equal(t, stableEndpointID(url), ids[url], "singleton ID must equal the bare base hash with no suffix")
	assert.NotContains(t, ids[url], "-", "singleton ID must not carry a disambiguator suffix")
}

// TestBuildEndpointIDs_SortsCollisionGroupByName: the positional suffix is
// assigned by NAME order, not raw-URL order, so a credential rotation cannot
// reshuffle suffixes. With names "bravo" and "alpha" but URLs that sort the
// opposite way, bravo must still get the higher suffix.
func TestBuildEndpointIDs_SortsCollisionGroupByName(t *testing.T) {
	t.Parallel()

	// Names sort alpha < bravo; URLs sort ?k=bravo < ?k=alpha (b<a), i.e. the
	// opposite order. If suffixes were assigned by URL order, alpha would get
	// suffix 1; by Name order, alpha must get suffix 0.
	endpoints := []*domain.Endpoint{
		{Name: "bravo", URLString: "http://host/v1?k=bravo"},
		{Name: "alpha", URLString: "http://host/v1?k=alpha"},
	}

	ids := buildEndpointIDs(endpoints)

	alphaID := ids["http://host/v1?k=alpha"]
	bravoID := ids["http://host/v1?k=bravo"]

	// Both share the same base; alpha gets "-0", bravo gets "-1".
	base := stableEndpointID("http://host/v1")
	assert.Equal(t, base+"-0", alphaID, "alpha (Name sort winner) must get suffix 0")
	assert.Equal(t, base+"-1", bravoID, "bravo must get suffix 1")
	assert.Less(t, alphaID, bravoID, "alpha ID sorts before bravo ID")
}

// TestBuildEndpointIDs_NilEndpointsSkipped guards the parallel-array
// invariant: a nil entry in the input slice must not panic and must not get
// an ID. Production code does not yield nil endpoints today, but the
// house rule is production code must not panic.
func TestBuildEndpointIDs_NilEndpointsSkipped(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		nil,
		{Name: "real", URLString: "http://host:11434"},
		nil,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildEndpointIDs panicked on nil entry: %v", r)
		}
	}()

	ids := buildEndpointIDs(endpoints)
	assert.Len(t, ids, 1, "nil endpoints must be skipped, real endpoint must get an ID")
	assert.Contains(t, ids, "http://host:11434")
}

// TestBuildEndpointIDs_DisambiguatorEncodesNoSecret: belt-and-braces guard
// against any future regression where the secret-bearing raw URL might
// re-enter the disambiguator. The disambiguator is the positional suffix
// only, so neither the original secret nor the rotated secret may appear in
// the resulting ID.
func TestBuildEndpointIDs_DisambiguatorEncodesNoSecret(t *testing.T) {
	t.Parallel()

	const secretA = "api-key-value-alpha"
	const secretB = "api-key-value-beta"
	endpoints := []*domain.Endpoint{
		{Name: "node-a", URLString: "http://host:11434/v1?api_key=" + secretA},
		{Name: "node-b", URLString: "http://host:11434/v1?api_key=" + secretB},
	}

	ids := buildEndpointIDs(endpoints)
	for raw, id := range ids {
		assert.NotContains(t, id, secretA, "id for %q leaked secret A", raw)
		assert.NotContains(t, id, secretB, "id for %q leaked secret B", raw)
	}
}
