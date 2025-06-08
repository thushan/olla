package balancer

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func TestNewPrioritySelector(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())

	if selector == nil {
		t.Fatal("NewPrioritySelector returned nil")
	}

	if selector.statsCollector.GetConnectionStats() == nil {
		t.Error("Connections map not initialised")
	}

	if selector.Name() != DefaultBalancerPriority {
		t.Errorf("Expected name '%s', got %q", DefaultBalancerPriority, selector.Name())
	}
}

func TestPrioritySelector_Select_NoEndpoints(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoint, err := selector.Select(ctx, []*domain.Endpoint{})
	if err == nil {
		t.Error("Expected error for empty endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for empty slice")
	}
}

func TestPrioritySelector_Select_NoRoutableEndpoints(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("offline", 11434, domain.StatusOffline, 100),
		createPriorityEndpoint("unhealthy", 11435, domain.StatusUnhealthy, 200),
		createPriorityEndpoint("unknown", 11436, domain.StatusUnknown, 300),
	}

	endpoint, err := selector.Select(ctx, endpoints)
	if err == nil {
		t.Error("Expected error for no routable endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for no routable endpoints")
	}
}

func TestPrioritySelector_Select_SingleEndpoint(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("single", 11434, domain.StatusHealthy, 100),
	}

	endpoint, err := selector.Select(ctx, endpoints)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if endpoint == nil {
		t.Fatal("Expected endpoint, got nil")
	}
	if endpoint.Name != "single" {
		t.Errorf("Expected 'single', got %s", endpoint.Name)
	}
}

func TestPrioritySelector_Select_HighestPriority(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("low", 11434, domain.StatusHealthy, 100),
		createPriorityEndpoint("high", 11435, domain.StatusHealthy, 300),
		createPriorityEndpoint("medium", 11436, domain.StatusHealthy, 200),
	}

	// Should always select hightest priority
	for i := 0; i < 10; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if endpoint.Name != "high" {
			t.Errorf("Expected highest priority 'high', got %s", endpoint.Name)
		}
	}
}

func TestPrioritySelector_Select_SamePriorityWeightedSelection(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	// All endpoints have same priority but different statuses
	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("healthy", 11434, domain.StatusHealthy, 100),
		createPriorityEndpoint("busy", 11435, domain.StatusBusy, 100),
		createPriorityEndpoint("warming", 11436, domain.StatusWarming, 100),
	}

	selections := make(map[string]int)
	totalSelections := 1000

	// Run many selections to test weighted distribution
	for i := 0; i < totalSelections; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// Healthy should be selected most (weight 1.0)
	// Busy should be selected less (weight 0.3)
	// Warming should be selected least (weight 0.1)
	if selections["healthy"] <= selections["busy"] {
		t.Error("Healthy endpoint should be selected more than busy")
	}
	if selections["busy"] <= selections["warming"] {
		t.Error("Busy endpoint should be selected more than warming")
	}

	// All should be selected at least once
	for name, count := range selections {
		if count == 0 {
			t.Errorf("Endpoint %s was never selected", name)
		}
	}
}

func TestPrioritySelector_Select_PriorityOverridesWeight(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("low-healthy", 11434, domain.StatusHealthy, 100), // Lower priority but healthy
		createPriorityEndpoint("high-busy", 11435, domain.StatusBusy, 200),      // Higher priority but busy
	}

	// High priority endpoint should always be selected despite less weight
	for i := 0; i < 20; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if endpoint.Name != "high-busy" {
			t.Errorf("Expected higher priority endpoint, got %s", endpoint.Name)
		}
	}
}

