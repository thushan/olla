package discovery

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"testing"
)

func TestStaticEndpointRepository_BasicOperations(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Test empty repository
	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints, got %d", len(endpoints))
	}

	// Add endpoint
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:           "test",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}

	err = repo.Add(ctx, endpoint)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify endpoint was added
	endpoints, err = repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}

	// Test endpoint copy (mutation safety)
	endpoints[0].Status = domain.StatusOffline
	original, _ := repo.GetAll(ctx)
	if original[0].Status == domain.StatusOffline {
		t.Error("Endpoint mutation affected repository - copies not working")
	}

	// Remove endpoint
	err = repo.Remove(ctx, testURL)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	endpoints, _ = repo.GetAll(ctx)
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints after removal, got %d", len(endpoints))
	}
}

func TestStaticEndpointRepository_StatusFiltering(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add endpoints with different statuses
	testCases := []struct {
		name   string
		port   string
		status domain.EndpointStatus
	}{
		{"healthy", "11434", domain.StatusHealthy},
		{"busy", "11435", domain.StatusBusy},
		{"offline", "11436", domain.StatusOffline},
		{"warming", "11437", domain.StatusWarming},
	}

	for _, tc := range testCases {
		testURL, _ := url.Parse("http://localhost:" + tc.port)
		healthURL, _ := url.Parse("http://localhost:" + tc.port + "/health")
		endpoint := &domain.Endpoint{
			Name:           tc.name,
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         tc.status,
		}
		repo.Add(ctx, endpoint)
	}

	// Test GetHealthy (should return only healthy)
	healthy, err := repo.GetHealthy(ctx)
	if err != nil {
		t.Fatalf("GetHealthy failed: %v", err)
	}
	if len(healthy) != 1 || healthy[0].Name != "healthy" {
		t.Errorf("Expected 1 healthy endpoint, got %d", len(healthy))
	}

	// Test GetRoutable (should return healthy, busy, warming)
	routable, err := repo.GetRoutable(ctx)
	if err != nil {
		t.Fatalf("GetRoutable failed: %v", err)
	}
	if len(routable) != 3 {
		t.Errorf("Expected 3 routable endpoints, got %d", len(routable))
	}
}

func TestStaticEndpointRepository_ConcurrentAccess(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:           "concurrent-test",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}

	repo.Add(ctx, endpoint)

	// Concurrent reads and writes
	done := make(chan bool)

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			repo.GetAll(ctx)
			repo.GetHealthy(ctx)
			repo.GetRoutable(ctx)
		}
		done <- true
	}()

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			endpoint.Status = domain.StatusBusy
			repo.UpdateEndpoint(ctx, endpoint)
			endpoint.Status = domain.StatusHealthy
			repo.UpdateEndpoint(ctx, endpoint)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify repository still works
	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("Repository corrupted after concurrent access: %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint after concurrent test, got %d", len(endpoints))
	}
}
