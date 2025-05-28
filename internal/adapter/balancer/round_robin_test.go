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

func TestNewRoundRobinSelector(t *testing.T) {
	selector := NewRoundRobinSelector()

	if selector == nil {
		t.Fatal("NewRoundRobinSelector returned nil")
	}

	if selector.connections == nil {
		t.Error("Connections map not initialised")
	}

	if selector.Name() != DefaultBalancerRoundRobin {
		t.Errorf("Expected name '%s', got %q", DefaultBalancerRoundRobin, selector.Name())
	}

	// Counter should start at 0
	if selector.counter != 0 {
		t.Errorf("Expected counter to start at 0, got %d", selector.counter)
	}
}

func TestRoundRobinSelector_Select_NoEndpoints(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoint, err := selector.Select(ctx, []*domain.Endpoint{})
	if err == nil {
		t.Error("Expected error for empty endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for empty slice")
	}
}

func TestRoundRobinSelector_Select_NoRoutableEndpoints(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("offline", 11434, domain.StatusOffline),
		createRoundRobinEndpoint("unhealthy", 11435, domain.StatusUnhealthy),
		createRoundRobinEndpoint("unknown", 11436, domain.StatusUnknown),
	}

	endpoint, err := selector.Select(ctx, endpoints)
	if err == nil {
		t.Error("Expected error for no routable endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for no routable endpoints")
	}
}

func TestRoundRobinSelector_Select_SingleEndpoint(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("single", 11434, domain.StatusHealthy),
	}

	// Should always return the same endpoint
	for i := 0; i < 5; i++ {
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
}

func TestRoundRobinSelector_Select_RoundRobinDistribution(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-3", 11436, domain.StatusHealthy),
	}

	// Test sequential round-robin behaviour - starts from index 0
	expectedOrder := []string{"endpoint-1", "endpoint-2", "endpoint-3", "endpoint-1", "endpoint-2", "endpoint-3"}

	for i, expected := range expectedOrder {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select %d failed: %v", i, err)
		}
		if endpoint.Name != expected {
			t.Errorf("Selection %d: expected %s, got %s", i, expected, endpoint.Name)
		}
	}
}

func TestRoundRobinSelector_Select_OnlyRoutableEndpoints(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("offline", 11434, domain.StatusOffline),     // Not routable
		createRoundRobinEndpoint("healthy", 11435, domain.StatusHealthy),     // Routable
		createRoundRobinEndpoint("unhealthy", 11436, domain.StatusUnhealthy), // Not routable
		createRoundRobinEndpoint("busy", 11437, domain.StatusBusy),           // Routable
		createRoundRobinEndpoint("warming", 11438, domain.StatusWarming),     // Routable
	}

	selections := make(map[string]int)
	routableEndpoints := []string{"healthy", "busy", "warming"}

	// Run enough selections to cycle through routable endpoints multiple times
	for i := 0; i < 15; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// Should never select non-routable endpoints
	if selections["offline"] > 0 {
		t.Error("Offline endpoint was selected")
	}
	if selections["unhealthy"] > 0 {
		t.Error("Unhealthy endpoint was selected")
	}

	// Should select all routable endpoints
	for _, name := range routableEndpoints {
		if selections[name] == 0 {
			t.Errorf("Routable endpoint %s was never selected", name)
		}
	}

	// Should distribute evenly (15 selections / 3 routable = 5 each)
	for _, name := range routableEndpoints {
		if selections[name] != 5 {
			t.Errorf("Expected 5 selections for %s, got %d", name, selections[name])
		}
	}
}