func TestPrioritySelector_Select_OnlyRoutableEndpoints(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("offline-high", 11434, domain.StatusOffline, 300),    // Highest priority but offline
		createPriorityEndpoint("healthy-low", 11435, domain.StatusHealthy, 100),     // Lower priority but healthy
		createPriorityEndpoint("unhealthy-med", 11436, domain.StatusUnhealthy, 200), // Medium priority but unhealthy
	}

	// Should select healthy endpoint despite lower priority
	for i := 0; i < 10; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if endpoint.Name != "healthy-low" {
			t.Errorf("Expected routable endpoint, got %s", endpoint.Name)
		}
	}
}
func TestPrioritySelector_ConnectionTracking(t *testing.T) {
	mockCollector := NewTestStatsCollector()
	selector := NewPrioritySelector(mockCollector)
	endpoint := createPriorityEndpoint("test", 11434, domain.StatusHealthy, 100)

	selector.IncrementConnections(endpoint)
	selector.IncrementConnections(endpoint)

	// Get connection count from stats collector instead
	connectionStats := mockCollector.GetConnectionStats()
	count := connectionStats[endpoint.URL.String()]
	if count != 2 {
		t.Errorf("Expected 2 connections, got %d", count)
	}

	selector.DecrementConnections(endpoint)
	connectionStats = mockCollector.GetConnectionStats()
	count = connectionStats[endpoint.URL.String()]
	if count != 1 {
		t.Errorf("Expected 1 connection after decrement, got %d", count)
	}

	selector.DecrementConnections(endpoint)
	connectionStats = mockCollector.GetConnectionStats()
	count = connectionStats[endpoint.URL.String()]
	if count != 0 {
		t.Errorf("Expected 0 connections, got %d", count)
	}
}

func TestPrioritySelector_DecrementBelowZero(t *testing.T) {
	mockCollector := NewTestStatsCollector()
	selector := NewPrioritySelector(mockCollector)
	endpoint := createPriorityEndpoint("test", 11434, domain.StatusHealthy, 100)

	selector.DecrementConnections(endpoint)

	// Get connection count from stats collector
	connectionStats := mockCollector.GetConnectionStats()
	count := connectionStats[endpoint.URL.String()]
	if count != 0 {
		t.Errorf("Expected 0 connections after decrement below zero, got %d", count)
	}
}
func TestPrioritySelector_GetConnectionStats(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("endpoint-1", 11434, domain.StatusHealthy, 100),
		createPriorityEndpoint("endpoint-2", 11435, domain.StatusHealthy, 200),
	}

	selector.IncrementConnections(endpoints[0])
	selector.IncrementConnections(endpoints[0])
	selector.IncrementConnections(endpoints[1])

	stats := selector.statsCollector.GetConnectionStats()
	if len(stats) != 2 {
		t.Errorf("Expected 2 entries in stats, got %d", len(stats))
	}

	url1 := endpoints[0].URL.String()
	url2 := endpoints[1].URL.String()

	if stats[url1] != 2 {
		t.Errorf("Expected 2 connections for endpoint-1, got %d", stats[url1])
	}
	if stats[url2] != 1 {
		t.Errorf("Expected 1 connection for endpoint-2, got %d", stats[url2])
	}
}

func TestPrioritySelector_WeightedSelect_ZeroWeight(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	// All endpoints have zero weight (emulating an offline status)
	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("offline-1", 11434, domain.StatusOffline, 100),
		createPriorityEndpoint("offline-2", 11435, domain.StatusOffline, 100),
	}

	// Should still work, but fallback to random selection
	// We just verify no panic or error occurs
	for i := 0; i < 10; i++ {
		_, err := selector.Select(ctx, endpoints)
		if err == nil {
			t.Error("Expected error for offline endpoints")
		}
	}
}

func TestPrioritySelector_WeightedSelect_SingleEndpointSamePriority(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("single", 11434, domain.StatusHealthy, 100),
	}

	for i := 0; i < 10; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if endpoint.Name != "single" {
			t.Errorf("Expected 'single', got %s", endpoint.Name)
		}
	}
}

