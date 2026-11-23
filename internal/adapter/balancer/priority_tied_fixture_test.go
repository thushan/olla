package balancer

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

// tiedPriorityFixture is the FR-9 (b) fixture for the priority balancer sort.
//
// It returns 26 endpoints in a fixed insertion order: 24 routable across three
// priority tiers (10, 8 and 6 endpoints) with ties inside every tier and the
// health statuses mixed within each tier, plus two non-routable endpoints at
// the highest priority so the routable filter is exercised before the sort.
// The routable tiers are interleaved in insertion order rather than grouped,
// because a slice that is already in priority order is detected as sorted and
// left untouched, which would hide the tie hazard. There are more than 12
// routable elements in total, which is where sort.Slice leaves insertion order
// behind (up to 12 elements it falls back to insertion sort, which happens to
// be stable, so a smaller fixture proves nothing about ties).
//
// Names encode tier and status so a printed order reads as
// "p100-healthy-1, p100-busy-2, ...". Labels and order are load-bearing: a
// review of a replacement sort compares the orders two implementations
// produce on exactly this slice.
func tiedPriorityFixture() []*domain.Endpoint {
	statuses := []domain.EndpointStatus{domain.StatusHealthy, domain.StatusBusy, domain.StatusHealthy, domain.StatusWarming}

	// Non-routable at the top: filtered out before the sort, never selected.
	endpoints := []*domain.Endpoint{
		createBenchEndpoint("p200-offline", 11501, domain.StatusOffline, 200),
		createBenchEndpoint("p200-unhealthy", 11502, domain.StatusUnhealthy, 200),
	}

	tiers := []struct {
		priority int
		count    int
	}{
		{100, 10},
		{50, 8},
		{10, 6},
	}
	port := 11434
	// Round-robin across tiers: p100-1, p50-1, p10-1, p100-2, p50-2, p10-2, ...
	for i := 1; i <= tiers[0].count; i++ {
		for _, tier := range tiers {
			if i > tier.count {
				continue
			}
			status := statuses[(i-1)%len(statuses)]
			endpoints = append(endpoints, createBenchEndpoint(
				fmt.Sprintf("p%d-%s-%d", tier.priority, status, i), port, status, tier.priority,
			))
			port++
		}
	}
	return endpoints
}

// fixtureOrder renders a slice as comma-separated endpoint names.
func fixtureOrder(endpoints []*domain.Endpoint) string {
	names := make([]string, len(endpoints))
	for i, ep := range endpoints {
		names[i] = ep.Name
	}
	return strings.Join(names, ", ")
}

// routableOf applies the same filter Select applies, preserving input order.
func routableOf(endpoints []*domain.Endpoint) []*domain.Endpoint {
	routable := make([]*domain.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Status.IsRoutable() {
			routable = append(routable, ep)
		}
	}
	return routable
}

// byPriorityDesc is the comparator in PrioritySelector.Select, copied verbatim.
func byPriorityDesc(a, b *domain.Endpoint) int {
	switch {
	case a.Priority > b.Priority:
		return -1
	case a.Priority < b.Priority:
		return 1
	}
	return 0
}

// TestPrioritySelector_TiedPriorityFixture_UnstableSortReorders shows an
// unstable descending-priority sort does not preserve insertion order among
// equal-priority endpoints on the FR-9 (b) fixture, and prints the order each
// implementation produces so the choice of sort can be compared against the
// order sort.Slice gave (the production sort before WP-3d; slices.SortFunc
// replaced it because it produced the identical order on this fixture).
//
// Select itself never reveals the order (it weight-selects one endpoint from
// the top tier), so the test applies each sort to a copy of the routable
// fixture with the production comparator. If the unstable assertion fails, the
// fixture has stopped exercising the hazard (a sort.Slice change in the
// standard library, or an edit to tiedPriorityFixture) and must be re-cut
// before FR-9 (b) is evaluated.
func TestPrioritySelector_TiedPriorityFixture_UnstableSortReorders(t *testing.T) {
	fixture := tiedPriorityFixture()
	routable := routableOf(fixture)

	if len(routable) <= 12 {
		t.Fatalf("fixture has %d routable endpoints, needs more than 12 to exercise the unstable path", len(routable))
	}
	if n := countPriority(routable, routable[0].Priority); n < 2 {
		t.Fatalf("fixture has no tied top tier")
	}

	topPriority := routable[0].Priority
	insertionTop := tierOf(routable, topPriority)
	t.Logf("insertion top tier:     %s", fixtureOrder(insertionTop))

	// The reference order: sort.Slice with the descending-priority comparator,
	// which Select used before WP-3d.
	current := slices.Clone(routable)
	sort.Slice(current, func(i, j int) bool {
		return current[i].Priority > current[j].Priority
	})
	t.Logf("sort.Slice order:       %s", fixtureOrder(current))

	// A stable replacement: ties keep insertion order by contract.
	stable := slices.Clone(routable)
	slices.SortStableFunc(stable, byPriorityDesc)
	t.Logf("SortStableFunc order:   %s", fixtureOrder(stable))

	// The production sort since WP-3d: slices.SortFunc with the same comparator.
	unstable := slices.Clone(routable)
	slices.SortFunc(unstable, byPriorityDesc)
	t.Logf("SortFunc order:         %s", fixtureOrder(unstable))

	for _, tc := range []struct {
		name  string
		order []*domain.Endpoint
	}{
		{"sort.Slice", current},
		{"slices.SortStableFunc", stable},
		{"slices.SortFunc", unstable},
	} {
		assertPriorityDescending(t, tc.name, tc.order)
		assertSameEndpoints(t, tc.name, routable, tc.order)
	}

	// A stable sort keeps each tier in insertion order; the tiers themselves
	// regroup because the fixture interleaves them.
	if got, want := fixtureOrder(tierOf(stable, topPriority)), fixtureOrder(insertionTop); got != want {
		t.Errorf("slices.SortStableFunc changed the top tier's tie order:\n got  %s\n want %s", got, want)
	}
	if top := current[0].Priority; top != topPriority {
		t.Errorf("sort.Slice top priority %d, want %d", top, topPriority)
	}

	// The claim FR-9 (b) rests on: the unstable sort reorders equal-priority
	// endpoints against insertion order, within the tier that gets selected.
	if got, want := fixtureOrder(tierOf(current, topPriority)), fixtureOrder(insertionTop); got == want {
		t.Errorf("sort.Slice preserved the top tier's insertion order on the tied fixture; the fixture no longer demonstrates the FR-9 (b) hazard")
	}

	// sort.Slice is deterministic for a fixed input, which is why the order
	// difference is a hazard rather than noise: a stable replacement changes
	// the order every request sees, consistently.
	again := slices.Clone(routable)
	sort.Slice(again, func(i, j int) bool {
		return again[i].Priority > again[j].Priority
	})
	if fixtureOrder(again) != fixtureOrder(current) {
		t.Errorf("sort.Slice is not deterministic on the fixture")
	}
}

