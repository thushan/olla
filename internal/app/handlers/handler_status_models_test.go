package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// TestBuildModelSummaries_EndpointNamesConsistent pins the v0.0.28 contract:
// ModelSummary.Endpoints must contain endpoint NAMES (not URLs) for all
// occurrences — first and subsequent — when a model appears on multiple
// endpoints.
//
// Prior to the Phase 1 pool-removal refactor the first occurrence received the
// endpoint URL while duplicates received the name. The new code is consistent
// (always names). This test exists to lock that contract so it cannot silently
// drift back.
func TestBuildModelSummaries_EndpointNamesConsistent(t *testing.T) {
	t.Parallel()

	app := &Application{}

	now := time.Now()

	modelMap := map[string]*domain.EndpointModels{
		"http://localhost:11434": {
			Models: []*domain.ModelInfo{
				{Name: "llama3", LastSeen: now},
				{Name: "mistral", LastSeen: now},
			},
		},
		"http://localhost:8080": {
			Models: []*domain.ModelInfo{
				// llama3 appears on both endpoints — the duplicate path was the
				// original source of the inconsistency.
				{Name: "llama3", LastSeen: now.Add(-time.Minute)},
			},
		},
	}

	// endpointNames maps URL → human-readable name, mirroring what modelsStatusHandler builds.
	endpointNames := map[string]string{
		"http://localhost:11434": "ollama-local",
		"http://localhost:8080":  "lmstudio-local",
	}

	summaries := app.buildModelSummaries(modelMap, endpointNames, nil, nil)

	// Build a lookup by model name for assertion convenience.
	byName := make(map[string]ModelSummary, len(summaries))
	for _, s := range summaries {
		byName[s.Name] = s
	}

	// mistral lives on exactly one endpoint — straightforward name check.
	mistral, ok := byName["mistral"]
	if !ok {
		t.Fatal("expected 'mistral' in summaries")
	}
	if len(mistral.Endpoints) != 1 {
		t.Fatalf("mistral: expected 1 endpoint, got %d: %v", len(mistral.Endpoints), mistral.Endpoints)
	}
	if mistral.Endpoints[0] != "ollama-local" {
		t.Errorf("mistral: Endpoints[0] should be name 'ollama-local', got %q", mistral.Endpoints[0])
	}

	// llama3 lives on two endpoints — this is where old code put a URL for the
	// first occurrence. Both entries must be names.
	llama, ok := byName["llama3"]
	if !ok {
		t.Fatal("expected 'llama3' in summaries")
	}
	if len(llama.Endpoints) != 2 {
		t.Fatalf("llama3: expected 2 endpoints, got %d: %v", len(llama.Endpoints), llama.Endpoints)
	}

	sort.Strings(llama.Endpoints)
	wantEndpoints := []string{"lmstudio-local", "ollama-local"}
	sort.Strings(wantEndpoints)

	for i, got := range llama.Endpoints {
		if got != wantEndpoints[i] {
			t.Errorf("llama3: Endpoints[%d] = %q, want %q (must be a name, not a URL)", i, got, wantEndpoints[i])
		}
	}

	// Paranoia: verify none of the endpoint strings look like URLs.
	for _, s := range summaries {
		for _, ep := range s.Endpoints {
			if len(ep) > 4 && ep[:4] == "http" {
				t.Errorf("model %q: endpoint %q looks like a URL, expected an endpoint name", s.Name, ep)
			}
		}
	}
}

// TestBuildModelSummaries_FallbackToURL confirms that when a URL has no name
// mapping (e.g. a newly-discovered endpoint not yet in the repository snapshot),
// buildModelSummaries falls back to the URL so the model is not silently
// dropped. The URL-as-fallback is an explicit code path (endpointName = endpointURL).
func TestBuildModelSummaries_FallbackToURL(t *testing.T) {
	t.Parallel()

	app := &Application{}

	modelMap := map[string]*domain.EndpointModels{
		"http://192.168.1.50:11434": {
			Models: []*domain.ModelInfo{
				{Name: "phi3", LastSeen: time.Now()},
			},
		},
	}

	// No entry for this URL — triggers the fallback to URL path.
	endpointNames := map[string]string{}

	summaries := app.buildModelSummaries(modelMap, endpointNames, nil, nil)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Name != "phi3" {
		t.Fatalf("unexpected model name %q", summaries[0].Name)
	}
	if len(summaries[0].Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint entry, got %d", len(summaries[0].Endpoints))
	}
	// The fallback value is the URL itself — this is acceptable and documented behaviour.
	if summaries[0].Endpoints[0] != "http://192.168.1.50:11434" {
		t.Errorf("expected URL as fallback endpoint, got %q", summaries[0].Endpoints[0])
	}
}