func TestPrioritySelector_ConcurrentAccess(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("endpoint-1", 11434, domain.StatusHealthy, 200),
		createPriorityEndpoint("endpoint-2", 11435, domain.StatusHealthy, 100),
		createPriorityEndpoint("endpoint-3", 11436, domain.StatusBusy, 200),
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// probably a bit lame way to test concurrency
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := selector.Select(ctx, endpoints)
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			endpoint := endpoints[id%len(endpoints)]
			for j := 0; j < 10; j++ {
				selector.IncrementConnections(endpoint)
				selector.DecrementConnections(endpoint)
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				stats := selector.statsCollector.GetConnectionStats()
				if stats == nil {
					errors <- fmt.Errorf("got nil stats")
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestPrioritySelector_MultiTierPriority(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		// Tier 1 (priority 300) - should always be selected first
		createPriorityEndpoint("tier1-healthy", 11434, domain.StatusHealthy, 300),
		createPriorityEndpoint("tier1-busy", 11435, domain.StatusBusy, 300),

		// Tier 2 (priority 200) - should only be selected if tier 1 unavailable
		createPriorityEndpoint("tier2-healthy", 11436, domain.StatusHealthy, 200),

		// Tier 3 (priority 100) - should only be selected if tiers 1&2 unavailable
		createPriorityEndpoint("tier3-healthy", 11437, domain.StatusHealthy, 100),
	}

	selections := make(map[string]int)

	for i := 0; i < 100; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	if selections["tier2-healthy"] > 0 || selections["tier3-healthy"] > 0 {
		t.Error("Lower tier endpoints selected when higher tier available")
	}

	if selections["tier1-healthy"] == 0 && selections["tier1-busy"] == 0 {
		t.Error("No tier 1 endpoints selected")
	}
}

func TestPrioritySelector_FallbackToLowerTier(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		// Tier 1 (offline)
		createPriorityEndpoint("tier1-offline", 11434, domain.StatusOffline, 300),

		// Tier 2 (available)
		createPriorityEndpoint("tier2-healthy", 11435, domain.StatusHealthy, 200),
		createPriorityEndpoint("tier2-busy", 11436, domain.StatusBusy, 200),

		// Tier 3 (available but shouldn't be selected)
		createPriorityEndpoint("tier3-healthy", 11437, domain.StatusHealthy, 100),
	}

	selections := make(map[string]int)

	for i := 0; i < 100; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// Tien 1 should never be selected (offline)
	if selections["tier1-offline"] > 0 {
		t.Error("Offline tier 1 endpoint was selected")
	}

	// Tier 3 should never be selected (tier 2 available)
	if selections["tier3-healthy"] > 0 {
		t.Error("Tier 3 endpoint selected when tier 2 available")
	}

	// Tear 2 should be selected
	if selections["tier2-healthy"] == 0 && selections["tier2-busy"] == 0 {
		t.Error("No tier 2 endpoints selected")
	}
}

func TestPrioritySelector_TrafficWeightDistribution(t *testing.T) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("healthy", 11434, domain.StatusHealthy, 100), // Weight: 1.0
		createPriorityEndpoint("busy", 11435, domain.StatusBusy, 100),       // Weight: 0.3
		createPriorityEndpoint("warming", 11436, domain.StatusWarming, 100), // Weight: 0.1
	}

	selections := make(map[string]int)
	totalSelections := 1000

	for i := 0; i < totalSelections; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	healthyRatio := float64(selections["healthy"]) / float64(totalSelections)
	busyRatio := float64(selections["busy"]) / float64(totalSelections)
	warmingRatio := float64(selections["warming"]) / float64(totalSelections)

	// Healthy should have highest ratio (~71% based on weights 1.0, 0.3, 0.1)
	// Busy should have medium ratio (~21%)
	// Warming should have lowest ratio (~7%)

	if healthyRatio < 0.6 {
		t.Errorf("Healthy endpoint selected too rarely: %.2f", healthyRatio)
	}
	if busyRatio > healthyRatio {
		t.Error("Busy endpoint selected more than healthy")
	}
	if warmingRatio > busyRatio {
		t.Error("Warming endpoint selected more than busy")
	}

	for name, count := range selections {
		if count == 0 {
			t.Errorf("Endpoint %s was never selected", name)
		}
	}
}

func TestPrioritySelector_SeedingConsistency(t *testing.T) {
	// This test ensures that weighted selection isn't completely deterministic
	// but also not completely random (should have some consistency)

	endpoints := []*domain.Endpoint{
		createPriorityEndpoint("a", 11434, domain.StatusHealthy, 100),
		createPriorityEndpoint("b", 11435, domain.StatusHealthy, 100),
	}

	// Run multiple times to see if we get some variation
	results := make([]string, 20)

	for i := 0; i < 20; i++ {
		selector := NewPrioritySelector(NewTestStatsCollector())
		endpoint, _ := selector.Select(context.Background(), endpoints)
		results[i] = endpoint.Name
	}

	// Should have some variation (not all same)
	firstResult := results[0]
	hasVariation := false
	for _, result := range results[1:] {
		if result != firstResult {
			hasVariation = true
			break
		}
	}

	if !hasVariation {
		t.Error("No variation in weighted selection - may be too deterministic")
	}
}

func createPriorityEndpoint(name string, port int, status domain.EndpointStatus, priority int) *domain.Endpoint {
	testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
	return &domain.Endpoint{
		Name:           name,
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         status,
		Priority:       priority,
		CheckInterval:  5 * time.Second,
		CheckTimeout:   2 * time.Second,
	}
}
