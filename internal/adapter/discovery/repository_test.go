package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestStaticEndpointRepository_BasicOperations(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

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

	aerr := repo.Add(ctx, endpoint)
	if aerr != nil {
		t.Fatalf("failed to add endpoint: %v", aerr)
	}

	// First call should build cache
	healthy1, _ := repo.GetHealthy(ctx)
	if len(healthy1) != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", len(healthy1))
	}

	// Change status - should invalidate cache
	endpoint.Status = domain.StatusOffline
	err := repo.UpdateEndpoint(ctx, endpoint)
	if err != nil {
		// fail test
		t.Fatalf("UpdateEndpoint failed: %v", err)
	}

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
func TestStaticEndpointRepository_UpsertFromConfig_BasicOperation(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "test-endpoint",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	_, err := repo.UpsertFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}

	endpoint := endpoints[0]
	if endpoint.Name != "test-endpoint" {
		t.Errorf("Expected name 'test-endpoint', got %s", endpoint.Name)
	}
	if endpoint.Priority != 100 {
		t.Errorf("Expected priority 100, got %d", endpoint.Priority)
	}
	if endpoint.Status != domain.StatusUnknown {
		t.Errorf("Expected status Unknown for new endpoint, got %s", endpoint.Status)
	}
}
func TestStaticEndpointRepository_UpsertFromConfig_HealthStatePreservation(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// First, add an endpoint using the old method
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	modelURL, _ := url.Parse("http://localhost:11434/api/tags")
	originalEndpoint := &domain.Endpoint{
		Name:                 "test",
		URL:                  testURL,
		HealthCheckURL:       healthURL,
		ModelUrl:             modelURL,
		Status:               domain.StatusHealthy,
		Priority:             100,
		CheckInterval:        5 * time.Second,
		CheckTimeout:         2 * time.Second,
		URLString:            testURL.String(),
		HealthCheckURLString: healthURL.String(),
		ModelURLString:       modelURL.String(),
		LastChecked:          time.Now(),
		ConsecutiveFailures:  2,
		BackoffMultiplier:    4,
		LastLatency:          100 * time.Millisecond,
	}
	repo.Add(ctx, originalEndpoint)

	// Now use UpsertFromConfig with the EXACT same endpoint config
	// This is crucial - the URLs must match exactly for state preservation
	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "http://localhost:11434/health",   // Absolute URL to match existing
			ModelURL:       "http://localhost:11434/api/tags", // Absolute URL to match existing
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	_, err := repo.UpsertFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	endpoints, _ := repo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	updatedEndpoint := endpoints[0]

	// Health state should be preserved
	if updatedEndpoint.Status != domain.StatusHealthy {
		t.Errorf("Expected preserved status Healthy, got %s", updatedEndpoint.Status)
	}
	if updatedEndpoint.ConsecutiveFailures != 2 {
		t.Errorf("Expected preserved ConsecutiveFailures 2, got %d", updatedEndpoint.ConsecutiveFailures)
	}
	if updatedEndpoint.BackoffMultiplier != 4 {
		t.Errorf("Expected preserved BackoffMultiplier 4, got %d", updatedEndpoint.BackoffMultiplier)
	}
	if updatedEndpoint.LastLatency != 100*time.Millisecond {
		t.Errorf("Expected preserved LastLatency 100ms, got %v", updatedEndpoint.LastLatency)
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_ConfigChanged(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add endpoint with health state
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	originalEndpoint := &domain.Endpoint{
		Name:                "test",
		URL:                 testURL,
		HealthCheckURL:      healthURL,
		Status:              domain.StatusHealthy,
		Priority:            100,
		CheckInterval:       5 * time.Second,
		CheckTimeout:        2 * time.Second,
		ConsecutiveFailures: 2,
		BackoffMultiplier:   4,
	}
	repo.Add(ctx, originalEndpoint)

	// Update with changed priority
	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			Priority:       200, // Changed priority
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	_, err := repo.UpsertFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	endpoints, _ := repo.GetAll(ctx)
	updatedEndpoint := endpoints[0]

	// Config should be updated, health state should be reset
	if updatedEndpoint.Priority != 200 {
		t.Errorf("Expected updated priority 200, got %d", updatedEndpoint.Priority)
	}
	if updatedEndpoint.Status != domain.StatusUnknown {
		t.Errorf("Expected reset status Unknown, got %s", updatedEndpoint.Status)
	}
	if updatedEndpoint.ConsecutiveFailures != 0 {
		t.Errorf("Expected reset ConsecutiveFailures 0, got %d", updatedEndpoint.ConsecutiveFailures)
	}
	if updatedEndpoint.BackoffMultiplier != 1 {
		t.Errorf("Expected reset BackoffMultiplier 1, got %d", updatedEndpoint.BackoffMultiplier)
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_EmptyConfig(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add some endpoints first
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:           "test",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}
	repo.Add(ctx, endpoint)

	// Verify endpoint exists
	endpoints, _ := repo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint before empty config, got %d", len(endpoints))
	}

	// UpsertFromConfig with empty slice should clear all endpoints
	_, err := repo.UpsertFromConfig(ctx, []config.EndpointConfig{})
	if err != nil {
		t.Fatalf("UpsertFromConfig with empty config failed: %v", err)
	}

	endpoints, _ = repo.GetAll(ctx)
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints after empty config, got %d", len(endpoints))
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name   string
		config config.EndpointConfig
	}{
		{
			name: "empty URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
		},
		{
			name: "empty health check URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "", // This should fail validation
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
		},
		{
			name: "empty model URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "", // This should fail validation
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
		},
		{
			name: "timeout too long",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  2 * time.Second,
				CheckTimeout:   5 * time.Second, // Greater than interval
			},
		},
		{
			name: "negative priority",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
				Priority:       -1, // Negative priority should fail
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewStaticEndpointRepository() // Fresh repo for each test
			ctx := context.Background()

			_, err := repo.UpsertFromConfig(ctx, []config.EndpointConfig{tc.config})
			if err == nil {
				t.Errorf("Expected validation error for %s", tc.name)
			}

			// Verify no endpoints were added despite error
			endpoints, _ := repo.GetAll(ctx)
			if len(endpoints) != 0 {
				t.Errorf("Expected 0 endpoints after validation error, got %d", len(endpoints))
			}
		})
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_AtomicOperation(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add an existing endpoint
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	existingEndpoint := &domain.Endpoint{
		Name:           "existing",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}
	repo.Add(ctx, existingEndpoint)

	// Try to upsert with mix of valid and invalid configs
	configs := []config.EndpointConfig{
		{
			Name:           "valid",
			URL:            "http://localhost:11435",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
		{
			Name:           "invalid",
			URL:            "", // Invalid
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	_, err := repo.UpsertFromConfig(ctx, configs)
	if err == nil {
		t.Fatal("Expected error due to invalid config")
	}

	// Origina endpoint should still exist (atomic failure)
	endpoints, _ := repo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint after atomic failure, got %d", len(endpoints))
	}
	if endpoints[0].Name != "existing" {
		t.Errorf("Expected existing endpoint to remain, got %s", endpoints[0].Name)
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_URLResolution(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/api/health", // Relative URL
			ModelURL:       "/api/tags",   // Relative URL
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	_, err := repo.UpsertFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	endpoints, _ := repo.GetAll(ctx)
	endpoint := endpoints[0]

	// URLs should be resolved to absolute
	expectedHealthURL := "http://localhost:11434/api/health"
	expectedModelURL := "http://localhost:11434/api/tags"

	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("Expected health URL %s, got %s", expectedHealthURL, endpoint.HealthCheckURLString)
	}
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("Expected model URL %s, got %s", expectedModelURL, endpoint.ModelURLString)
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_CacheInvalidation(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add endpoint and build cache
	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	repo.UpsertFromConfig(ctx, configs)

	// Get healthy endpoints to build cache
	repo.GetHealthy(ctx)

	// Update endpoint status manually
	endpoints, _ := repo.GetAll(ctx)
	endpoints[0].Status = domain.StatusHealthy
	repo.UpdateEndpoint(ctx, endpoints[0])

	// Cache should show healthy endpoint
	healthy1, _ := repo.GetHealthy(ctx)
	if len(healthy1) != 1 {
		t.Fatalf("Expected 1 healthy endpoint, got %d", len(healthy1))
	}

	// UpsertFromConfig should invalidate cache
	// Same config should preserve health state
	repo.UpsertFromConfig(ctx, configs)

	// Cache should still be valid and show healthy endpoint
	healthy2, _ := repo.GetHealthy(ctx)
	if len(healthy2) != 1 {
		t.Errorf("Expected 1 healthy endpoint after upsert, got %d", len(healthy2))
	}
}
func TestStaticEndpointRepository_UpsertFromConfig_ValidationErrorsComprehensive(t *testing.T) {
	testCases := []struct {
		name        string
		config      config.EndpointConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
				Priority:       100,
			},
			expectError: false,
		},
		{
			name: "empty URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectError: true,
		},
		{
			name: "empty health check URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectError: true,
		},
		{
			name: "empty model URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectError: true,
		},
		{
			name: "check interval too short",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  500 * time.Millisecond, // Below MinHealthCheckInterval
				CheckTimeout:   2 * time.Second,
			},
			expectError: true,
		},
		{
			name: "check timeout too long",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   35 * time.Second, // Above MaxHealthCheckTimeout
			},
			expectError: true,
		},
		{
			name: "timeout greater than interval",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  2 * time.Second,
				CheckTimeout:   5 * time.Second,
			},
			expectError: true,
		},
		{
			name: "negative priority",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
				Priority:       -1,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewStaticEndpointRepository() // Fresh repo for each test
			ctx := context.Background()

			_, err := repo.UpsertFromConfig(ctx, []config.EndpointConfig{tc.config})

			if tc.expectError && err == nil {
				t.Errorf("Expected validation error for %s", tc.name)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.name, err)
			}

			// Verify endpoint count matches expectation
			endpoints, _ := repo.GetAll(ctx)
			expectedCount := 0
			if !tc.expectError {
				expectedCount = 1
			}

			if len(endpoints) != expectedCount {
				t.Errorf("Expected %d endpoints for %s, got %d", expectedCount, tc.name, len(endpoints))
			}
		})
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_HealthStatePreservationBug(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Create endpoint with health history
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	modelURL, _ := url.Parse("http://localhost:11434/api/tags")

	originalEndpoint := &domain.Endpoint{
		Name:                 "test",
		URL:                  testURL,
		HealthCheckURL:       healthURL,
		ModelUrl:             modelURL,
		Status:               domain.StatusHealthy,
		Priority:             100,
		CheckInterval:        5 * time.Second,
		CheckTimeout:         2 * time.Second,
		URLString:            testURL.String(),
		HealthCheckURLString: healthURL.String(),
		ModelURLString:       modelURL.String(),
		LastChecked:          time.Now().Add(-1 * time.Minute),
		ConsecutiveFailures:  3,
		BackoffMultiplier:    8,
		LastLatency:          500 * time.Millisecond,
	}

	err := repo.Add(ctx, originalEndpoint)
	if err != nil {
		t.Fatalf("Failed to add original endpoint: %v", err)
	}

	// Update with EXACT same config - should preserve health state
	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "http://localhost:11434/health",   // Absolute URL
			ModelURL:       "http://localhost:11434/api/tags", // Absolute URL
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	changeResult, err := repo.UpsertFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	// Should detect no changes
	if changeResult.Changed {
		t.Error("CRITICAL BUG: Should not detect changes for identical config")
	}

	endpoints, _ := repo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	endpoint := endpoints[0]

	// CRITICAL: Health state should be preserved for unchanged endpoints
	if endpoint.Status != domain.StatusHealthy {
		t.Errorf("CRITICAL BUG: Expected preserved status Healthy, got %s", endpoint.Status)
	}
	if endpoint.ConsecutiveFailures != 3 {
		t.Errorf("CRITICAL BUG: Expected preserved ConsecutiveFailures 3, got %d", endpoint.ConsecutiveFailures)
	}
	if endpoint.BackoffMultiplier != 8 {
		t.Errorf("CRITICAL BUG: Expected preserved BackoffMultiplier 8, got %d", endpoint.BackoffMultiplier)
	}
	if endpoint.LastLatency != 500*time.Millisecond {
		t.Errorf("CRITICAL BUG: Expected preserved LastLatency 500ms, got %v", endpoint.LastLatency)
	}

	// Check that LastChecked is preserved (within 1 second tolerance)
	if time.Since(endpoint.LastChecked) > time.Since(originalEndpoint.LastChecked)+time.Second {
		t.Errorf("CRITICAL BUG: LastChecked not preserved, got %v", endpoint.LastChecked)
	}
}