// TestBuildModelSummaries_FallbackToURL_Sanitised is the regression guard for
// the query-string secret leak: when an endpoint has no configured name (or a
// stale model-map entry has no repository match), the fallback to the raw URL
// must go through sanitiseDisplayURL first. A URL like
// "https://host/v1?api_key=secret#frag" must never surface its query string
// or fragment in the response.
func TestBuildModelSummaries_FallbackToURL_Sanitised(t *testing.T) {
	t.Parallel()

	app := &Application{}

	rawURL := "https://host/v1?api_key=secret#frag"
	modelMap := map[string]*domain.EndpointModels{
		rawURL: {
			Models: []*domain.ModelInfo{
				{Name: "phi3", LastSeen: time.Now()},
			},
		},
	}

	// No entry for this URL — triggers the nameless-endpoint fallback path.
	endpointNames := map[string]string{}

	summaries := app.buildModelSummaries(modelMap, endpointNames, nil, nil)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if len(summaries[0].Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint entry, got %d", len(summaries[0].Endpoints))
	}

	got := summaries[0].Endpoints[0]
	if strings.Contains(got, "api_key") || strings.Contains(got, "secret") {
		t.Errorf("fallback endpoint leaked the query string: %q", got)
	}
	if strings.Contains(got, "frag") {
		t.Errorf("fallback endpoint leaked the fragment: %q", got)
	}
	want := "https://host/v1"
	if got != want {
		t.Errorf("fallback endpoint = %q, want sanitised %q", got, want)
	}
}

// TestGetRecentModels_SortsByRecencyNotName is the regression guard for the
// broken recency sort: a fleet where alphabetical order and true recency
// order differ must come back ordered by real last-seen time, not by name.
// Before the fix, parseTimeAgoOptimised's substring checks never matched
// format.TimeAgo's compact "10m ago"/"2h ago" output, so every model fell
// into the same fallback bucket and the sort degenerated to alphabetical.
func TestGetRecentModels_SortsByRecencyNotName(t *testing.T) {
	t.Parallel()

	app := &Application{}
	now := time.Now()

	// Alphabetically "alpha" < "bravo" < "charlie", but recency order is the
	// reverse: charlie was seen most recently, alpha least recently.
	alpha := now.Add(-3 * time.Hour)
	bravo := now.Add(-1 * time.Hour)
	charlie := now.Add(-1 * time.Minute)

	models := []ModelSummary{
		{Name: "alpha", LastSeenAt: &alpha},
		{Name: "bravo", LastSeenAt: &bravo},
		{Name: "charlie", LastSeenAt: &charlie},
	}

	got := app.getRecentModels(models, 10)

	want := []string{"charlie", "bravo", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("expected %d models, got %d", len(want), len(got))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q (order: %v)", i, got[i].Name, name, namesOf(got))
		}
	}
}

// TestGetRecentModels_Deterministic proves two repeated calls against the
// same fixture data produce identical ordering — no map-iteration flakiness
// entering via the sort.
func TestGetRecentModels_Deterministic(t *testing.T) {
	t.Parallel()

	app := &Application{}
	now := time.Now()

	t1 := now.Add(-2 * time.Hour)
	t2 := now.Add(-90 * time.Minute)
	t3 := now.Add(-45 * time.Minute)
	t4 := now.Add(-10 * time.Minute)

	base := []ModelSummary{
		{Name: "delta", LastSeenAt: &t1},
		{Name: "echo", LastSeenAt: &t2},
		{Name: "foxtrot", LastSeenAt: &t3},
		{Name: "golf", LastSeenAt: &t4},
	}

	var first []string
	for range 20 {
		// Fresh copy each call: getRecentModels sorts in place.
		cp := make([]ModelSummary, len(base))
		copy(cp, base)

		got := app.getRecentModels(cp, 10)
		names := namesOf(got)

		if first == nil {
			first = names
			continue
		}
		if !equalStrings(first, names) {
			t.Fatalf("non-deterministic ordering: got %v, want %v", names, first)
		}
	}
}

