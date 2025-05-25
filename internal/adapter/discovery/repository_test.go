package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"sync"
	"testing"
	"time"
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

func TestStaticEndpointRepository_CacheInvalidation(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:           "cache-test",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}

	repo.Add(ctx, endpoint)

	// First call should build cache
	healthy1, _ := repo.GetHealthy(ctx)
	if len(healthy1) != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", len(healthy1))
	}

	// Change status - should invalidate cache
	endpoint.Status = domain.StatusOffline
	repo.UpdateEndpoint(ctx, endpoint)

	// Should return updated results
	healthy2, _ := repo.GetHealthy(ctx)
	if len(healthy2) != 0 {
		t.Errorf("Expected 0 healthy endpoints after status change, got %d", len(healthy2))
	}

	routable, _ := repo.GetRoutable(ctx)
	if len(routable) != 0 {
		t.Errorf("Expected 0 routable endpoints after status change, got %d", len(routable))
	}
}

func TestStaticEndpointRepository_ConcurrentCacheAccess(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add just 3 test endpoints with simple valid URLs for now
	endpoints := []*domain.Endpoint{
		{
			Name:           "endpoint-1",
			URL:            parseURLOrFail(t, "http://localhost:11434"),
			HealthCheckURL: parseURLOrFail(t, "http://localhost:11434/health"),
			Status:         domain.StatusHealthy,
		},
		{
			Name:           "endpoint-2",
			URL:            parseURLOrFail(t, "http://localhost:11435"),
			HealthCheckURL: parseURLOrFail(t, "http://localhost:11435/health"),
			Status:         domain.StatusBusy,
		},
		{
			Name:           "endpoint-3",
			URL:            parseURLOrFail(t, "http://localhost:11436"),
			HealthCheckURL: parseURLOrFail(t, "http://localhost:11436/health"),
			Status:         domain.StatusOffline,
		},
	}

	// Add endpoints to repo
	for _, ep := range endpoints {
		if err := repo.Add(ctx, ep); err != nil {
			t.Fatalf("Failed to add endpoint %s: %v", ep.Name, err)
		}
	}

	var wg sync.WaitGroup
	errorCh := make(chan error, 10)

	// Start 5 concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := repo.GetAll(ctx); err != nil {
					errorCh <- fmt.Errorf("GetAll error: %v", err)
					return
				}
				if _, err := repo.GetHealthy(ctx); err != nil {
					errorCh <- fmt.Errorf("GetHealthy error: %v", err)
					return
				}
				if _, err := repo.GetRoutable(ctx); err != nil {
					errorCh <- fmt.Errorf("GetRoutable error: %v", err)
					return
				}
			}
		}()
	}

	// Start 2 concurrent writers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			targetURL := parseURLOrFail(t, fmt.Sprintf("http://localhost:%d", 11434+writerID))

			for j := 0; j < 10; j++ {
				status := domain.StatusHealthy
				if j%2 == 0 {
					status = domain.StatusOffline
				}

				if err := repo.UpdateStatus(ctx, targetURL, status); err != nil {
					errorCh <- fmt.Errorf("UpdateStatus error: %v", err)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errorCh)

	// Check for errors
	for err := range errorCh {
		t.Error(err)
	}

	// Verify repository is still functional
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("Repository corrupted after concurrent access: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Expected 3 endpoints after concurrent test, got %d", len(all))
	}
}

// Helper function to parse URLs and fail test immediately if invalid
func parseURLOrFail(t *testing.T, urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", urlStr, err)
	}
	if u == nil {
		t.Fatalf("URL.Parse returned nil for %q", urlStr)
	}
	return u
}

func TestStaticEndpointRepository_StatusFilteringAccuracy(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusOffline,
		domain.StatusWarming,
		domain.StatusUnhealthy,
		domain.StatusUnknown,
	}

	// Add endpoints with different statuses
	for i, status := range statuses {
		port := 11434 + i
		testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
		healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
		endpoint := &domain.Endpoint{
			Name:           fmt.Sprintf("status-%s", status),
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         status,
		}
		repo.Add(ctx, endpoint)
	}

	all, _ := repo.GetAll(ctx)
	healthy, _ := repo.GetHealthy(ctx)
	routable, _ := repo.GetRoutable(ctx)

	if len(all) != 6 {
		t.Errorf("Expected 6 total endpoints, got %d", len(all))
	}

	if len(healthy) != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", len(healthy))
	}

	// Routable = healthy + busy + warming = 3
	if len(routable) != 3 {
		t.Errorf("Expected 3 routable endpoints, got %d", len(routable))
	}

	// Verify routable endpoints are correct
	routableStatuses := make(map[domain.EndpointStatus]bool)
	for _, ep := range routable {
		routableStatuses[ep.Status] = true
	}

	expectedRoutable := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusWarming,
	}

	for _, status := range expectedRoutable {
		if !routableStatuses[status] {
			t.Errorf("Expected %s to be in routable endpoints", status)
		}
	}
}