func TestStaticEndpointRepository_UpsertFromConfig_NewEndpointsNeedScheduling(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "new-endpoint",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	before := time.Now()
	changeResult, err := repo.UpsertFromConfig(ctx, configs)
	after := time.Now()

	if err != nil {
		t.Fatalf("UpsertFromConfig failed: %v", err)
	}

	if !changeResult.Changed {
		t.Error("Should detect changes when adding new endpoint")
	}

	if len(changeResult.Added) != 1 {
		t.Errorf("Expected 1 added endpoint, got %d", len(changeResult.Added))
	}

	endpoints, _ := repo.GetAll(ctx)
	endpoint := endpoints[0]

	// New endpoints should have NextCheckTime set to "now" for immediate scheduling
	if endpoint.NextCheckTime.Before(before) || endpoint.NextCheckTime.After(after.Add(time.Second)) {
		t.Errorf("CRITICAL: New endpoint NextCheckTime not set properly for scheduling: %v", endpoint.NextCheckTime)
	}

	// New endpoints should start with StatusUnknown
	if endpoint.Status != domain.StatusUnknown {
		t.Errorf("New endpoint should start with StatusUnknown, got %s", endpoint.Status)
	}

	t.Logf("INTEGRATION REQUIREMENT: New endpoint %s needs to be scheduled for health checks", endpoint.Name)
}

func TestStaticEndpointRepository_UpsertFromConfig_RaceConditionProtection(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Add initial endpoint
	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:                 "test",
		URL:                  testURL,
		HealthCheckURL:       healthURL,
		Status:               domain.StatusHealthy,
		URLString:            testURL.String(),
		HealthCheckURLString: healthURL.String(),
	}
	repo.Add(ctx, endpoint)

	// Simulate concurrent config updates
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	configs := []config.EndpointConfig{
		{
			Name:           "test",
			URL:            "http://localhost:11434",
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	// Run concurrent UpsertFromConfig calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.UpsertFromConfig(ctx, configs)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent UpsertFromConfig error: %v", err)
	}

	// Verify repository is in consisent state
	endpoints, _ := repo.GetAll(ctx)
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint after concurrent updates, got %d", len(endpoints))
	}
}