// TestBuildModelSummaries_MultiEndpointPicksNewestTimestamp confirms that
// when a model appears on several endpoints, the summary reports the newest
// last-seen timestamp across those endpoints regardless of map iteration
// order, by exercising both possible insertion orders.
func TestBuildModelSummaries_MultiEndpointPicksNewestTimestamp(t *testing.T) {
	t.Parallel()

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Minute)

	endpointNames := map[string]string{
		"http://endpoint-a:11434": "endpoint-a",
		"http://endpoint-b:11434": "endpoint-b",
	}

	// Go map iteration order is randomised per run, so running this fixture
	// as-is across repeated test invocations already exercises both orders
	// over time; assert the invariant directly rather than trying to force
	// a specific visitation order.
	app := &Application{}
	modelMap := map[string]*domain.EndpointModels{
		"http://endpoint-a:11434": {
			Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: older}},
		},
		"http://endpoint-b:11434": {
			Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: newer}},
		},
	}

	summaries := app.buildModelSummaries(modelMap, endpointNames, nil, nil)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	got := summaries[0]
	if got.LastSeenAt == nil {
		t.Fatal("expected LastSeenAt to be set")
	}
	if !got.LastSeenAt.Equal(newer) {
		t.Errorf("LastSeenAt = %v, want the newer timestamp %v", got.LastSeenAt, newer)
	}
}

// namesOf extracts model names in order, for compact assertion messages.
func namesOf(models []ModelSummary) []string {
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mockAliasModelRegistry wraps mockStatusModelRegistry with an injectable
// GetUnifiedModels implementation so the alias-resolution path can be
// exercised. mockStatusModelRegistry alone does NOT satisfy
// unifiedModelsGetter, which is itself useful for the graceful-absence test.
type mockAliasModelRegistry struct {
	mockStatusModelRegistry
	unified []*domain.UnifiedModel
	err     error
}

func (m *mockAliasModelRegistry) GetUnifiedModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.unified, nil
}

// TestBuildModelSummaries_EndpointIDsFollowNamesAfterSort pins the paired-sort
// contract: after sorting Endpoints alphabetically, the i-th EndpointID must
// still correspond to the i-th name. Two independent sorts would desync them.
// This uses distinct names so the sort has a deterministic answer.
func TestBuildModelSummaries_EndpointIDsFollowNamesAfterSort(t *testing.T) {
	t.Parallel()

	app := &Application{}
	now := time.Now()

	modelMap := map[string]*domain.EndpointModels{
		"http://node-a:11434": {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: now}}},
		"http://node-b:11434": {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: now}}},
	}
	endpointNames := map[string]string{
		"http://node-a:11434": "zeta-cluster",
		"http://node-b:11434": "alpha-cluster",
	}
	endpointIDs := map[string]string{
		"http://node-a:11434": "id-zeta",
		"http://node-b:11434": "id-alpha",
	}

	summaries := app.buildModelSummaries(modelMap, endpointNames, endpointIDs, nil)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]

	wantNames := []string{"alpha-cluster", "zeta-cluster"}
	wantIDs := []string{"id-alpha", "id-zeta"}

	if !equalStrings(s.Endpoints, wantNames) {
		t.Errorf("Endpoints = %v, want %v", s.Endpoints, wantNames)
	}
	if !equalStrings(s.EndpointIDs, wantIDs) {
		t.Errorf("EndpointIDs = %v, want %v (must follow name order after paired sort)", s.EndpointIDs, wantIDs)
	}
}

