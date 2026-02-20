package discovery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

func BenchmarkRepository_GetAll(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Setup test endpoints via config
	configs := make([]config.EndpointConfig, 100)
	for i := 0; i < 100; i++ {
		port := 11434 + i
		configs[i] = config.EndpointConfig{
			Name:           fmt.Sprintf("bench-%d", i),
			URL:            fmt.Sprintf("http://localhost:%d", port),
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			Priority:       ptrInt(100),
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		}
	}

	repo.LoadFromConfig(ctx, configs)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := repo.GetAll(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepository_GetRoutable(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Mix of statuses - 60% routable (healthy + busy + warming)
	configs := make([]config.EndpointConfig, 100)
	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusWarming,
		domain.StatusOffline,
		domain.StatusUnhealthy,
	}

	for i := 0; i < 100; i++ {
		port := 11434 + i
		configs[i] = config.EndpointConfig{
			Name:           fmt.Sprintf("bench-%d", i),
			URL:            fmt.Sprintf("http://localhost:%d", port),
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			Priority:       ptrInt(100),
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		}
	}

	repo.LoadFromConfig(ctx, configs)

	// Set mixed statuses after loading
	endpoints, _ := repo.GetAll(ctx)
	for i, endpoint := range endpoints {
		endpoint.Status = statuses[i%len(statuses)]
		repo.UpdateEndpoint(ctx, endpoint)
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

func BenchmarkRepository_UpdateEndpoint(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	cfg := config.EndpointConfig{
		Name:           "bench-update",
		URL:            "http://localhost:11434",
		HealthCheckURL: "/health",
		ModelURL:       "/api/tags",
		Priority:       ptrInt(100),
		CheckInterval:  5 * time.Second,
		CheckTimeout:   2 * time.Second,
	}

	repo.LoadFromConfig(ctx, []config.EndpointConfig{cfg})
	endpoints, _ := repo.GetAll(ctx)
	endpoint := endpoints[0]

	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusOffline,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		endpoint.Status = statuses[i%len(statuses)]
		endpoint.LastChecked = time.Now()
		endpoint.LastLatency = time.Duration(i%100) * time.Millisecond

		err := repo.UpdateEndpoint(ctx, endpoint)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepository_LoadFromConfig(b *testing.B) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	// Create realistic config with 10 endpoints
	configs := make([]config.EndpointConfig, 10)
	for i := 0; i < 10; i++ {
		port := 11434 + i
		configs[i] = config.EndpointConfig{
			Name:           fmt.Sprintf("load-bench-%d", i),
			URL:            fmt.Sprintf("http://localhost:%d", port),
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			Priority:       ptrInt(100 + i*10),
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := repo.LoadFromConfig(ctx, configs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