func TestRoundRobinSelector_Select_CounterOverflow(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
	}

	// Set counter to a high value near overflow
	selector.counter = ^uint64(0) - 5 // Near uint64 max

	// Should handle overflow gracefully
	for i := 0; i < 10; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed with high counter: %v", err)
		}
		if endpoint == nil {
			t.Fatal("Got nil endpoint with high counter")
		}
	}

	// Verify it still works after overflow
	endpoint, err := selector.Select(ctx, endpoints)
	if err != nil {
		t.Fatalf("Select failed after overflow: %v", err)
	}
	if endpoint == nil {
		t.Fatal("Got nil endpoint after overflow")
	}
}

func TestRoundRobinSelector_ConnectionTracking(t *testing.T) {
	selector := NewRoundRobinSelector()
	endpoint := createRoundRobinEndpoint("test", 11434, domain.StatusHealthy)

	// Test increment
	selector.IncrementConnections(endpoint)
	selector.IncrementConnections(endpoint)

	// Verify count
	count := selector.GetConnectionCount(endpoint)
	if count != 2 {
		t.Errorf("Expected 2 connections, got %d", count)
	}

	// Test decrement
	selector.DecrementConnections(endpoint)
	count = selector.GetConnectionCount(endpoint)
	if count != 1 {
		t.Errorf("Expected 1 connection after decrement, got %d", count)
	}

	// Connection tracking doesn't affect selection in round-robin,
	// but we should verify it doesn't cause errors
	endpoints := []*domain.Endpoint{endpoint}
	selected, err := selector.Select(context.Background(), endpoints)
	if err != nil {
		t.Errorf("Selection failed after connection tracking: %v", err)
	}
	if selected.Name != "test" {
		t.Error("Wrong endpoint selected after connection tracking")
	}
}

func TestRoundRobinSelector_DecrementBelowZero(t *testing.T) {
	selector := NewRoundRobinSelector()
	endpoint := createRoundRobinEndpoint("test", 11434, domain.StatusHealthy)

	// Try to decrement without any connections
	selector.DecrementConnections(endpoint)

	// Should handle gracefully - verify no panic and count stays at 0
	count := selector.GetConnectionCount(endpoint)
	if count != 0 {
		t.Errorf("Expected 0 connections after decrement below zero, got %d", count)
	}

	endpoints := []*domain.Endpoint{endpoint}
	_, err := selector.Select(context.Background(), endpoints)
	if err != nil {
		t.Errorf("Selection failed after decrement below zero: %v", err)
	}
}

func TestRoundRobinSelector_ConcurrentAccess(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-3", 11436, domain.StatusHealthy),
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)
	selections := make(chan string, 300)

	// Concurrent selections
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				endpoint, err := selector.Select(ctx, endpoints)
				if err != nil {
					errors <- err
					return
				}
				selections <- endpoint.Name
			}
		}()
	}

	// Concurrent connection tracking
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			endpoint := endpoints[id%len(endpoints)]
			for j := 0; j < 5; j++ {
				selector.IncrementConnections(endpoint)
				selector.DecrementConnections(endpoint)
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(selections)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Verify all selections were valid
	selectionCounts := make(map[string]int)
	totalSelections := 0
	for selection := range selections {
		selectionCounts[selection]++
		totalSelections++

		// Verify it's a valid endpoint name
		valid := false
		for _, endpoint := range endpoints {
			if endpoint.Name == selection {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("Invalid endpoint selected: %s", selection)
		}
	}

	if totalSelections != 200 {
		t.Errorf("Expected 200 total selections, got %d", totalSelections)
	}

	// All endpoints should have been selected at least once
	for _, endpoint := range endpoints {
		if selectionCounts[endpoint.Name] == 0 {
			t.Errorf("Endpoint %s was never selected", endpoint.Name)
		}
	}
}

func TestRoundRobinSelector_DynamicEndpointChanges(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	// Start with 2 endpoints
	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
	}

	// Select a few times
	for i := 0; i < 4; i++ {
		selector.Select(ctx, endpoints)
	}

	// Add a third endpoint (simulates dynamic discovery)
	endpoints = append(endpoints, createRoundRobinEndpoint("endpoint-3", 11436, domain.StatusHealthy))

	// Continue selecting - should include new endpoint
	selections := make(map[string]int)
	for i := 0; i < 12; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// All endpoints should be selected
	if selections["endpoint-1"] == 0 {
		t.Error("endpoint-1 not selected after adding endpoint-3")
	}
	if selections["endpoint-2"] == 0 {
		t.Error("endpoint-2 not selected after adding endpoint-3")
	}
	if selections["endpoint-3"] == 0 {
		t.Error("endpoint-3 not selected after being added")
	}

	// Each should be selected 4 times (12 selections / 3 endpoints)
	for name, count := range selections {
		if count != 4 {
			t.Errorf("Expected 4 selections for %s, got %d", name, count)
		}
	}
}

