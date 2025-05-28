package balancer

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()

	if factory == nil {
		t.Fatal("NewFactory returned nil")
	}

	if factory.creators == nil {
		t.Error("Factory creators map not initialised")
	}

	// Test default strategies are registered
	expectedStrategies := []string{DefaultBalancerPriority, DefaultBalancerRoundRobin, DefaultBalancerLeastConnections}
	available := factory.GetAvailableStrategies()

	if len(available) != len(expectedStrategies) {
		t.Errorf("Expected %d default strategies, got %d", len(expectedStrategies), len(available))
	}

	for _, expected := range expectedStrategies {
		found := false
		for _, actual := range available {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected strategy %q not found in available strategies", expected)
		}
	}
}

func TestFactory_Create_DefaultStrategies(t *testing.T) {
	factory := NewFactory()

	testCases := []struct {
		name          string
		expectedType  string
		shouldSucceed bool
	}{
		{DefaultBalancerPriority, DefaultBalancerPriority, true},
		{DefaultBalancerRoundRobin, DefaultBalancerRoundRobin, true},
		{DefaultBalancerLeastConnections, DefaultBalancerLeastConnections, true},
		{"unknown-strategy", "", false},
		{"", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selector, err := factory.Create(tc.name)

			if tc.shouldSucceed {
				if err != nil {
					t.Fatalf("Create(%q) failed: %v", tc.name, err)
				}
				if selector == nil {
					t.Fatal("Create returned nil selector")
				}
				if selector.Name() != tc.expectedType {
					t.Errorf("Expected selector name %q, got %q", tc.expectedType, selector.Name())
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for unknown strategy %q", tc.name)
				}
				if selector != nil {
					t.Error("Expected nil selector for unknown strategy")
				}
			}
		})
	}
}

func TestFactory_Register_CustomStrategy(t *testing.T) {
	factory := NewFactory()
	customName := "custom"

	// Mock custom selector
	customCreator := func() domain.EndpointSelector {
		return &mockEndpointSelector{name: customName}
	}

	// Register custom strategy
	factory.Register(customName, customCreator)

	// Verify it's in available strategies
	available := factory.GetAvailableStrategies()
	found := false
	for _, strategy := range available {
		if strategy == customName {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom strategy not found in available strategies")
	}

	// Test creating custom strategy
	selector, err := factory.Create(customName)
	if err != nil {
		t.Fatalf("Failed to create custom strategy: %v", err)
	}
	if selector.Name() != customName {
		t.Errorf("Expected custom selector name, got %q", selector.Name())
	}
}

func TestFactory_Register_OverrideStrategy(t *testing.T) {
	factory := NewFactory()
	customName := "custom-priority"

	// Create custom priority selector
	customCreator := func() domain.EndpointSelector {
		return &mockEndpointSelector{name: customName}
	}

	// Override existing priority strategy
	factory.Register(DefaultBalancerPriority, customCreator)

	// Test that override works, should overwrite the default priority select
	selector, err := factory.Create(DefaultBalancerPriority)
	if err != nil {
		t.Fatalf("Failed to create overridden strategy: %v", err)
	}
	if selector.Name() != customName {
		t.Errorf("Expected overridden selector, got %q", selector.Name())
	}
}

func TestFactory_GetAvailableStrategies_EmptyFactory(t *testing.T) {
	factory := &Factory{creators: make(map[string]func() domain.EndpointSelector)}

	strategies := factory.GetAvailableStrategies()
	if len(strategies) != 0 {
		t.Errorf("Expected 0 strategies for empty factory, got %d", len(strategies))
	}
}

func TestFactory_ConcurrentAccess(t *testing.T) {
	factory := NewFactory()
	routines := 10
	done := make(chan bool, routines)

	for i := 0; i < routines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			selector, err := factory.Create(DefaultBalancerPriority)
			if err != nil {
				t.Errorf("Goroutine %d: Create failed: %v", id, err)
				return
			}
			if selector == nil {
				t.Errorf("Goroutine %d: Got nil selector", id)
				return
			}

			strategies := factory.GetAvailableStrategies()
			if len(strategies) < 3 {
				t.Errorf("Goroutine %d: Expected at least 3 strategies, got %d", id, len(strategies))
			}
		}(i)
	}

	// wait for all goroutines to finito!
	for i := 0; i < routines; i++ {
		<-done
	}
}

func TestFactory_ConcurrentRegistration(t *testing.T) {
	factory := NewFactory()

	routines := 10
	done := make(chan bool, routines*2)

	for i := 0; i < routines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			strategyName := fmt.Sprintf("strategy-%d", id)
			creator := func() domain.EndpointSelector {
				return &mockEndpointSelector{name: strategyName}
			}
			factory.Register(strategyName, creator)
		}(i)
	}

	for i := 0; i < routines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Try to create default strategies
			_, err := factory.Create(DefaultBalancerPriority)
			if err != nil {
				t.Errorf("Concurrent create failed: %v", err)
			}
		}(i)
	}

	for i := 0; i < routines*2; i++ {
		<-done
	}

	strategies := factory.GetAvailableStrategies()
	customStrategies := 0
	for _, strategy := range strategies {
		if len(strategy) > 9 && strategy[:9] == "strategy-" {
			customStrategies++
		}
	}

	if customStrategies != routines {
		t.Errorf("Expected %d custom strategies, found %d", routines, customStrategies)
	}
}

// Mock endpoint selector for testing
type mockEndpointSelector struct {
	name string
}

func (m *mockEndpointSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	return endpoints[0], nil
}

func (m *mockEndpointSelector) Name() string {
	return m.name
}

func (m *mockEndpointSelector) IncrementConnections(endpoint *domain.Endpoint) {}

func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {}

// Helper function to create test endpoints
func createTestEndpoints(count int, status domain.EndpointStatus) []*domain.Endpoint {
	endpoints := make([]*domain.Endpoint, count)
	for i := 0; i < count; i++ {
		port := 11434 + i
		testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
		healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
		endpoints[i] = &domain.Endpoint{
			Name:           fmt.Sprintf("endpoint-%d", i),
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         status,
			Priority:       100 + i,
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		}
	}
	return endpoints
}