// TestPrioritySelector_TiedPriorityFixture_SelectsTopTier pins that Select on
// the fixture returns an endpoint from the tied top tier only, so a sort
// change that promoted a lower tier would be caught independently of order.
func TestPrioritySelector_TiedPriorityFixture_SelectsTopTier(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	fixture := tiedPriorityFixture()
	ctx := t.Context()

	for range 1000 {
		ep, err := selector.Select(ctx, fixture)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if ep.Priority != 100 {
			t.Fatalf("selected %s (priority %d), want a priority-100 endpoint", ep.Name, ep.Priority)
		}
	}
}

// BenchmarkPrioritySelector_SelectTiedPriority times Select on the FR-9 (b)
// fixture: the per-request sort with ties, mixed tiers and more than 12
// routable endpoints, followed by weighted selection across the top tier.
// BenchmarkLargeEndpointSet gives every endpoint a distinct priority and
// never reaches the tied path.
func BenchmarkPrioritySelector_SelectTiedPriority(b *testing.B) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	fixture := tiedPriorityFixture()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := selector.Select(ctx, fixture); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrioritySelector_TiedPrioritySort isolates the comparator and swap
// cost on the tied fixture, so the sort can be read without the weighted
// selection on top. It times a test-local sort.Slice on a copy of the fixture,
// not the production sort in priority.go, so it will not respond to a change
// there; BenchmarkPrioritySelector_SelectTiedPriority is the row that does.
func BenchmarkPrioritySelector_TiedPrioritySort(b *testing.B) {
	routable := routableOf(tiedPriorityFixture())
	work := make([]*domain.Endpoint, len(routable))

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		copy(work, routable)
		sort.Slice(work, func(i, j int) bool {
			return work[i].Priority > work[j].Priority
		})
	}
}

func countPriority(endpoints []*domain.Endpoint, priority int) int {
	n := 0
	for _, ep := range endpoints {
		if ep.Priority == priority {
			n++
		}
	}
	return n
}

// tierOf returns the endpoints at one priority, in the order they appear.
func tierOf(endpoints []*domain.Endpoint, priority int) []*domain.Endpoint {
	var tier []*domain.Endpoint
	for _, ep := range endpoints {
		if ep.Priority == priority {
			tier = append(tier, ep)
		}
	}
	return tier
}

func assertPriorityDescending(t *testing.T, name string, order []*domain.Endpoint) {
	t.Helper()
	for i := 1; i < len(order); i++ {
		if order[i-1].Priority < order[i].Priority {
			t.Errorf("%s: priority rises at %d: %s (%d) before %s (%d)",
				name, i, order[i-1].Name, order[i-1].Priority, order[i].Name, order[i].Priority)
		}
	}
}

func assertSameEndpoints(t *testing.T, name string, want, got []*domain.Endpoint) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s: %d endpoints, want %d", name, len(got), len(want))
		return
	}
	seen := make(map[string]int, len(want))
	for _, ep := range want {
		seen[ep.Name]++
	}
	for _, ep := range got {
		seen[ep.Name]--
	}
	for epName, n := range seen {
		if n != 0 {
			t.Errorf("%s: endpoint multiset differs from fixture (%s count delta %d)", name, epName, n)
		}
	}
}