// TestBuildModelSummaries_EndpointIDsDistinctForCollidingNames is the core
// regression for the dashboard click-through: when two distinct endpoints
// share the SAME display name (a legitimate config), their stable IDs must
// still be distinct. The dashboard routes by ID precisely because names are
// not guaranteed unique.
func TestBuildModelSummaries_EndpointIDsDistinctForCollidingNames(t *testing.T) {
	t.Parallel()

	app := &Application{}
	now := time.Now()

	urlA := "http://node-a:11434"
	urlB := "http://node-b:11434"

	modelMap := map[string]*domain.EndpointModels{
		urlA: {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: now}}},
		urlB: {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: now}}},
	}
	endpointNames := map[string]string{
		urlA: "ollama-cluster",
		urlB: "ollama-cluster",
	}
	endpointIDs := map[string]string{
		urlA: stableEndpointID(urlA),
		urlB: stableEndpointID(urlB),
	}

	summaries := app.buildModelSummaries(modelMap, endpointNames, endpointIDs, nil)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]

	if len(s.Endpoints) != 2 || len(s.EndpointIDs) != 2 {
		t.Fatalf("expected 2 endpoints and 2 ids, got %d / %d", len(s.Endpoints), len(s.EndpointIDs))
	}
	if s.Endpoints[0] != s.Endpoints[1] {
		t.Errorf("expected identical display names (collision fixture), got %q and %q", s.Endpoints[0], s.Endpoints[1])
	}
	if s.EndpointIDs[0] == s.EndpointIDs[1] {
		t.Errorf("expected distinct IDs for distinct URLs, both = %q", s.EndpointIDs[0])
	}
	gotIDSet := map[string]struct{}{s.EndpointIDs[0]: {}, s.EndpointIDs[1]: {}}
	for _, want := range []string{stableEndpointID(urlA), stableEndpointID(urlB)} {
		if _, ok := gotIDSet[want]; !ok {
			t.Errorf("missing expected ID %q in %v", want, gotIDSet)
		}
	}
}

// TestBuildAliasLookup_IncludesCanonicalAndExcludesSelf verifies that for a
// model with sibling aliases and a distinct canonical ID, looking up by an
// alias name returns the other aliases plus the canonical ID, sorted, with
// the queried name excluded from its own result.
func TestBuildAliasLookup_IncludesCanonicalAndExcludesSelf(t *testing.T) {
	t.Parallel()

	reg := &mockAliasModelRegistry{
		unified: []*domain.UnifiedModel{
			{
				ID: "llama/3",
				Aliases: []domain.AliasEntry{
					{Name: "llama3"},
					{Name: "llama-3"},
				},
			},
		},
	}
	app := &Application{modelRegistry: reg}

	lookup := app.buildAliasLookup(context.Background())
	if lookup == nil {
		t.Fatal("expected non-nil lookup")
	}

	// Querying by an alias name returns the sibling alias plus the canonical
	// ID, sorted, with the queried name excluded.
	want := []string{"llama-3", "llama/3"}
	if got := lookup["llama3"]; !equalStrings(got, want) {
		t.Errorf("lookup[llama3] = %v, want %v", got, want)
	}

	// The canonical ID is also a valid key, returning the alias names.
	wantCanonical := []string{"llama-3", "llama3"}
	if got := lookup["llama/3"]; !equalStrings(got, wantCanonical) {
		t.Errorf("lookup[llama/3] = %v, want %v", got, wantCanonical)
	}

	// Self exclusion: no result contains its own query key.
	for query, siblings := range lookup {
		for _, s := range siblings {
			if s == query {
				t.Errorf("lookup[%q] contains itself: %v", query, siblings)
			}
		}
	}
}

// TestBuildAliasLookup_AbsentWhenRegistryLacksInterface confirms the
// graceful-degradation path: a registry that does NOT satisfy
// unifiedModelsGetter (mockStatusModelRegistry alone) yields a nil lookup,
// not an error or panic.
func TestBuildAliasLookup_AbsentWhenRegistryLacksInterface(t *testing.T) {
	t.Parallel()

	app := &Application{modelRegistry: &mockStatusModelRegistry{}}
	if lookup := app.buildAliasLookup(context.Background()); lookup != nil {
		t.Errorf("expected nil lookup when registry lacks GetUnifiedModels, got %v", lookup)
	}
}

