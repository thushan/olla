package balancer

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/ports"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

const balancerName = "least-connections"

func TestNewLeastConnectionsSelector(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())

	if selector == nil {
		t.Fatal("NewLeastConnectionsSelector returned nil")
	}

	if selector.statsCollector.GetConnectionStats() == nil {
		t.Error("Connections map not initialised")
	}

	if selector.Name() != balancerName {
		t.Errorf("Expected name '%s', got %q", balancerName, selector.Name())
	}
}

func TestLeastConnectionsSelector_Select_NoEndpoints(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoint, err := selector.Select(ctx, []*domain.Endpoint{})
	if err == nil {
		t.Error("Expected error for empty endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for empty slice")
	}
}

func TestLeastConnectionsSelector_Select_NoRoutableEndpoints(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	// Create endpoints with non-routable statuses
	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", 11434, domain.StatusOffline),
		createTestEndpoint("endpoint-2", 11435, domain.StatusUnhealthy),
		createTestEndpoint("endpoint-3", 11436, domain.StatusUnknown),
	}

	endpoint, err := selector.Select(ctx, endpoints)
	if err == nil {
		t.Error("Expected error for no routable endpoints")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint for no routable endpoints")
	}
}

func TestLeastConnectionsSelector_Select_SingleEndpoint(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", 11434, domain.StatusHealthy),
	}

	endpoint, err := selector.Select(ctx, endpoints)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if endpoint == nil {
		t.Fatal("Expected endpoint, got nil")
	}
	if endpoint.Name != "endpoint-1" {
		t.Errorf("Expected endpoint-1, got %s", endpoint.Name)
	}
}

func TestLeastConnectionsSelector_Select_MultipleEndpoints(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createTestEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createTestEndpoint("endpoint-3", 11436, domain.StatusBusy),
	}

	// First selection should pick first endpoint (all have 0 connections)
	endpoint, err := selector.Select(ctx, endpoints)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if endpoint.Name != "endpoint-1" {
		t.Errorf("Expected endpoint-1 first, got %s", endpoint.Name)
	}

	// Add connections to first endpoint
	selector.IncrementConnections(endpoints[0])
	selector.IncrementConnections(endpoints[0])

	// Add one connection to second endpoint
	selector.IncrementConnections(endpoints[1])

	// Third endpoint should now be selected (0 connections vs 2 and 1)
	endpoint, err = selector.Select(ctx, endpoints)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if endpoint.Name != "endpoint-3" {
		t.Errorf("Expected endpoint-3 (least connections), got %s", endpoint.Name)
	}
}

func TestLeastConnectionsSelector_Select_OnlyRoutableEndpoints(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("offline", 11434, domain.StatusOffline),
		createTestEndpoint("healthy", 11435, domain.StatusHealthy),
		createTestEndpoint("unhealthy", 11436, domain.StatusUnhealthy),
		createTestEndpoint("busy", 11437, domain.StatusBusy),
		createTestEndpoint("warming", 11438, domain.StatusWarming),
	}

	// Should only consider healthy, busy, and warming endpoints
	selectedNames := make(map[string]int)
	connectionCounts := make(map[string]int)

	for i := 0; i < 50; i++ {
		endpoint, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selectedNames[endpoint.Name]++

		// Simulate connection tracking to ensure load balancing
		selector.IncrementConnections(endpoint)
		connectionCounts[endpoint.Name]++

		// Occasionally complete some connections to create variation
		if i%5 == 0 && connectionCounts[endpoint.Name] > 1 {
			selector.DecrementConnections(endpoint)
			connectionCounts[endpoint.Name]--
		}
	}

	// Should never select offline or unhealthy
	if selectedNames["offline"] > 0 {
		t.Error("Offline endpoint was selected")
	}
	if selectedNames["unhealthy"] > 0 {
		t.Error("Unhealthy endpoint was selected")
	}

	// Should select routable endpoints
	if selectedNames["healthy"] == 0 {
		t.Error("Healthy endpoint was never selected")
	}
	if selectedNames["busy"] == 0 {
		t.Error("Busy endpoint was never selected")
	}
	if selectedNames["warming"] == 0 {
		t.Error("Warming endpoint was never selected")
	}
}