func TestRoundRobinSelector_StatusChanges(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-3", 11436, domain.StatusHealthy),
	}

	// Make endpoint-2 offline
	endpoints[1].Status = domain.StatusOffline

	// Should only select endpoint-1 and endpoint-3
	selections := make(map[string]int)
	for i := 0; i < 10; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// endpoint-2 should never be selected
	if selections["endpoint-2"] > 0 {
		t.Error("Offline endpoint-2 was selected")
	}

	// endpoint-1 and endpoint-3 should be selected equally
	if selections["endpoint-1"] != 5 || selections["endpoint-3"] != 5 {
		t.Errorf("Expected equal distribution: endpoint-1=%d, endpoint-3=%d",
			selections["endpoint-1"], selections["endpoint-3"])
	}
}

func TestRoundRobinSelector_DistributionFairness(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusBusy),
		createRoundRobinEndpoint("endpoint-3", 11436, domain.StatusWarming),
	}

	selections := make(map[string]int)
	totalSelections := 300 // Multiple of 3 for even distribution

	for i := 0; i < totalSelections; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// Each endpoint should be selected exactly 100 times
	expectedCount := totalSelections / len(endpoints)
	for _, endpoint := range endpoints {
		count := selections[endpoint.Name]
		if count != expectedCount {
			t.Errorf("Unfair distribution: %s selected %d times, expected %d",
				endpoint.Name, count, expectedCount)
		}
	}
}

func TestRoundRobinSelector_LargeEndpointSet(t *testing.T) {
	selector := NewRoundRobinSelector()
	ctx := context.Background()

	// Create 50 endpoints
	endpoints := make([]*domain.Endpoint, 50)
	for i := 0; i < 50; i++ {
		endpoints[i] = createRoundRobinEndpoint(
			fmt.Sprintf("endpoint-%d", i),
			11434+i,
			domain.StatusHealthy,
		)
	}

	selections := make(map[string]int)
	totalSelections := 500 // 10 rounds through all endpoints

	for i := 0; i < totalSelections; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selections[endpoint.Name]++
	}

	// Each endpoint should be selected exactly 10 times
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("endpoint-%d", i)
		count := selections[name]
		if count != 10 {
			t.Errorf("Endpoint %s selected %d times, expected 10", name, count)
		}
	}
}

func TestRoundRobinSelector_GetConnectionStats(t *testing.T) {
	selector := NewRoundRobinSelector()

	endpoints := []*domain.Endpoint{
		createRoundRobinEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createRoundRobinEndpoint("endpoint-2", 11435, domain.StatusHealthy),
	}

	// Add some connections
	selector.IncrementConnections(endpoints[0])
	selector.IncrementConnections(endpoints[0])
	selector.IncrementConnections(endpoints[1])

	stats := selector.GetConnectionStats()
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

// Helper function to create test endpoint for round-robin tests
func createRoundRobinEndpoint(name string, port int, status domain.EndpointStatus) *domain.Endpoint {
	testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
	return &domain.Endpoint{
		Name:           name,
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         status,
		Priority:       100,
		CheckInterval:  5 * time.Second,
		CheckTimeout:   2 * time.Second,
	}
}
