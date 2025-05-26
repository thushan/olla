package discovery

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"net/url"
	"testing"
)

func BenchmarkRepository_GetAll(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Setup 100 test endpoints
	for i := 0; i < 100; i++ {
		port := 11434 + i
		testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
		healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
		endpoint := &domain.Endpoint{
			Name:           fmt.Sprintf("bench-%d", i),
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         domain.StatusHealthy,
		}
		repo.Add(ctx, endpoint)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := repo.GetAll(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepository_GetHealthy(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Mix of statuses - 25% healthy
	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusOffline,
		domain.StatusUnhealthy,
	}

	for i := 0; i < 100; i++ {
		port := 11434 + i
		testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
		healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
		endpoint := &domain.Endpoint{
			Name:           fmt.Sprintf("bench-%d", i),
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         statuses[i%len(statuses)],
		}
		repo.Add(ctx, endpoint)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := repo.GetHealthy(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepository_GetRoutable(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// 60% routable (healthy + busy + warming)
	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusWarming,
		domain.StatusOffline,
		domain.StatusUnhealthy,
	}

	for i := 0; i < 100; i++ {
		port := 11434 + i
		testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
		healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
		endpoint := &domain.Endpoint{
			Name:           fmt.Sprintf("bench-%d", i),
			URL:            testURL,
			HealthCheckURL: healthURL,
			Status:         statuses[i%len(statuses)],
		}
		repo.Add(ctx, endpoint)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := repo.GetRoutable(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepository_StatusUpdate(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	testURL, _ := url.Parse("http://localhost:11434")
	healthURL, _ := url.Parse("http://localhost:11434/health")
	endpoint := &domain.Endpoint{
		Name:           "bench-update",
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         domain.StatusHealthy,
	}
	repo.Add(ctx, endpoint)

	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusOffline,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		status := statuses[i%len(statuses)]
		err := repo.UpdateStatus(ctx, testURL, status)
		if err != nil {
			b.Fatal(err)
		}
	}
}