func TestLeastConnectionsSelector_ConnectionTracking(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoint := createTestEndpoint("test", 11434, domain.StatusHealthy)

	// Test increment
	selector.IncrementConnections(endpoint)
	selector.IncrementConnections(endpoint)

	// Verify internal state by selecting from multiple endpoints
	endpoints := []*domain.Endpoint{
		endpoint,
		createTestEndpoint("test2", 11435, domain.StatusHealthy),
	}

	selected, _ := selector.Select(ctx, endpoints)
	// Should select test2 since it has 0 connections vs test with 2
	if selected.Name != "test2" {
		t.Error("Expected endpoint with fewer connections to be selected")
	}

	// Test decrement
	selector.DecrementConnections(endpoint)
	selected, _ = selector.Select(ctx, endpoints)
	// Now test has 1 connection, test2 has 0, so test2 should still be selected
	if selected.Name != "test2" {
		t.Error("Expected endpoint with fewer connections after decrement")
	}

	// Decrement again
	selector.DecrementConnections(endpoint)
	// Both should have 0 connections now, so either could be selected
	// We just verify no error occurs
	_, err := selector.Select(ctx, endpoints)
	if err != nil {
		t.Errorf("Selection failed after decrement to zero: %v", err)
	}
}

func TestLeastConnectionsSelector_DecrementBelowZero(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	endpoint := createTestEndpoint("test", 11434, domain.StatusHealthy)

	// Try to decrement without any connections
	selector.DecrementConnections(endpoint)

	// Should handle gracefully - verify by checking selection behavior
	endpoints := []*domain.Endpoint{
		endpoint,
		createTestEndpoint("test2", 11435, domain.StatusHealthy),
	}

	selector.IncrementConnections(endpoints[1])
	selected, _ := selector.Select(context.Background(), endpoints)

	// First endpoint should be selected (0 connections vs 1)
	if selected.Name != "test" {
		t.Error("Decrement below zero not handled correctly")
	}
}

func TestLeastConnectionsSelector_ConcurrentAccess(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createTestEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createTestEndpoint("endpoint-3", 11436, domain.StatusHealthy),
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent selections
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

	// Concurrent connection tracking
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

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestLeastConnectionsSelector_LoadBalancing(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())
	ctx := context.Background()

	endpoints := []*domain.Endpoint{
		createTestEndpoint("endpoint-1", 11434, domain.StatusHealthy),
		createTestEndpoint("endpoint-2", 11435, domain.StatusHealthy),
		createTestEndpoint("endpoint-3", 11436, domain.StatusHealthy),
	}

	// Track selections
	selections := make(map[string]int)
	connectionCounts := make(map[string]int)

	// Simulate realistic usage pattern
	for i := 0; i < 100; i++ {
		// Select endpoint
		selected, err := selector.Select(ctx, endpoints)
		if err != nil {
			t.Fatalf("Selection failed: %v", err)
		}

		selections[selected.Name]++
		selector.IncrementConnections(selected)
		connectionCounts[selected.Name]++

		// Randomly complete some connections
		if i%3 == 0 && connectionCounts[selected.Name] > 0 {
			selector.DecrementConnections(selected)
			connectionCounts[selected.Name]--
		}
	}

	// Verify some distribution occurred
	if len(selections) < 2 {
		t.Error("Load balancing not working - only one endpoint selected")
	}

	// All endpoints should have been selected at least once
	for _, endpoint := range endpoints {
		if selections[endpoint.Name] == 0 {
			t.Errorf("Endpoint %s was never selected", endpoint.Name)
		}
	}
}

func TestLeastConnectionsSelector_DifferentURLFormats(t *testing.T) {
	selector := NewLeastConnectionsSelector(ports.NewMockStatsCollector())

	// Test with different URL formats
	testCases := []struct {
		name string
		url  string
	}{
		{"http", "http://localhost:11434"},
		{"https", "https://example.com:8080"},
		{"ip", "http://192.168.1.100:11434"},
		{"path", "http://localhost:11434/api"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testURL, _ := url.Parse(tc.url)
			endpoint := &domain.Endpoint{
				Name:   tc.name,
				URL:    testURL,
				Status: domain.StatusHealthy,
			}

			// Test connection tracking with different URL formats
			selector.IncrementConnections(endpoint)
			selector.IncrementConnections(endpoint)
			selector.DecrementConnections(endpoint)

			// Verify it works by selecting
			endpoints := []*domain.Endpoint{endpoint}
			selected, err := selector.Select(context.Background(), endpoints)
			if err != nil {
				t.Errorf("Selection failed for URL %s: %v", tc.url, err)
			}
			if selected.Name != tc.name {
				t.Errorf("Wrong endpoint selected for URL %s", tc.url)
			}
		})
	}
}

// Helper function to create test endpoint
func createTestEndpoint(name string, port int, status domain.EndpointStatus) *domain.Endpoint {
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