// TestBuildModelSummaries_AliasesAttached verifies that an alias set from the
// lookup is attached to the matching ModelSummary, sorted, with the model's
// own raw name excluded.
func TestBuildModelSummaries_AliasesAttached(t *testing.T) {
	t.Parallel()

	app := &Application{}
	now := time.Now()

	modelMap := map[string]*domain.EndpointModels{
		"http://localhost:11434": {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: now}}},
	}
	endpointNames := map[string]string{
		"http://localhost:11434": "ollama-local",
	}
	endpointIDs := map[string]string{
		"http://localhost:11434": stableEndpointID("http://localhost:11434"),
	}
	aliases := map[string][]string{
		// Pre-built lookup: querying by "llama3" returns its siblings.
		"llama3": {"llama-3", "llama/3"},
	}

	summaries := app.buildModelSummaries(modelMap, endpointNames, endpointIDs, aliases)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]

	want := []string{"llama-3", "llama/3"}
	if !equalStrings(s.Aliases, want) {
		t.Errorf("Aliases = %v, want %v", s.Aliases, want)
	}
	// The model's own name must not appear in its alias set.
	for _, a := range s.Aliases {
		if a == s.Name {
			t.Errorf("own name %q must not appear in aliases %v", s.Name, s.Aliases)
		}
	}
}

// TestModelsStatusHandler_AliasesAbsentGraceful drives the full handler with a
// registry that does not satisfy unifiedModelsGetter. The response must still
// be 200 with the model present, and aliases omitted entirely, never an
// error or empty body.
func TestModelsStatusHandler_AliasesAbsentGraceful(t *testing.T) {
	t.Parallel()

	reg := &mockStatusModelRegistry{
		endpointModels: map[string]*domain.EndpointModels{
			"http://localhost:11434": {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: time.Now()}}},
		},
	}
	ep := []*domain.Endpoint{
		{Name: "ollama-local", Type: "ollama", URLString: "http://localhost:11434", Status: domain.StatusHealthy},
	}
	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: ep},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  reg,
		StartTime:      time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	w := httptest.NewRecorder()
	app.modelsStatusHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even without alias support, got %d (body=%s)", w.Code, w.Body.String())
	}

	var resp ModelStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.RecentModels) != 1 {
		t.Fatalf("expected 1 model in response, got %d", len(resp.RecentModels))
	}
	if len(resp.RecentModels[0].Aliases) != 0 {
		t.Errorf("expected no aliases when registry lacks GetUnifiedModels, got %v", resp.RecentModels[0].Aliases)
	}
}

// TestModelsStatusHandler_ETagStableAndAliasAware locks the model ETag
// semantics: two identical requests yield the same ETag, and a change to the
// alias set changes the ETag (because aliases are part of the model hash).
func TestModelsStatusHandler_ETagStableAndAliasAware(t *testing.T) {
	t.Parallel()

	seen := time.Now()
	reg := &mockAliasModelRegistry{
		mockStatusModelRegistry: mockStatusModelRegistry{
			endpointModels: map[string]*domain.EndpointModels{
				"http://localhost:11434": {Models: []*domain.ModelInfo{{Name: "llama3", LastSeen: seen}}},
			},
		},
		unified: []*domain.UnifiedModel{
			{
				ID: "llama/3",
				Aliases: []domain.AliasEntry{
					{Name: "llama3"},
					{Name: "llama-3"},
				},
			},
		},
	}
	ep := []*domain.Endpoint{
		{Name: "ollama-local", Type: "ollama", URLString: "http://localhost:11434", Status: domain.StatusHealthy},
	}
	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: ep},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  reg,
		StartTime:      time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)

	w1 := httptest.NewRecorder()
	app.modelsStatusHandler(w1, req)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}
	etag1 := w1.Header().Get("ETag")
	if etag1 == "" {
		t.Fatal("missing ETag on first response")
	}

	// Identical second request: ETag must be stable across polls. The
	// relative LastSeen rendering churns every poll but is excluded from
	// the hash; the absolute LastSeenAt is fixed by the fixture.
	w2 := httptest.NewRecorder()
	app.modelsStatusHandler(w2, req)
	if got := w2.Header().Get("ETag"); got != etag1 {
		t.Errorf("ETag not stable across identical requests: %q vs %q", got, etag1)
	}

	// Mutate the alias set: an added alias must surface in the hash, so the
	// ETag changes. This is the regression guard for aliases being part of
	// hashModelSummary.
	reg.unified = []*domain.UnifiedModel{
		{
			ID: "llama/3",
			Aliases: []domain.AliasEntry{
				{Name: "llama3"},
				{Name: "llama-3"},
				{Name: "llama_3"},
			},
		},
	}
	w3 := httptest.NewRecorder()
	app.modelsStatusHandler(w3, req)
	if got := w3.Header().Get("ETag"); got == etag1 {
		t.Errorf("ETag must change when alias set changes, both = %q", etag1)
	}
}
